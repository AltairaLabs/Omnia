package checks

import (
	"context"
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

func TestReadinessCheck_Pass(t *testing.T) {
	srv := newTestServer(http.StatusOK, http.StatusOK)
	defer srv.Close()

	check := readinessCheck("sessionAPI", srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusPass {
		t.Errorf("expected pass, got %s: %s", result.Status, result.Detail)
	}
}

func TestReadinessCheck_Fail_NonOK(t *testing.T) {
	srv := newTestServer(http.StatusOK, http.StatusServiceUnavailable)
	defer srv.Close()

	check := readinessCheck("sessionAPI", srv.URL)
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
	if !strings.Contains(result.Detail, "503") {
		t.Errorf("expected detail to contain HTTP status code, got %q", result.Detail)
	}
}

func TestReadinessCheck_Fail_Unreachable(t *testing.T) {
	check := readinessCheck("sessionAPI", "http://127.0.0.1:1")
	result := check.Run(context.Background())

	if result.Status != doctor.StatusFail {
		t.Errorf("expected fail, got %s", result.Status)
	}
}

func TestReadinessCheck_Skip_EmptyURL(t *testing.T) {
	check := readinessCheck("sessionAPI", "")
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

func TestReadinessChecks_Names(t *testing.T) {
	srv := newTestServer(http.StatusOK, http.StatusOK)
	defer srv.Close()

	services := map[string]string{"sessionAPI": srv.URL}
	checks := ReadinessChecks(services)

	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Name != "sessionAPIReady" {
		t.Errorf("expected name 'sessionAPIReady', got %q", checks[0].Name)
	}
}
