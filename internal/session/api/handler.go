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
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// Handler constants.
const (
	defaultListLimit    = 20
	maxListLimit        = 100
	defaultMessageLimit = 50
	maxMessageLimit     = 500
	defaultDetailLimit  = 100
	maxDetailLimit      = 500
	maxStringParamLen   = 253 // K8s name limit
	maxSearchQueryLen   = 500
	maxOffsetLimit      = 10000

	// DefaultMaxBodySize is the maximum allowed request body size (16 MB).
	// Aligned with WebSocket and gRPC max message sizes across the pipeline.
	DefaultMaxBodySize int64 = 16 << 20
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
	service     *SessionService
	evalService *EvalService
	log         logr.Logger
	maxBodySize int64
}

// NewHandler creates a new session API handler.
// An optional maxBodySize can be passed (first value used); defaults to 10 MB.
func NewHandler(service *SessionService, log logr.Logger, maxBodySize ...int64) *Handler {
	mbs := DefaultMaxBodySize
	if len(maxBodySize) > 0 && maxBodySize[0] > 0 {
		mbs = maxBodySize[0]
	}
	return &Handler{
		service:     service,
		log:         log.WithName("session-handler"),
		maxBodySize: mbs,
	}
}

// SetEvalService configures the eval service for eval result endpoints.
func (h *Handler) SetEvalService(svc *EvalService) {
	h.evalService = svc
}

// TraceLogMiddleware extracts the OTel trace ID from the request's span context
// and injects it into logctx so that downstream handlers can produce logs
// correlated with the active trace (trace_id field in JSON output).
func TraceLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			ctx = logctx.WithTraceID(ctx, sc.TraceID().String())
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// requestLog returns a logger enriched with trace and request context from ctx.
func (h *Handler) requestLog(ctx context.Context) logr.Logger {
	return logctx.LoggerWithContext(h.log, ctx)
}

// Request types for write endpoints.

// CreateSessionRequest is the JSON body for POST /api/v1/sessions.
type CreateSessionRequest struct {
	ID                string            `json:"id"`
	AgentName         string            `json:"agentName"`
	Namespace         string            `json:"namespace"`
	WorkspaceName     string            `json:"workspaceName,omitempty"`
	TTLSeconds        int               `json:"ttlSeconds,omitempty"`
	PromptPackName    string            `json:"promptPackName,omitempty"`
	PromptPackVersion string            `json:"promptPackVersion,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	InitialState      map[string]string `json:"initialState,omitempty"`
	CohortID          string            `json:"cohortId,omitempty"`
	Variant           string            `json:"variant,omitempty"`
}

// AppendMessageRequest is the JSON body for POST /api/v1/sessions/{sessionID}/messages.
type AppendMessageRequest struct {
	session.Message
}

// UpdateStatsRequest is the JSON body for PATCH /api/v1/sessions/{sessionID}/stats.
type UpdateStatsRequest struct {
	session.SessionStatusUpdate
}

// RefreshTTLRequest is the JSON body for POST /api/v1/sessions/{sessionID}/ttl.
type RefreshTTLRequest struct {
	TTLSeconds int `json:"ttlSeconds"`
}

// RegisterRoutes registers the session API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Health check (lightweight, no DB call)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Read endpoints
	mux.HandleFunc("GET /api/v1/sessions", h.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/search", h.handleSearchSessions)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}", h.handleGetSession)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/messages", h.handleGetMessages)

	// Write endpoints
	mux.HandleFunc("POST /api/v1/sessions", h.handleCreateSession)
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/messages", h.handleAppendMessage)
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionID}/status", h.handleUpdateStats)
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionID}/stats", h.handleUpdateStats) // backward-compat alias
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/ttl", h.handleRefreshTTL)
	mux.HandleFunc("DELETE /api/v1/sessions/{sessionID}", h.handleDeleteSession)

	// Tool call endpoints
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/tool-calls", h.handleRecordToolCall)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/tool-calls", h.handleGetToolCalls)

	// Provider call endpoints
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/provider-calls", h.handleRecordProviderCall)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/provider-calls", h.handleGetProviderCalls)

	// Runtime event endpoints
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/events", h.handleRecordRuntimeEvent)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/events", h.handleGetRuntimeEvents)

	// Eval result endpoints
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/eval-results/summary", h.handleGetEvalResultSummary)
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/eval-results", h.handleGetSessionEvalResults)
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/evaluate", h.handleEvaluateSession)
	mux.HandleFunc("POST /api/v1/eval-results", h.handleCreateEvalResults)
	mux.HandleFunc("GET /api/v1/eval-results", h.handleListEvalResults)

	// API documentation
	h.registerDocsRoutes(mux)
}

// extractRequestContext extracts client IP and User-Agent from the request.
func extractRequestContext(r *http.Request) RequestContext {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For may contain multiple IPs; take the first (client).
		if idx := strings.IndexByte(ip, ','); idx != -1 {
			ip = strings.TrimSpace(ip[:idx])
		}
	} else {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return RequestContext{
		IPAddress: ip,
		UserAgent: r.Header.Get("User-Agent"),
	}
}

// sessionIDFromRequest extracts and validates the session ID path parameter.
// Returns the session ID or an error if missing or not a valid UUID.
func sessionIDFromRequest(r *http.Request) (string, error) {
	id := r.PathValue("sessionID")
	if id == "" {
		return "", ErrMissingSessionID
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", ErrInvalidSessionID
	}
	return id, nil
}

// truncateParam silently truncates s to maxLen if it exceeds the limit.
func truncateParam(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// validSessionStatus returns true if s is one of the known session status values.
func validSessionStatus(s session.SessionStatus) bool {
	switch s {
	case session.SessionStatusActive,
		session.SessionStatusCompleted,
		session.SessionStatusError,
		session.SessionStatusExpired:
		return true
	}
	return false
}

// handleListSessions returns a paginated list of sessions filtered by workspace.
func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	opts, err := parseListParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if opts.Namespace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	page, err := h.service.ListSessions(ctx, opts)
	if err != nil {
		h.requestLog(r.Context()).Error(err, "ListSessions failed")
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
	if len(q) > maxSearchQueryLen {
		writeError(w, ErrSearchQueryTooLong)
		return
	}

	opts, err := parseListParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if opts.Namespace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	page, err := h.service.SearchSessions(ctx, q, opts)
	if err != nil {
		h.requestLog(r.Context()).Error(err, "SearchSessions failed")
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
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	log := h.requestLog(r.Context())
	sess, err := h.service.GetSession(ctx, sessionID)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "GetSession failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	// Fetch messages separately — GetSession only returns session metadata.
	msgPtrs, err := h.service.GetMessages(ctx, sessionID, providers.MessageQueryOpts{
		Limit: defaultMessageLimit,
	})
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		log.Error(err, "GetMessages failed", "sessionID", sessionID)
		writeError(w, err)
		return
	}
	msgs := make([]session.Message, 0, len(msgPtrs))
	for _, m := range msgPtrs {
		msgs = append(msgs, *m)
	}

	writeJSON(w, SessionResponse{
		Session:  sess,
		Messages: msgs,
	})
}

// handleGetMessages returns messages for a session with filtering.
func (h *Handler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	limit := min(parseIntParam(r, "limit", defaultMessageLimit), maxMessageLimit)
	before := int32(parseIntParam(r, "before", 0))
	after := int32(parseIntParam(r, "after", 0))

	opts := providers.MessageQueryOpts{
		Limit:     limit + 1, // fetch one extra to determine hasMore
		BeforeSeq: before,
		AfterSeq:  after,
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	msgs, err := h.service.GetMessages(ctx, sessionID, opts)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			h.requestLog(r.Context()).Error(err, "GetMessages failed", "sessionID", sessionID)
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

// limitBody wraps the request body with a max bytes reader to prevent
// oversized payloads from consuming excessive memory.
func (h *Handler) limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
}

// handleCreateSession creates a new session.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	h.limitBody(w, r)
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	if req.AgentName == "" {
		writeError(w, ErrMissingAgentName)
		return
	}
	if req.Namespace == "" {
		writeError(w, ErrMissingNamespace)
		return
	}
	if req.WorkspaceName == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	if req.ID != "" {
		if _, err := uuid.Parse(req.ID); err != nil {
			writeError(w, ErrInvalidSessionID)
			return
		}
	}

	now := time.Now()
	sess := &session.Session{
		ID:                req.ID,
		AgentName:         req.AgentName,
		Namespace:         req.Namespace,
		WorkspaceName:     req.WorkspaceName,
		PromptPackName:    req.PromptPackName,
		PromptPackVersion: req.PromptPackVersion,
		Tags:              req.Tags,
		State:             req.InitialState,
		Status:            session.SessionStatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
		CohortID:          req.CohortID,
		Variant:           req.Variant,
	}
	if req.TTLSeconds > 0 {
		sess.ExpiresAt = now.Add(time.Duration(req.TTLSeconds) * time.Second)
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	log := h.requestLog(r.Context())
	if err := h.service.CreateSession(ctx, sess); err != nil {
		log.Error(err, "CreateSession failed")
		writeError(w, err)
		return
	}

	log.V(1).Info("session created", "sessionID", sess.ID, "agent", req.AgentName, "namespace", req.Namespace)
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SessionResponse{Session: sess})
}

// handleAppendMessage appends a message to a session.
func (h *Handler) handleAppendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var msg session.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.AppendMessage(r.Context(), sessionID, &msg); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "AppendMessage failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("message appended", "sessionID", sessionID, "role", msg.Role)
	w.WriteHeader(http.StatusCreated)
}

// handleUpdateStats applies incremental counter updates to a session.
func (h *Handler) handleUpdateStats(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var update session.SessionStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.UpdateSessionStatus(r.Context(), sessionID, update); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "UpdateSessionStatus failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("session status updated", "sessionID", sessionID)
	w.WriteHeader(http.StatusOK)
}

// handleRefreshTTL extends the expiry of a session.
func (h *Handler) handleRefreshTTL(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var req RefreshTTLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if err := h.service.RefreshTTL(r.Context(), sessionID, ttl); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RefreshTTL failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("session TTL refreshed", "sessionID", sessionID, "ttlSeconds", req.TTLSeconds)
	w.WriteHeader(http.StatusOK)
}

// handleDeleteSession deletes a session by ID.
func (h *Handler) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	log := h.requestLog(r.Context())
	if err := h.service.DeleteSession(ctx, sessionID); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "DeleteSession failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(1).Info("session deleted", "sessionID", sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// handleRecordToolCall records a tool call for a session.
func (h *Handler) handleRecordToolCall(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var tc session.ToolCall
	if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.RecordToolCall(r.Context(), sessionID, &tc); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordToolCall failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("tool call recorded", "sessionID", sessionID, "toolName", tc.Name)
	w.WriteHeader(http.StatusCreated)
}

// handleGetToolCalls returns tool calls for a session with pagination.
func (h *Handler) handleGetToolCalls(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail(h, w, r, "GetToolCalls", h.service.GetToolCalls)
}

// handleRecordProviderCall records a provider call for a session.
func (h *Handler) handleRecordProviderCall(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var pc session.ProviderCall
	if err := json.NewDecoder(r.Body).Decode(&pc); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.RecordProviderCall(r.Context(), sessionID, &pc); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordProviderCall failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("provider call recorded", "sessionID", sessionID, "provider", pc.Provider)
	w.WriteHeader(http.StatusCreated)
}

// handleGetProviderCalls returns provider calls for a session with pagination.
func (h *Handler) handleGetProviderCalls(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail(h, w, r, "GetProviderCalls", h.service.GetProviderCalls)
}

// handleRecordRuntimeEvent records a runtime event for a session.
func (h *Handler) handleRecordRuntimeEvent(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var evt session.RuntimeEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.RecordRuntimeEvent(r.Context(), sessionID, &evt); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordRuntimeEvent failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("runtime event recorded", "sessionID", sessionID, "eventType", evt.EventType)
	w.WriteHeader(http.StatusCreated)
}

// handleGetRuntimeEvents returns runtime events for a session with pagination.
func (h *Handler) handleGetRuntimeEvents(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail(h, w, r, "GetRuntimeEvents", h.service.GetRuntimeEvents)
}

// servePaginatedDetail is a generic handler for paginated detail endpoints
// (tool calls, provider calls, runtime events). It extracts the session ID,
// parses pagination params, calls the service function, and writes the result.
func servePaginatedDetail[T any](h *Handler, w http.ResponseWriter, r *http.Request, opName string, fn func(context.Context, string, providers.PaginationOpts) (T, error)) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	opts := parseDetailPagination(r)
	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	result, err := fn(ctx, sessionID, opts)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			h.requestLog(r.Context()).Error(err, opName+" failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	writeJSON(w, result)
}

// parseListParams extracts common list/search query parameters from the request.
func parseListParams(r *http.Request) (providers.SessionListOpts, error) {
	q := r.URL.Query()

	limit := min(parseIntParam(r, "limit", defaultListLimit), maxListLimit)
	offset := min(parseIntParam(r, "offset", 0), maxOffsetLimit)

	// "namespace" scopes by k8s namespace, "workspace" scopes by workspace name.
	// Both are sent by the dashboard proxy.
	opts := providers.SessionListOpts{
		Limit:         limit,
		Offset:        offset,
		Namespace:     truncateParam(q.Get("namespace"), maxStringParamLen),
		WorkspaceName: truncateParam(q.Get("workspace"), maxStringParamLen),
		AgentName:     truncateParam(q.Get("agent"), maxStringParamLen),
		IncludeCount:  q.Get("count") == "true",
	}

	if status := q.Get("status"); status != "" {
		s := session.SessionStatus(status)
		if !validSessionStatus(s) {
			return opts, ErrInvalidStatus
		}
		opts.Status = s
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

// parseDetailPagination extracts limit/offset query params for detail endpoints
// (tool calls, provider calls, runtime events). Default limit is 100, max 500.
func parseDetailPagination(r *http.Request) providers.PaginationOpts {
	limit := min(parseIntParam(r, "limit", defaultDetailLimit), maxDetailLimit)
	offset := min(parseIntParam(r, "offset", 0), maxOffsetLimit)
	return providers.PaginationOpts{
		Limit:  limit,
		Offset: offset,
	}
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
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
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
	case errors.Is(err, ErrInvalidSessionID):
		status = http.StatusBadRequest
		msg = ErrInvalidSessionID.Error()
	case errors.Is(err, ErrMissingBody):
		status = http.StatusBadRequest
		msg = ErrMissingBody.Error()
	case errors.Is(err, ErrMissingAgentName):
		status = http.StatusBadRequest
		msg = ErrMissingAgentName.Error()
	case errors.Is(err, ErrMissingNamespace):
		status = http.StatusBadRequest
		msg = ErrMissingNamespace.Error()
	case errors.Is(err, ErrInvalidStatus):
		status = http.StatusBadRequest
		msg = ErrInvalidStatus.Error()
	case errors.Is(err, ErrSearchQueryTooLong):
		status = http.StatusBadRequest
		msg = ErrSearchQueryTooLong.Error()
	case errors.Is(err, ErrRateLimitExceeded):
		status = http.StatusTooManyRequests
		msg = ErrRateLimitExceeded.Error()
	case errors.Is(err, ErrBodyTooLarge) || isMaxBytesError(err):
		status = http.StatusRequestEntityTooLarge
		msg = ErrBodyTooLarge.Error()
	default:
		var timeErr *time.ParseError
		if errors.As(err, &timeErr) {
			status = http.StatusBadRequest
			msg = "invalid time format, expected RFC3339"
		}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

// isMaxBytesError checks if the error is an http.MaxBytesError from MaxBytesReader.
func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
