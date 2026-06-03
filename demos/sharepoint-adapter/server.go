package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

const (
	keyError        = "error"
	contentTypeJSON = "application/json"
)

// DocSource is the subset of GraphClient the server depends on (lets tests
// inject a fake without a live Graph).
type DocSource interface {
	List(ctx context.Context) ([]Doc, error)
	Fetch(ctx context.Context, url string) (*DocContent, error)
}

// Server exposes the adapter HTTP API.
type Server struct {
	src DocSource
	log *slog.Logger
}

func NewServer(src DocSource, log *slog.Logger) *Server {
	return &Server{src: src, log: log}
}

// Routes builds the adapter's HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/list", s.handleList)
	mux.HandleFunc("/fetch", s.handleFetch)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{keyError: "GET only"})
		return
	}
	docs, err := s.src.List(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"docs": docs})
}

type fetchRequest struct {
	URL string `json:"url"`
}

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{keyError: "POST only"})
		return
	}
	var req fetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{keyError: "missing or invalid url"})
		return
	}
	doc, err := s.src.Fetch(r.Context(), req.URL)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// writeUpstreamError passes a Graph status through (so a restricted-site 403
// surfaces to the agent as a denial), otherwise returns 502.
func (s *Server) writeUpstreamError(w http.ResponseWriter, err error) {
	var ge *GraphError
	if errors.As(err, &ge) {
		s.log.Warn("graph error", "status", ge.StatusCode)
		writeJSON(w, ge.StatusCode, map[string]string{keyError: ge.Body})
		return
	}
	s.log.Error("upstream error", "err", err.Error())
	writeJSON(w, http.StatusBadGateway, map[string]string{keyError: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
