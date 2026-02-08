/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Handler constants.
const (
	contentTypeJSON   = "application/json"
	headerContentType = "Content-Type"

	defaultListLimit    = 20
	maxListLimit        = 100
	defaultMessageLimit = 50
)

// SessionListResponse is the JSON response for session list/search endpoints.
type SessionListResponse struct {
	Sessions []*session.Session `json:"sessions"`
	Total    int64              `json:"total"`
	HasMore  bool               `json:"hasMore"`
}

// SessionResponse is the JSON response for a single session.
type SessionResponse struct {
	Session  *session.Session  `json:"session"`
	Messages []session.Message `json:"messages,omitempty"`
}

// MessagesResponse is the JSON response for a messages query.
type MessagesResponse struct {
	Messages []*session.Message `json:"messages"`
	HasMore  bool               `json:"hasMore"`
}

// ErrorResponse is the JSON response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Handler provides HTTP endpoints for session history.
type Handler struct {
	service *SessionService
	log     logr.Logger
}

// NewHandler creates a new session API handler.
func NewHandler(service *SessionService, log logr.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log.WithName("session-handler"),
	}
}

// RegisterRoutes registers the session API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/sessions", h.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/search", h.handleSearchSessions)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}", h.handleGetSession)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/messages", h.handleGetMessages)
}

// handleListSessions returns a paginated list of sessions filtered by workspace.
func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	opts, err := parseListParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if opts.WorkspaceName == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	page, err := h.service.ListSessions(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "ListSessions failed")
		writeError(w, err)
		return
	}

	writeJSON(w, SessionListResponse{
		Sessions: page.Sessions,
		Total:    page.TotalCount,
		HasMore:  page.HasMore,
	})
}

// handleSearchSessions performs full-text search across sessions.
func (h *Handler) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, ErrMissingQuery)
		return
	}

	opts, err := parseListParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if opts.WorkspaceName == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	page, err := h.service.SearchSessions(r.Context(), q, opts)
	if err != nil {
		h.log.Error(err, "SearchSessions failed")
		writeError(w, err)
		return
	}

	writeJSON(w, SessionListResponse{
		Sessions: page.Sessions,
		Total:    page.TotalCount,
		HasMore:  page.HasMore,
	})
}

// handleGetSession returns a single session by ID including its messages.
func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		writeError(w, ErrMissingSessionID)
		return
	}

	sess, err := h.service.GetSession(r.Context(), sessionID)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			h.log.Error(err, "GetSession failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	writeJSON(w, SessionResponse{
		Session:  sess,
		Messages: sess.Messages,
	})
}

// handleGetMessages returns messages for a session with filtering.
func (h *Handler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		writeError(w, ErrMissingSessionID)
		return
	}

	limit := parseIntParam(r, "limit", defaultMessageLimit)
	before := int32(parseIntParam(r, "before", 0))
	after := int32(parseIntParam(r, "after", 0))

	opts := providers.MessageQueryOpts{
		Limit:     limit + 1, // fetch one extra to determine hasMore
		BeforeSeq: before,
		AfterSeq:  after,
	}

	msgs, err := h.service.GetMessages(r.Context(), sessionID, opts)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			h.log.Error(err, "GetMessages failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	writeJSON(w, MessagesResponse{
		Messages: msgs,
		HasMore:  hasMore,
	})
}

// parseListParams extracts common list/search query parameters from the request.
func parseListParams(r *http.Request) (providers.SessionListOpts, error) {
	q := r.URL.Query()

	limit := min(parseIntParam(r, "limit", defaultListLimit), maxListLimit)

	opts := providers.SessionListOpts{
		Limit:         limit,
		Offset:        parseIntParam(r, "offset", 0),
		WorkspaceName: q.Get("workspace"),
		AgentName:     q.Get("agent"),
		Namespace:     q.Get("namespace"),
	}

	if status := q.Get("status"); status != "" {
		opts.Status = session.SessionStatus(status)
	}

	if from := q.Get("from"); from != "" {
		t, err := parseTimeParam(from)
		if err != nil {
			return opts, err
		}
		opts.CreatedAfter = t
	}

	if to := q.Get("to"); to != "" {
		t, err := parseTimeParam(to)
		if err != nil {
			return opts, err
		}
		opts.CreatedBefore = t
	}

	return opts, nil
}

// parseIntParam returns an integer query parameter or the default value.
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// parseTimeParam parses a time string in RFC3339 format.
func parseTimeParam(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// writeJSON writes a JSON 200 OK response.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set(headerContentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Response is already partially written; log but don't try to write another error.
		_ = err
	}
}

// writeError maps known errors to HTTP status codes and writes a JSON error response.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal server error"

	switch {
	case errors.Is(err, session.ErrSessionNotFound):
		status = http.StatusNotFound
		msg = "session not found"
	case errors.Is(err, ErrWarmStoreRequired):
		status = http.StatusServiceUnavailable
		msg = "warm store not configured"
	case errors.Is(err, ErrMissingWorkspace):
		status = http.StatusBadRequest
		msg = ErrMissingWorkspace.Error()
	case errors.Is(err, ErrMissingQuery):
		status = http.StatusBadRequest
		msg = ErrMissingQuery.Error()
	case errors.Is(err, ErrMissingSessionID):
		status = http.StatusBadRequest
		msg = ErrMissingSessionID.Error()
	default:
		var timeErr *time.ParseError
		if errors.As(err, &timeErr) {
			status = http.StatusBadRequest
			msg = "invalid time format, expected RFC3339"
		}
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}
