package checks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/internal/doctor"
)

func metricsServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestConsolidationWorkerRunning_Pass(t *testing.T) {
	srv := metricsServer(t, `omnia_memory_worker_running{name="consolidation"} 1
omnia_memory_worker_running{name="retention"} 1
`, http.StatusOK)
	res := NewConsolidationChecker(srv.URL).checkWorkerRunning(t.Context())
	if res.Status != doctor.StatusPass {
		t.Fatalf("want pass, got %s (%s / %s)", res.Status, res.Detail, res.Error)
	}
}

func TestConsolidationWorkerRunning_Fail_Exited(t *testing.T) {
	srv := metricsServer(t, `omnia_memory_worker_running{name="consolidation"} 0
`, http.StatusOK)
	res := NewConsolidationChecker(srv.URL).checkWorkerRunning(t.Context())
	if res.Status != doctor.StatusFail {
		t.Fatalf("want fail, got %s", res.Status)
	}
}

func TestConsolidationWorkerRunning_Skip_SeriesAbsent(t *testing.T) {
	srv := metricsServer(t, `omnia_memory_worker_running{name="retention"} 1
`, http.StatusOK)
	res := NewConsolidationChecker(srv.URL).checkWorkerRunning(t.Context())
	if res.Status != doctor.StatusSkip {
		t.Fatalf("want skip (consolidation series absent), got %s", res.Status)
	}
}

func TestConsolidationWorkerRunning_Skip_NoURL(t *testing.T) {
	res := NewConsolidationChecker("").checkWorkerRunning(t.Context())
	if res.Status != doctor.StatusSkip {
		t.Fatalf("want skip when URL unset, got %s", res.Status)
	}
}

func TestConsolidationWorkerRunning_Fail_FetchError(t *testing.T) {
	srv := metricsServer(t, "boom", http.StatusInternalServerError)
	res := NewConsolidationChecker(srv.URL).checkWorkerRunning(t.Context())
	if res.Status != doctor.StatusFail {
		t.Fatalf("want fail on fetch error, got %s", res.Status)
	}
}
