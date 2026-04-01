package doctor

import (
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

// Server is the HTTP server for Omnia Doctor.
type Server struct {
	runner    *Runner
	addr      string
	log       logr.Logger
	latestRun *RunResult
	mu        sync.RWMutex
}

// NewServer creates a new doctor HTTP server.
func NewServer(runner *Runner, addr string, log logr.Logger) *Server {
	return &Server{
		runner: runner,
		addr:   addr,
		log:    log.WithName("server"),
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

	ch := make(chan TestResult, 64)
	ctx := r.Context()

	var run *RunResult
	done := make(chan struct{})
	go func() {
		run = s.runner.Run(ctx, ch)
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
	ch := make(chan TestResult, 64)
	// Drain results in background.
	go func() {
		for range ch { //nolint:revive // intentional drain
		}
	}()

	run := s.runner.Run(r.Context(), ch)
	s.storeRun(run)

	w.Header().Set(headerContentType, mimeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"runId": run.ID})
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
