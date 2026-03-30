package doctor

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func testServer(checks ...Check) *Server {
	r := NewRunner()
	r.Register(checks...)
	return NewServer(r, ":0", logr.Discard())
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
	// 2 test results + 1 complete event = 3 data lines
	if dataLines != 3 {
		t.Errorf("expected 3 data lines, got %d; body:\n%s", dataLines, body)
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
	r := NewRunner()
	s := NewServer(r, ":9090", logr.Discard())
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.addr != ":9090" {
		t.Errorf("expected addr :9090, got %s", s.addr)
	}
}

func TestServer_ListenAndServe(t *testing.T) {
	srv := testServer()
	// Override to use a random port.
	s := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: srv.Handler(),
	}

	// Use a listener to get the actual port.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() { _ = s.Serve(ln) }()
	defer func() { _ = s.Close() }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected 'ok', got %q", string(body))
	}
}
