package checks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/internal/doctor"
)

const sampleSessionAPIMetrics = `# HELP omnia_session_api_requests_total Total requests.
# TYPE omnia_session_api_requests_total counter
omnia_session_api_requests_total 42
# HELP omnia_session_api_errors_total Total errors.
# TYPE omnia_session_api_errors_total counter
omnia_session_api_errors_total 0
`

func TestObservabilityChecks_Pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleSessionAPIMetrics))
	}))
	defer srv.Close()

	checks := ObservabilityChecks(map[string]string{"SessionAPI": srv.URL})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}

	result := checks[0].Run(t.Context())
	if result.Status != doctor.StatusPass {
		t.Errorf("expected StatusPass, got %s (error: %s)", result.Status, result.Error)
	}
	if result.Detail != "2 metrics found" {
		t.Errorf("expected '2 metrics found', got %q", result.Detail)
	}
}

func TestObservabilityChecks_Skip_EmptyURL(t *testing.T) {
	checks := ObservabilityChecks(map[string]string{"SessionAPI": ""})
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}

	result := checks[0].Run(t.Context())
	if result.Status != doctor.StatusSkip {
		t.Errorf("expected StatusSkip, got %s", result.Status)
	}
	if result.Detail != "no metrics URL configured" {
		t.Errorf("unexpected detail: %q", result.Detail)
	}
}

func TestObservabilityChecks_Fail_NoMatchingPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// metrics from a different service — no omnia_session_api_ prefix
		_, _ = w.Write([]byte("go_goroutines 12\ngo_gc_duration_seconds 0.001\n"))
	}))
	defer srv.Close()

	checks := ObservabilityChecks(map[string]string{"SessionAPI": srv.URL})
	result := checks[0].Run(t.Context())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected StatusFail, got %s", result.Status)
	}
	if result.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestObservabilityChecks_Fail_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	checks := ObservabilityChecks(map[string]string{"SessionAPI": srv.URL})
	result := checks[0].Run(t.Context())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected StatusFail, got %s", result.Status)
	}
	if result.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestObservabilityChecks_Fail_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately so connection is refused

	checks := ObservabilityChecks(map[string]string{"SessionAPI": srv.URL})
	result := checks[0].Run(t.Context())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected StatusFail, got %s", result.Status)
	}
}

func TestObservabilityChecks_Multiple(t *testing.T) {
	makeServer := func(metrics string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(metrics))
		}))
	}

	sessionSrv := makeServer(sampleSessionAPIMetrics)
	defer sessionSrv.Close()

	memoryMetrics := "omnia_memory_api_requests_total 7\n"
	memorySrv := makeServer(memoryMetrics)
	defer memorySrv.Close()

	urls := map[string]string{
		"SessionAPI": sessionSrv.URL,
		"MemoryAPI":  memorySrv.URL,
	}
	checks := ObservabilityChecks(urls)
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	passed := 0
	for _, c := range checks {
		r := c.Run(t.Context())
		if r.Status == doctor.StatusPass {
			passed++
		}
	}
	if passed != 2 {
		t.Errorf("expected 2 passing checks, got %d", passed)
	}
}

func TestServiceMetricsPrefix(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect string
	}{
		{"SessionAPI", "SessionAPI", "omnia_session_api_"},
		{"MemoryAPI", "MemoryAPI", "omnia_memory_api_"},
		{"lowercase", "facade", "omnia_facade_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceMetricsPrefix(tc.input)
			if got != tc.expect {
				t.Errorf("serviceMetricsPrefix(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}
