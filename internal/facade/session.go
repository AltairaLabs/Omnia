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

package facade

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	"github.com/altairalabs/omnia/internal/session/otlp"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/policy"
)

// captureSessionConsentGrants stamps a non-empty msg.SessionConsentGrants
// onto the connection. Last-writer-wins: a subsequent non-empty list
// replaces the previously-cached value. Empty / omitted lists are ignored —
// they do NOT clear the cache (use the binary opt-out instead). Copies the
// slice so subsequent client mutation can't corrupt the cached value.
func captureSessionConsentGrants(c *Connection, msg *ClientMessage) {
	if len(msg.SessionConsentGrants) == 0 {
		return
	}
	grants := append([]string{}, msg.SessionConsentGrants...)
	c.mu.Lock()
	c.sessionConsentGrants = grants
	c.mu.Unlock()
}

// effectiveConsentGrants returns the consent grants and layer label to
// attach to the runtime call. Last-writer-wins: per-message overrides
// session; session overrides persistent. nil grants + "persistent"
// means memory-api falls back to its persistent store.
func effectiveConsentGrants(c *Connection, msg *ClientMessage) ([]string, string) {
	if len(msg.ConsentGrants) > 0 {
		return msg.ConsentGrants, "per-message"
	}
	c.mu.Lock()
	cached := c.sessionConsentGrants
	c.mu.Unlock()
	if len(cached) > 0 {
		return cached, "session"
	}
	return nil, "persistent"
}

// processMessage handles processing of an incoming client message.
func (s *Server) processMessage(ctx context.Context, c *Connection, msg *ClientMessage, log logr.Logger) error {
	// Get or create session first — the session ID determines the trace ID.
	sessionID, err := s.ensureSession(ctx, c, msg.SessionID, log)
	if err != nil {
		s.sendError(c, msg.SessionID, ErrorCodeInternalError, "failed to create session")
		return err
	}

	// Start the message span using the session ID as the trace ID.
	// This makes every message in a session part of the same trace,
	// and allows direct trace lookup by session ID without indexing.
	var msgSpan trace.Span
	ctx, msgSpan = s.startMessageSpan(ctx, c, sessionID)
	defer msgSpan.End()

	// Enrich context with session ID, namespace, trace ID, and user ID for
	// log↔trace correlation and privacy header propagation.
	ctx = logctx.WithSessionID(ctx, sessionID)
	ctx = logctx.WithNamespace(ctx, c.namespace)
	ctx = logctx.WithTraceID(ctx, msgSpan.SpanContext().TraceID().String())
	if c.userID != "" {
		ctx = httpclient.WithUserID(ctx, c.userID)
		ctx = policy.WithUserID(ctx, c.userID)
	}
	captureSessionConsentGrants(c, msg)
	effective, layer := effectiveConsentGrants(c, msg)
	if effective != nil {
		ctx = policy.WithConsentGrants(ctx, effective)
	}
	ctx = policy.WithConsentLayer(ctx, layer)
	log = logctx.LoggerWithContext(s.log, ctx)

	// Update connection's session ID and mark as persisted
	c.mu.Lock()
	c.sessionID = sessionID
	c.sessionPersisted = true
	c.mu.Unlock()

	// Send connected message if this is a new session
	if msg.SessionID == "" {
		if err := s.sendConnected(c, sessionID, false); err != nil {
			return err
		}
	}

	// Create response writer (needed for all message types)
	writer := &connResponseWriter{
		conn:      c,
		sessionID: sessionID,
		server:    s,
	}

	// Handle upload_request messages separately
	var processErr error
	if msg.Type == MessageTypeUploadRequest {
		processErr = s.handleUploadRequest(ctx, sessionID, msg, writer, log)
	} else {
		processErr = s.processRegularMessage(ctx, c, sessionID, msg, writer, log)
	}

	if processErr != nil {
		tracing.RecordError(msgSpan, processErr)
	} else {
		tracing.SetSuccess(msgSpan)
	}
	return processErr
}

// startMessageSpan starts a tracing span for the message if tracing is enabled.
// It always derives the trace ID from the session ID (UUID → 128-bit trace ID)
// so that all messages in a session share the same trace, enabling direct Tempo
// lookup by session ID without search indexing. Downstream spans (runtime,
// session-api, eval-worker) inherit this trace ID, keeping evals nested under
// the session that originated them.
//
// When a W3C traceparent was present on the WebSocket upgrade request (e.g. from
// arena-worker), the caller's span context is added as a span link for
// cross-referencing in Tempo, but the session-derived trace ID remains primary.
func (s *Server) startMessageSpan(ctx context.Context, c *Connection, sessionID string) (context.Context, trace.Span) {
	if s.tracingProvider == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	sessionTraceID := sessionIDToTraceID(sessionID)
	var opts []trace.SpanStartOption

	// If a caller (e.g. arena-worker) injected a W3C traceparent, add it as
	// a span link so the two traces can be cross-referenced in Tempo.
	// The session-derived trace ID stays primary so that every span in the
	// session (facade → runtime → session-api → eval-worker) shares one trace.
	if parentSC := trace.SpanContextFromContext(ctx); parentSC.IsValid() {
		callerLink := trace.Link{
			SpanContext: parentSC,
			Attributes: []attribute.KeyValue{
				attribute.String("link.type", "caller-trace"),
			},
		}
		opts = append(opts, trace.WithLinks(callerLink))
	}

	// Always use the session-derived trace ID as the primary trace.
	remoteCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    sessionTraceID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, remoteCtx)

	spanAttrs := []attribute.KeyValue{
		attribute.String("session.id", sessionID),
		attribute.String(otlp.AttrOmniaAgentName, c.agentName),
		attribute.String(otlp.AttrOmniaAgentNamespace, c.namespace),
		attribute.String(otlp.AttrOmniaPromptPackName, s.config.PromptPackName),
		attribute.String(otlp.AttrOmniaPromptPackVersion, s.config.PromptPackVersion),
		attribute.String(otlp.AttrOmniaPromptPackNamespace, c.namespace),
	}
	if c.cohortID != "" {
		spanAttrs = append(spanAttrs, attribute.String(otlp.AttrOmniaCohortID, c.cohortID))
	}
	if c.variant != "" {
		spanAttrs = append(spanAttrs, attribute.String(otlp.AttrOmniaVariant, c.variant))
	}

	opts = append(opts,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(spanAttrs...),
	)

	return s.tracingProvider.Tracer().Start(ctx, "omnia.facade.message", opts...)
}

// sessionIDToTraceID converts a UUID session ID to an OpenTelemetry trace ID.
// A UUID is 128 bits — the same size as a trace ID — so the mapping is lossless.
func sessionIDToTraceID(sessionID string) trace.TraceID {
	cleaned := strings.ReplaceAll(sessionID, "-", "")
	var tid trace.TraceID
	_, _ = hex.Decode(tid[:], []byte(cleaned))
	return tid
}

// processRegularMessage dispatches to the handler. The conversation (user turn
// + assistant turn) is recorded by the RuntimeClient bus interceptor off the
// gRPC bus, so it is recorded once, protocol- and runtime-agnostically — no
// per-protocol recording here.
func (s *Server) processRegularMessage(ctx context.Context, c *Connection, sessionID string, msg *ClientMessage, writer *connResponseWriter, log logr.Logger) error {
	if s.handler != nil {
		if err := safeHandleMessage(s.handler, ctx, sessionID, msg, writer, log); err != nil {
			s.sendError(c, sessionID, ErrorCodeInternalError, "internal server error")
			return err
		}
	} else {
		// Default echo behavior if no handler
		if err := writer.WriteDone("Handler not configured"); err != nil {
			return err
		}
	}

	return nil
}

// ensureSession gets an existing session or creates a new one.
// When sessionID is non-empty, the store is checked first; if not found the
// session is created with that same ID. This supports deferred persistence:
// handleConnection generates a UUID on connect (for the "connected" message)
// but does not persist — the first processMessage call triggers the actual write.
func (s *Server) ensureSession(ctx context.Context, c *Connection, sessionID string, log logr.Logger) (string, error) {
	if sessionID != "" {
		// Try to resume existing session
		sess, err := s.sessionStore.GetSession(ctx, sessionID)
		if err == nil {
			s.metrics.SessionCreated()
			// Refresh TTL
			if err := s.sessionStore.RefreshTTL(ctx, sessionID, s.config.SessionTTL); err != nil {
				log.Error(err, "failed to refresh session TTL")
			}
			return sess.ID, nil
		}
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "failed to resume session", "sessionID", sessionID)
			return "", err
		}
		// Session not found or expired — create with the requested ID so the
		// client-visible session ID stays stable.
		log.V(1).Info("session not found, creating", "sessionID", sessionID)
	}

	// virtual_user_id is a NOT-NULL column and session-api 400s an empty
	// value (design principle: "no sessions without a user"). c.userID is the
	// per-user pseudonym resolved at upgrade (ResolveUserPseudonym). For a
	// truly anonymous connection (no auth identity, no device_id, no Istio
	// header) c.userID is empty, so fall back to a per-connection anonymous
	// pseudonym derived from this session's id — each anonymous session
	// becomes its own deterministic virtual user.
	fallbackSeed := sessionID
	if fallbackSeed == "" {
		fallbackSeed = c.SessionID()
	}
	virtualUserID := virtualUserIDForSession(c.userID, fallbackSeed)

	// Create new session, preserving the requested ID when provided.
	sess, err := s.sessionStore.CreateSession(ctx, session.CreateSessionOptions{
		ID:                sessionID,
		AgentName:         c.agentName,
		Namespace:         c.namespace,
		WorkspaceName:     c.workspaceName,
		TTL:               s.config.SessionTTL,
		PromptPackName:    s.config.PromptPackName,
		PromptPackVersion: s.config.PromptPackVersion,
		Tags:              buildSessionTags(c),
		InitialState:      buildSessionState(c, s.config),
		CohortID:          c.cohortID,
		Variant:           c.variant,
		VirtualUserID:     virtualUserID,
	})
	if err != nil {
		return "", err
	}

	s.metrics.SessionCreated()

	return sess.ID, nil
}

// virtualUserIDForSession returns the non-empty virtual_user_id to persist on
// a new session. It prefers the resolved per-user pseudonym; when that is empty
// (a truly anonymous connection) it falls back to a per-connection anonymous
// pseudonym derived from the session id, so the NOT-NULL create still succeeds
// and each anonymous session is its own deterministic virtual user.
func virtualUserIDForSession(userID, sessionID string) string {
	if userID != "" {
		return userID
	}
	return identity.PseudonymizeID(sessionID)
}

// buildSessionTags creates tags for a new interactive session.
func buildSessionTags(c *Connection) []string {
	tags := []string{"source:interactive"}
	if c.userID != "" {
		tags = append(tags, "user:"+c.userID)
	}
	return tags
}

// buildSessionState creates initial state metadata for a new interactive session.
func buildSessionState(c *Connection, cfg ServerConfig) map[string]string {
	state := make(map[string]string)
	if c.userID != "" {
		state["user.id"] = c.userID
	}
	if c.userRoles != "" {
		state["user.roles"] = c.userRoles
	}
	if cfg.PromptPackName != "" {
		state["promptpack.name"] = cfg.PromptPackName
	}
	if cfg.PromptPackVersion != "" {
		state["promptpack.version"] = cfg.PromptPackVersion
	}
	return state
}

// handleUploadRequest processes an upload_request message from the client.
func (s *Server) handleUploadRequest(ctx context.Context, sessionID string, msg *ClientMessage, writer *connResponseWriter, log logr.Logger) error {
	// Check if media storage is enabled
	if s.mediaStorage == nil {
		log.Info("upload_request received but media storage not enabled")
		return writer.WriteError(ErrorCodeMediaNotEnabled, "media storage is not enabled")
	}

	// Validate the upload request
	if msg.UploadRequest == nil {
		log.Info("upload_request missing upload_request field")
		return writer.WriteError(ErrorCodeInvalidMessage, "upload_request field is required")
	}

	req := msg.UploadRequest
	if req.Filename == "" {
		return writer.WriteError(ErrorCodeInvalidMessage, "filename is required")
	}
	if req.MimeType == "" {
		return writer.WriteError(ErrorCodeInvalidMessage, "mime_type is required")
	}
	if req.SizeBytes <= 0 {
		return writer.WriteError(ErrorCodeInvalidMessage, "size_bytes must be positive")
	}

	// Request upload URL from storage
	creds, err := s.mediaStorage.GetUploadURL(ctx, media.UploadRequest{
		SessionID: sessionID,
		Filename:  req.Filename,
		MIMEType:  req.MimeType,
		SizeBytes: req.SizeBytes,
	})
	if err != nil {
		log.Error(err, "failed to get upload URL", "filename", req.Filename)
		return writer.WriteError(ErrorCodeUploadFailed, "failed to prepare upload")
	}

	// Send upload_ready response
	log.Info("upload ready", "uploadID", creds.UploadID, "storageRef", creds.StorageRef)
	return writer.WriteUploadReady(&UploadReadyInfo{
		UploadID:   creds.UploadID,
		UploadURL:  creds.URL,
		StorageRef: creds.StorageRef,
		ExpiresAt:  creds.ExpiresAt,
	})
}

// safeHandleMessage wraps handler.HandleMessage with panic recovery to prevent
// a panic in the handler from crashing the connection goroutine.
func safeHandleMessage(handler MessageHandler, ctx context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter, log logr.Logger) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Error(fmt.Errorf("panic: %v", r), "handler panic recovered",
				"sessionID", sessionID,
				"stack", string(stack))
			retErr = fmt.Errorf("internal error: handler panic")
		}
	}()
	return handler.HandleMessage(ctx, sessionID, msg, writer)
}
