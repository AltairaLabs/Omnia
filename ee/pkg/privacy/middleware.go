/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Drop reasons, used as the "reason" label on writesDropped and in the warning.
const (
	dropReasonRecordingDisabled = "recording-disabled"
	dropReasonRuntimeData       = "runtime-data-disabled"
	dropReasonUserOptedOut      = "user-opted-out"
	dropReasonRedactionFailed   = "redaction-failed"

	remediationRecording   = "recording.enabled is false in the effective SessionPrivacyPolicy"
	remediationRuntimeData = "recording.runtimeData is false; set runtimeData:true " +
		"(or attach a privacyPolicyRef) to record assistant message content"
)

// writesDropped counts session-api write requests dropped by the privacy
// middleware, labelled by reason. A non-zero rate means recorded data is being
// silently discarded — pair with the warning log to find the agent + policy.
var writesDropped = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "omnia_session_api_writes_dropped_total",
	Help: "Session-api write requests dropped by the privacy middleware, by reason.",
}, []string{"reason"})

// dropWarned dedupes the drop warning to once per namespace/agent + reason. A
// drop is a property of the agent's effective policy, not the session, so this
// stays bounded by the number of agents and avoids per-write log spam.
var dropWarned sync.Map // key: ns + "/" + agent + "|" + reason

// UserIDHeader is the HTTP header carrying the originating subject's
// pseudonymous virtual-user identity (not a raw user ID). The facade and
// runtime set this header on all write requests.
const UserIDHeader = "X-Omnia-User-ID"

// sessionIDPattern extracts the session ID from write endpoint paths.
var sessionIDPattern = regexp.MustCompile(`/api/v1/sessions/([^/]+)`)

// messageEndpointRe matches the only endpoint carrying conversation content.
// Metering (provider calls), tool calls, runtime events and eval results are
// recorded whenever recording is enabled — only message content is gated.
var messageEndpointRe = regexp.MustCompile(`/api/v1/sessions/[^/]+/messages$`)

// reportPolicyDrop increments the drop metric and emits the warning once per
// namespace/agent + reason, so a silently-dropped write is visible without
// flooding the log on every write of a session.
func (m *PrivacyMiddleware) reportPolicyDrop(sessionID, ns, agent, path, reason, remediation string) {
	writesDropped.WithLabelValues(reason).Inc()
	if _, loaded := dropWarned.LoadOrStore(ns+"/"+agent+"|"+reason, struct{}{}); loaded {
		return
	}
	m.log.Info("session write dropped by privacy policy",
		"namespace", ns, "agent", agent, "reason", reason,
		"endpoint", path, "sessionID", sessionID, "remediation", remediation)
}

// checkRecordingPolicy enforces recording for the write. Returns true if the
// request was blocked (response already written). Only message CONTENT is gated
// by runtimeData; provider-call metering, tool calls, runtime events and eval
// results are recorded whenever recording is enabled — they carry no
// conversation content.
func (m *PrivacyMiddleware) checkRecordingPolicy(
	w http.ResponseWriter, r *http.Request, policy *EffectivePolicy, ns, agent, sessionID string,
) bool {
	if !policy.Recording.Enabled {
		m.reportPolicyDrop(sessionID, ns, agent, r.URL.Path, dropReasonRecordingDisabled, remediationRecording)
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	// Non-message records are never gated by content flags.
	if !messageEndpointRe.MatchString(r.URL.Path) {
		return false
	}
	if m.messageContentAllowed(r, policy) {
		return false
	}
	m.reportPolicyDrop(sessionID, ns, agent, r.URL.Path, dropReasonRuntimeData, remediationRuntimeData)
	w.WriteHeader(http.StatusNoContent)
	return true
}

// messageContentAllowed reports whether a /messages write may be recorded.
// Facade-emitted content (user turns; the facade is Omnia-controlled) is always
// allowed; runtime-emitted content (assistant turns) requires runtimeData. When
// the writer doesn't set X-Omnia-Source (pre-source-header build), fall back to
// the legacy role heuristic so rollout is seamless.
func (m *PrivacyMiddleware) messageContentAllowed(r *http.Request, policy *EffectivePolicy) bool {
	switch r.Header.Get(session.SourceHeader) {
	case session.SourceFacade:
		return true
	case session.SourceRuntime:
		return policy.Recording.RuntimeData
	default:
		return !isRichMessage(r) || policy.Recording.RuntimeData
	}
}

// isRichMessage peeks at the request body to determine if a message is
// rich content. User messages are always allowed; assistant/system messages
// and tool call/result metadata types are rich content.
func isRichMessage(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return false
	}
	var msg struct {
		Role     string            `json:"role"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return false
	}
	if msg.Role == "user" {
		return false
	}
	if msg.Role == "assistant" || msg.Role == "system" {
		return true
	}
	t := msg.Metadata["type"]
	return t == "tool_call" || t == "tool_result"
}

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

		// Check recording policy (safety net for stale facade images).
		if m.checkRecordingPolicy(w, r, policy, ns, agent, sessionID) {
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
		writesDropped.WithLabelValues(dropReasonRedactionFailed).Inc()
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
		writesDropped.WithLabelValues(dropReasonUserOptedOut).Inc()
		m.log.V(1).Info("session write skipped: user opted out",
			"namespace", ns, "agent", agent, "sessionID", extractSessionID(r.URL.Path))
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
