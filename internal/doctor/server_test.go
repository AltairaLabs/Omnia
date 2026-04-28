package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func testServer(checks ...Check) *Server {
	return NewServer(staticRunnerBuilder(checks...), ":0", logr.Discard())
}

// staticRunnerBuilder returns a RunnerBuilder that always yields a
// runner pre-loaded with the given checks. Used by tests that don't
// need per-call rebuilding.
func staticRunnerBuilder(checks ...Check) RunnerBuilder {
	return func(_ context.Context) (*Runner, error) {
		r := NewRunner()
		r.Register(checks...)
		return r, nil
	}
}

func TestHandleIndex(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content type, got %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Omnia Doctor") {
		t.Error("expected page to contain 'Omnia Doctor'")
	}
}

func TestHandleHealthz(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rec.Body.String())
	}
}

func TestHandleRunSSE_MissingStreamParam(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/run", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRunSSE_StreamsResults(t *testing.T) {
	srv := testServer(
		passCheck("a", "cat1"),
		failCheck("b", "cat1"),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/run?stream=true", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	body := rec.Body.String()
	// Should have two data: lines for test results plus a complete event.
	dataLines := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLines++
		}
	}
	// 2 running events + 2 test results + 1 complete event = 5 data lines
	if dataLines != 5 {
		t.Errorf("expected 5 data lines, got %d; body:\n%s", dataLines, body)
	}
	if !strings.Contains(body, "event: complete") {
		t.Error("expected complete event in SSE stream")
	}
}

func TestHandleRunTrigger(t *testing.T) {
	srv := testServer(passCheck("a", "cat1"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/run", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["runId"] == "" {
		t.Error("expected non-empty runId")
	}
}

func TestHandleLatest_NoRuns(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/results/latest", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleLatest_AfterRun(t *testing.T) {
	srv := testServer(passCheck("a", "cat1"))

	// Trigger a run first.
	triggerReq := httptest.NewRequest(http.MethodPost, "/api/v1/run", nil)
	triggerRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(triggerRec, triggerReq)

	// Now fetch latest.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/results/latest", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var run RunResult
	if err := json.NewDecoder(rec.Body).Decode(&run); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if run.Status != StatusPass {
		t.Errorf("expected pass, got %s", run.Status)
	}
}

func TestHandleRunSSE_ContextCancellation(t *testing.T) {
	slowCheck := Check{
		Name:     "slow",
		Category: "cat1",
		Run: func(ctx context.Context) TestResult {
			<-ctx.Done()
			return TestResult{Status: StatusSkip, Detail: "cancelled"}
		},
	}

	srv := testServer(slowCheck)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := httptest.NewRequest(http.MethodGet, "/api/v1/run?stream=true", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// Should still return 200 with SSE headers even if no results streamed.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestNewServer(t *testing.T) {
	s := NewServer(staticRunnerBuilder(), ":9090", logr.Discard())
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.addr != ":9090" {
		t.Errorf("expected addr :9090, got %s", s.addr)
	}
}

// TestRunnerBuilderError covers the "builder failed" path on both
// handlers — a real failure mode in production (k8s API momentarily
// unreachable, transient DNS, etc). The handlers must surface the
// error rather than hang or silently produce empty results.
func TestRunnerBuilderError(t *testing.T) {
	failBuilder := func(_ context.Context) (*Runner, error) {
		return nil, errBuilderFail
	}
	srv := NewServer(failBuilder, ":0", logr.Discard())

	t.Run("trigger returns 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/run", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "builder fail") {
			t.Errorf("expected error in body, got %q", rec.Body.String())
		}
	})

	t.Run("SSE emits error event", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/run?stream=true", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("SSE pre-error status should be 200 (headers already sent), got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "event: error") {
			t.Errorf("expected SSE error event, got %q", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "builder fail") {
			t.Errorf("expected error message in event, got %q", rec.Body.String())
		}
	})
}

// errBuilderFail is a sentinel for TestRunnerBuilderError.
var errBuilderFail = newErr("builder fail")

func newErr(msg string) error { return &simpleErr{msg: msg} }

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

// TestRunnerBuilderInvokedPerRun is the regression test for issue
// #1040: Doctor must rebuild its runner on every /api/v1/run request
// so workspace service discovery re-runs and a startup-race against a
// not-yet-existing Workspace doesn't permanently cripple the pod.
func TestRunnerBuilderInvokedPerRun(t *testing.T) {
	calls := 0
	builder := func(_ context.Context) (*Runner, error) {
		calls++
		return NewRunner(), nil
	}
	srv := NewServer(builder, ":0", logr.Discard())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/run", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("run %d: expected 200, got %d (body: %s)", i, rec.Code, rec.Body.String())
		}
	}
	if calls != 3 {
		t.Errorf("expected builder to be invoked 3 times, got %d — runner is being cached and rediscovery won't happen", calls)
	}
}
