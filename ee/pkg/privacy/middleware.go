/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"net/http"
	"regexp"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/session/api"
)

// UserIDHeader is the HTTP header carrying the originating user's identity.
// The facade and runtime set this header on all write requests.
const UserIDHeader = "X-Omnia-User-ID"

// sessionIDPattern extracts the session ID from write endpoint paths.
var sessionIDPattern = regexp.MustCompile(`/api/v1/sessions/([^/]+)`)

// PrivacyMiddleware intercepts session-api write requests and applies PII
// redaction and user opt-out according to the effective SessionPrivacyPolicy.
type PrivacyMiddleware struct {
	policyWatcher *PolicyWatcher
	sessionCache  *SessionMetadataCache
	redactor      redaction.Redactor
	prefStore     PreferencesStore
	auditLogger   api.AuditLogger
	log           logr.Logger
}

// NewPrivacyMiddleware creates middleware that enforces privacy policy on writes.
func NewPrivacyMiddleware(
	watcher *PolicyWatcher,
	sessionCache *SessionMetadataCache,
	redactor redaction.Redactor,
	prefStore PreferencesStore,
	log logr.Logger,
) *PrivacyMiddleware {
	return &PrivacyMiddleware{
		policyWatcher: watcher,
		sessionCache:  sessionCache,
		redactor:      redactor,
		prefStore:     prefStore,
		log:           log.WithName("privacy-middleware"),
	}
}

// SetAuditLogger configures an optional audit logger. When set, the middleware
// emits session_accessed events for GET requests and session_created events
// for write requests that are not blocked by opt-out or redaction failure.
func (m *PrivacyMiddleware) SetAuditLogger(logger api.AuditLogger) {
	m.auditLogger = logger
}

// Wrap returns an http.Handler that enforces privacy policy before delegating
// to the next handler.
func (m *PrivacyMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWriteMethod(r.Method) {
			m.emitAccessEvent(r)
			next.ServeHTTP(w, r)
			return
		}

		sessionID := extractSessionID(r.URL.Path)
		if sessionID == "" {
			next.ServeHTTP(w, r)
			return
		}

		ns, agent, err := m.sessionCache.Resolve(r.Context(), sessionID)
		if err != nil {
			// Can't resolve session metadata — pass through without redaction
			// to avoid blocking writes for unknown sessions (e.g., during creation).
			m.log.V(1).Info("session lookup failed", "sessionID", sessionID, "error", err.Error())
			next.ServeHTTP(w, r)
			return
		}

		policy := m.policyWatcher.GetEffectivePolicy(ns, agent)
		if policy == nil {
			m.emitWriteEvent(r, sessionID)
			next.ServeHTTP(w, r)
			return
		}

		// Check user opt-out.
		if err := m.checkOptOut(r, policy, ns, agent, w); err != nil {
			return // 204 already sent
		}

		if !m.applyRedaction(w, r, policy, sessionID) {
			return // 500 already sent
		}

		m.emitWriteEvent(r, sessionID)
		next.ServeHTTP(w, r)
	})
}

// emitAccessEvent emits a session_accessed audit event for GET requests.
// Only fires when the audit logger is set and the path contains a session ID.
func (m *PrivacyMiddleware) emitAccessEvent(r *http.Request) {
	if m.auditLogger == nil {
		return
	}
	sessionID := extractSessionID(r.URL.Path)
	if sessionID == "" {
		return
	}
	ctx := r.Context()
	entry := &api.AuditEntry{
		EventType: audit.EventSessionAccessed,
		SessionID: sessionID,
	}
	go m.auditLogger.LogEvent(ctx, entry)
}

// emitWriteEvent emits a session_created audit event for write requests.
// Only fires when the audit logger is set.
func (m *PrivacyMiddleware) emitWriteEvent(r *http.Request, sessionID string) {
	if m.auditLogger == nil {
		return
	}
	ctx := r.Context()
	entry := &api.AuditEntry{
		EventType: audit.EventSessionCreated,
		SessionID: sessionID,
	}
	go m.auditLogger.LogEvent(ctx, entry)
}

// isWriteMethod returns true for HTTP methods that carry a request body.
func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut
}

// applyRedaction redacts PII from the request body if the policy requires it.
// Returns true if the request should continue, false if it was blocked.
func (m *PrivacyMiddleware) applyRedaction(
	w http.ResponseWriter, r *http.Request, policy *EffectivePolicy, sessionID string,
) bool {
	if policy.Recording.PII == nil || !policy.Recording.PII.Redact {
		return true
	}
	redactedBody, err := redactRequestBody(
		r.Body, r.URL.Path, m.redactor, policy.Recording.PII,
	)
	if err != nil {
		m.log.Error(err, "body redaction failed, blocking request", "sessionID", sessionID)
		http.Error(w, "redaction failed", http.StatusInternalServerError)
		return false
	}
	r.Body = redactedBody
	return true
}

// checkOptOut checks whether the user has opted out. Returns a non-nil error
// sentinel (not a real error) when opt-out is active and 204 has been sent.
func (m *PrivacyMiddleware) checkOptOut(
	r *http.Request,
	policy *EffectivePolicy,
	ns, agent string,
	w http.ResponseWriter,
) error {
	if policy.UserOptOut == nil || !policy.UserOptOut.Enabled {
		return nil
	}

	userID := r.Header.Get(UserIDHeader)
	if userID == "" {
		return nil
	}

	if !ShouldRecord(r.Context(), m.prefStore, userID, ns, agent) {
		w.WriteHeader(http.StatusNoContent)
		return errOptedOut
	}
	return nil
}

// errOptedOut is a sentinel used internally by checkOptOut.
var errOptedOut = &optOutError{}

type optOutError struct{}

func (e *optOutError) Error() string { return "user opted out" }

// extractSessionID extracts the session ID from the URL path.
func extractSessionID(path string) string {
	matches := sessionIDPattern.FindStringSubmatch(path)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}
