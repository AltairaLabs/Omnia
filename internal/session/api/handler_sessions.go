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
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/google/uuid"
)

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
	enc := h.encryptorFor(sessionID)
	if enc != nil {
		for _, m := range msgPtrs {
			if derr := decryptMessage(enc, m); derr != nil {
				log.Error(derr, "DecryptMessage failed", "sessionID", sessionID)
				writeError(w, derr)
				return
			}
		}
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
	// virtualUserId is persisted as a NOT NULL column. Reject an empty value here
	// so the API returns a 400 instead of letting the database raise a 500.
	if req.VirtualUserID == "" {
		writeError(w, ErrMissingVirtualUserID)
		return
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
		VirtualUserID:     req.VirtualUserID,
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

// handleDecorateSession merges tags and state into an existing session without
// touching counters or lifecycle status. Used to label a facade-recorded session
// with arena context after the fact.
func (h *Handler) handleDecorateSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var req DecorateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.DecorateSession(r.Context(), sessionID, session.DecorateSessionOptions{
		RemoveTags: req.RemoveTags,
		AddTags:    req.Tags,
		MergeState: req.State,
	}); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "DecorateSession failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("session decorated", "sessionID", sessionID, "tags", req.Tags)
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

	// Require the namespace so the delete is scoped to the caller's workspace;
	// the service rejects a session in any other namespace as not-found.
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeError(w, ErrMissingNamespace)
		return
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	log := h.requestLog(r.Context())
	if err := h.service.DeleteSession(ctx, sessionID, namespace); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "DeleteSession failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(1).Info("session deleted", "sessionID", sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// handleBulkDeleteSessions deletes all sessions matching a namespace scope,
// with optional agent and before-cutoff filters. Required: ?namespace=.
// Returns {"deleted": <count>}. User-agnostic — removes any matching session.
//
// Visibility window (SEC-8): purged sessions remain readable by exact ID from
// the hot cache for up to DefaultCacheTTL (15m) after this returns, because the
// bulk delete does not proactively invalidate the cache. See
// SessionService.DeleteSessionsByScope for the compliance implication.
func (h *Handler) handleBulkDeleteSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namespace := q.Get("namespace")
	if namespace == "" {
		writeError(w, ErrMissingNamespace)
		return
	}
	scope := providers.SessionDeleteScope{
		Namespace: namespace,
		AgentName: q.Get("agent"),
	}
	if b := q.Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339, b)
		if err != nil {
			w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "before must be an RFC3339 timestamp"})
			return
		}
		scope.Before = t
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	log := h.requestLog(r.Context())
	n, err := h.service.DeleteSessionsByScope(ctx, scope)
	if err != nil {
		log.Error(err, "DeleteSessionsByScope failed", "namespace", namespace)
		writeError(w, err)
		return
	}

	log.V(1).Info("sessions purged", "namespace", namespace, "agent", scope.AgentName, "count", n)
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]int64{"deleted": n})
}

// handleGetPrivacyPolicy returns the effective privacy policy for a namespace/agent pair.
// Returns 204 No Content when no resolver is configured (non-enterprise) or no policy applies.
func (h *Handler) handleGetPrivacyPolicy(w http.ResponseWriter, r *http.Request) {
	if h.policyResolver == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ns := r.URL.Query().Get("namespace")
	agent := r.URL.Query().Get("agent")

	policyJSON, ok := h.policyResolver.ResolveEffectivePolicy(ns, agent)
	if !ok || len(policyJSON) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_, _ = w.Write(policyJSON)
}
