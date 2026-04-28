package doctor

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-logr/logr"
)

//go:embed templates
var templateFS embed.FS

// HTTP header constants (SonarCloud S1192).
const (
	headerContentType = "Content-Type"
	mimeHTML          = "text/html; charset=utf-8"
	mimeJSON          = "application/json"
	mimeSSE           = "text/event-stream"
	mimePlain         = "text/plain"
)

// RunnerBuilder constructs a fresh Runner for a single check execution.
// Doctor calls this on every /api/v1/run invocation so workspace service
// discovery re-runs each time — without this, a Doctor pod that started
// before its workspace existed permanently uses stale fallback URLs (the
// failure mode behind issue #1040).
//
// Returning a fresh Runner per call is intentional: checks capture
// URLs / store handles by value at construction time, so a stale
// builder closure can't be patched after the fact. Callers that want
// the legacy "build once at startup" behaviour can ignore the ctx and
// return a memoised runner.
type RunnerBuilder func(ctx context.Context) (*Runner, error)

// Server is the HTTP server for Omnia Doctor.
type Server struct {
	build     RunnerBuilder
	addr      string
	log       logr.Logger
	latestRun *RunResult
	mu        sync.RWMutex
}

// NewServer creates a new doctor HTTP server. The builder is invoked
// per request so service discovery happens at run time, not pod start.
func NewServer(build RunnerBuilder, addr string, log logr.Logger) *Server {
	return &Server{
		build: build,
		addr:  addr,
		log:   log.WithName("server"),
	}
}

// Handler returns the configured http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/v1/run", s.handleRunSSE)
	mux.HandleFunc("POST /api/v1/run", s.handleRunTrigger)
	mux.HandleFunc("GET /api/v1/results/latest", s.handleLatest)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := templateFS.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, mimeHTML)
	_, _ = w.Write(data)
}

func (s *Server) handleRunSSE(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("stream") != "true" {
		http.Error(w, `query parameter stream=true is required`, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, mimeSSE)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	runner, err := s.build(ctx)
	if err != nil {
		s.log.Error(err, "build runner failed")
		// Headers are already set for SSE; emit an error event so the
		// client sees the failure instead of an empty stream.
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonOrFallback(map[string]string{"error": err.Error()}))
		flusher.Flush()
		return
	}

	ch := make(chan TestResult, 64)
	var run *RunResult
	done := make(chan struct{})
	go func() {
		run = runner.Run(ctx, ch)
		close(done)
	}()

	s.streamResults(w, flusher, ch)

	<-done

	s.storeRun(run)
	s.writeCompleteEvent(w, flusher, run)
}

func (s *Server) streamResults(w http.ResponseWriter, flusher http.Flusher, ch <-chan TestResult) {
	for result := range ch {
		data, err := json.Marshal(result)
		if err != nil {
			s.log.Error(err, "result marshal failed")
			continue
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (s *Server) writeCompleteEvent(w http.ResponseWriter, flusher http.Flusher, run *RunResult) {
	data, err := json.Marshal(run)
	if err != nil {
		s.log.Error(err, "run result marshal failed")
		return
	}
	_, _ = fmt.Fprintf(w, "event: complete\ndata: %s\n\n", data)
	flusher.Flush()
}

func (s *Server) handleRunTrigger(w http.ResponseWriter, r *http.Request) {
	runner, err := s.build(r.Context())
	if err != nil {
		s.log.Error(err, "build runner failed")
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	ch := make(chan TestResult, 64)
	// Drain results in background.
	go func() {
		for range ch { //nolint:revive // intentional drain
		}
	}()

	run := runner.Run(r.Context(), ch)
	s.storeRun(run)

	w.Header().Set(headerContentType, mimeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"runId": run.ID})
}

// jsonOrFallback marshals v to JSON; on error it returns a plain
// fallback so callers writing into an SSE stream always have a
// usable string.
func jsonOrFallback(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"marshal failed"}`
	}
	return string(b)
}

func (s *Server) handleLatest(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	run := s.latestRun
	s.mu.RUnlock()

	if run == nil {
		http.Error(w, `{"error":"no completed runs"}`, http.StatusNotFound)
		return
	}

	w.Header().Set(headerContentType, mimeJSON)
	_ = json.NewEncoder(w).Encode(run)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(headerContentType, mimePlain)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) storeRun(run *RunResult) {
	s.mu.Lock()
	s.latestRun = run
	s.mu.Unlock()
}
