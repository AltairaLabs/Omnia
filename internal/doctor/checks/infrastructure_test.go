package checks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altairalabs/omnia/internal/doctor"
)

func newTestServer(healthzCode, readyzCode int) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(healthzCode)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(readyzCode)
	})
	return httptest.NewServer(mux)
}

func TestHealthCheck_Pass(t *testing.T) {
	srv := newTestServer(http.StatusOK, http.StatusOK)
	defer srv.Close()

	check := healthCheck("sessionAPI", srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
}

func TestHealthCheck_Fail_NonOK(t *testing.T) {
	srv := newTestServer(http.StatusInternalServerError, http.StatusOK)
	defer srv.Close()

	check := healthCheck("sessionAPI", srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "500") {
		t.Errorf("expected detail to contain HTTP status code, got %q", result.Detail)
	}
}

func TestHealthCheck_Fail_Unreachable(t *testing.T) {
	check := healthCheck("sessionAPI", "http://127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "unreachable") {
		t.Errorf("expected detail to contain 'unreachable', got %q", result.Detail)
	}
}

func TestHealthCheck_Skip_EmptyURL(t *testing.T) {
	check := healthCheck("sessionAPI", "")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

func TestOllamaCheck_Pass(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := OllamaCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
	if check.Name != "OllamaHealthy" {
		t.Errorf("expected name 'OllamaHealthy', got %q", check.Name)
	}
	if check.Category != "Infrastructure" {
		t.Errorf("expected category 'Infrastructure', got %q", check.Category)
	}
}

func TestOllamaCheck_Fail_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := OllamaCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "500") {
		t.Errorf("expected detail to contain '500', got %q", result.Detail)
	}
}

func TestOllamaCheck_Fail_Unreachable(t *testing.T) {
	check := OllamaCheck("http://127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "unreachable") {
		t.Errorf("expected detail to contain 'unreachable', got %q", result.Detail)
	}
}

func TestOllamaCheck_Skip_EmptyURL(t *testing.T) {
	check := OllamaCheck("")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

// --- OperatorAPICheck tests ---

func TestOperatorAPICheck_Pass(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := OperatorAPICheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
	if check.Name != "OperatorAPIHealthy" {
		t.Errorf("expected name 'OperatorAPIHealthy', got %q", check.Name)
	}
}

func TestOperatorAPICheck_Fail_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := OperatorAPICheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "500") {
		t.Errorf("expected detail to contain '500', got %q", result.Detail)
	}
}

func TestOperatorAPICheck_Fail_Unreachable(t *testing.T) {
	check := OperatorAPICheck("http://127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
}

func TestOperatorAPICheck_Skip_EmptyURL(t *testing.T) {
	check := OperatorAPICheck("")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

// --- DashboardCheck tests ---

func TestDashboardCheck_Pass(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := DashboardCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
	if check.Name != "DashboardResponds" {
		t.Errorf("expected name 'DashboardResponds', got %q", check.Name)
	}
}

func TestDashboardCheck_Fail_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := DashboardCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
}

func TestDashboardCheck_Fail_Unreachable(t *testing.T) {
	check := DashboardCheck("http://127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
}

func TestDashboardCheck_Skip_EmptyURL(t *testing.T) {
	check := DashboardCheck("")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

// --- TCPCheck tests ---

func TestTCPCheck_Pass(t *testing.T) {
	// Start a TCP listener to test against.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	check := TCPCheck("Redis", ln.Addr().String())
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
	if check.Name != "RedisReachable" {
		t.Errorf("expected name 'RedisReachable', got %q", check.Name)
	}
	if result.Detail != "accepting connections" {
		t.Errorf("expected detail 'accepting connections', got %q", result.Detail)
	}
}

func TestTCPCheck_Fail_Unreachable(t *testing.T) {
	check := TCPCheck("Redis", "127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "unreachable") {
		t.Errorf("expected detail to contain 'unreachable', got %q", result.Detail)
	}
}

func TestTCPCheck_Skip_EmptyAddr(t *testing.T) {
	check := TCPCheck("Redis", "")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

// --- ArenaControllerCheck tests ---

func TestArenaControllerCheck_Pass(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := ArenaControllerCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
	if check.Name != "ArenaControllerHealthy" {
		t.Errorf("expected name 'ArenaControllerHealthy', got %q", check.Name)
	}
}

func TestArenaControllerCheck_Fail_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	check := ArenaControllerCheck(srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "500") {
		t.Errorf("expected detail to contain '500', got %q", result.Detail)
	}
}

func TestArenaControllerCheck_Skip_Unreachable(t *testing.T) {
	check := ArenaControllerCheck("http://127.0.0.1:1")
	result := check.Run(context.Background())

	// Unreachable arena controller should skip, not fail (not deployed).
	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s: %s", result.Status, result.Detail)
	}
	if !strings.Contains(result.Detail, "not deployed") {
		t.Errorf("expected detail to contain 'not deployed', got %q", result.Detail)
	}
}

func TestArenaControllerCheck_Skip_EmptyURL(t *testing.T) {
	check := ArenaControllerCheck("")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected skip, got %s", result.Status)
	}
}

func TestInfrastructureChecks_Names(t *testing.T) {
	srv := newTestServer(http.StatusOK, http.StatusOK)
	defer srv.Close()

	services := map[string]string{"operator": srv.URL}
	checks := InfrastructureChecks(services)

	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "operatorHealthy" {
		t.Errorf("expected name 'operatorHealthy', got %q", checks[0].Name)
	}
	if checks[0].Category != "Infrastructure" {
		t.Errorf("expected category 'Infrastructure', got %q", checks[0].Category)
	}
}
