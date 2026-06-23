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

// PolicyResolver returns the effective privacy policy JSON for a namespace/agent pair.
// Returns (policyJSON, true) when a policy applies, or (nil, false) when none applies.
// Using json.RawMessage keeps this package unaware of ee/ types.
type PolicyResolver interface {
	ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool)
}

// PolicyResolverFunc adapts a function to the PolicyResolver interface.
type PolicyResolverFunc func(namespace, agentName string) (json.RawMessage, bool)

// ResolveEffectivePolicy implements PolicyResolver.
func (f PolicyResolverFunc) ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool) {
	return f(namespace, agentName)
}

// Handler provides HTTP endpoints for session history.
type Handler struct {
	service              *SessionService
	evalService          *EvalService
	providerCallsService *ProviderCallsService
	providerUsageService *ProviderUsageService
	policyResolver       PolicyResolver
	encryptorResolver    EncryptorResolver
	log                  logr.Logger
	maxBodySize          int64
}

// NewHandler creates a new session API handler.
// An optional maxBodySize can be passed (first value used); defaults to
// DefaultMaxBodySize (16 MB).
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

// SetProviderCallsService configures the provider-calls service for
// /api/v1/provider-calls/* endpoints. When unset the endpoints return 503.
func (h *Handler) SetProviderCallsService(svc *ProviderCallsService) {
	h.providerCallsService = svc
}

// SetProviderUsageService configures the provider-usage service for
// POST /api/v1/provider-usage. When unset the endpoint returns 503.
func (h *Handler) SetProviderUsageService(svc *ProviderUsageService) {
	h.providerUsageService = svc
}

// SetPolicyResolver configures the resolver for GET /api/v1/privacy-policy.
// When unset, the endpoint returns 204 No Content (non-enterprise mode).
func (h *Handler) SetPolicyResolver(r PolicyResolver) {
	h.policyResolver = r
}

// SetEncryptorResolver configures per-session encryption. When nil (default),
// all sessions are stored in plaintext.
func (h *Handler) SetEncryptorResolver(r EncryptorResolver) {
	h.encryptorResolver = r
}

// EncryptorResolver returns the handler's encryption resolver for test introspection.
// Returns nil when no resolver has been configured.
func (h *Handler) EncryptorResolver() EncryptorResolver { return h.encryptorResolver }

// encryptorFor returns the Encryptor for sessionID, or nil if none applies.
func (h *Handler) encryptorFor(sessionID string) Encryptor {
	if h.encryptorResolver == nil {
		return nil
	}
	enc, ok := h.encryptorResolver.EncryptorForSession(sessionID)
	if !ok {
		return nil
	}
	return enc
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
	VirtualUserID     string            `json:"virtualUserId"`
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

// DecorateSessionRequest is the JSON body for PATCH /api/v1/sessions/{sessionID}/decorate.
// It merges additional tags and state into an existing session without touching
// counters or lifecycle status. RemoveTags are dropped before Tags are applied.
type DecorateSessionRequest struct {
	RemoveTags []string          `json:"removeTags,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	State      map[string]string `json:"state,omitempty"`
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
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionID}/decorate", h.handleDecorateSession)
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/ttl", h.handleRefreshTTL)
	mux.HandleFunc("DELETE /api/v1/sessions", h.handleBulkDeleteSessions)
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
	// Aggregate + discover: powers product dashboard views without Prometheus.
	// See CLAUDE.md → Observability Boundaries.
	mux.HandleFunc("GET /api/v1/eval-results/aggregate", h.handleAggregateEvalResults)
	mux.HandleFunc("GET /api/v1/eval-results/discover", h.handleDiscoverEvals)
	mux.HandleFunc("GET /api/v1/provider-calls/aggregate", h.handleAggregateProviderCalls)
	mux.HandleFunc("GET /api/v1/provider-calls/discover", h.handleDiscoverProviderCalls)

	// Provider usage endpoint: workspace-scoped, session-less spend (embeddings,
	// judge tokens). Written by memory-api + the eval worker.
	mux.HandleFunc("POST /api/v1/provider-usage", h.handleRecordProviderUsage)

	// Privacy policy endpoint
	mux.HandleFunc("GET /api/v1/privacy-policy", h.handleGetPrivacyPolicy)

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

// limitBody wraps the request body with a max bytes reader to prevent
// oversized payloads from consuming excessive memory.
func (h *Handler) limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
}

// servePaginatedDetail is a generic handler for paginated detail endpoints
// (tool calls, provider calls, runtime events). It extracts the session ID,
// parses pagination params, calls the service function, optionally transforms
// the result (e.g., for decryption), and writes the response.
// transform may be nil when no post-processing is needed.
func servePaginatedDetail[T any](
	h *Handler,
	w http.ResponseWriter,
	r *http.Request,
	opName string,
	fn func(context.Context, string, providers.PaginationOpts) (T, error),
	transform func(context.Context, string, T) (T, error),
) {
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

	if transform != nil {
		result, err = transform(r.Context(), sessionID, result)
		if err != nil {
			h.requestLog(r.Context()).Error(err, opName+" transform failed", "sessionID", sessionID)
			writeError(w, err)
			return
		}
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
	case errors.Is(err, ErrMissingVirtualUserID):
		status = http.StatusBadRequest
		msg = ErrMissingVirtualUserID.Error()
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
