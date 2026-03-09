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
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/otlp"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
)

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

	// Enrich context with session ID, namespace, and trace ID for log↔trace correlation
	ctx = logctx.WithSessionID(ctx, sessionID)
	ctx = logctx.WithNamespace(ctx, c.namespace)
	ctx = logctx.WithTraceID(ctx, msgSpan.SpanContext().TraceID().String())
	log = logctx.LoggerWithContext(s.log, ctx)

	// Update connection's session ID
	c.mu.Lock()
	c.sessionID = sessionID
	c.mu.Unlock()

	// Send connected message if this is a new session
	if msg.SessionID == "" {
		if err := s.sendConnected(c, sessionID); err != nil {
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

	opts = append(opts,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("session.id", sessionID),
			attribute.String(otlp.AttrOmniaAgentName, c.agentName),
			attribute.String(otlp.AttrOmniaAgentNamespace, c.namespace),
			attribute.String(otlp.AttrOmniaPromptPackName, s.config.PromptPackName),
			attribute.String(otlp.AttrOmniaPromptPackVersion, s.config.PromptPackVersion),
			attribute.String(otlp.AttrOmniaPromptPackNamespace, c.namespace),
		),
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

// processRegularMessage stores the user message and dispatches to the handler.
func (s *Server) processRegularMessage(ctx context.Context, c *Connection, sessionID string, msg *ClientMessage, writer *connResponseWriter, log logr.Logger) error {
	// Store user message
	if err := s.sessionStore.AppendMessage(ctx, sessionID, session.Message{
		ID:        uuid.New().String(),
		Role:      session.RoleUser,
		Content:   msg.Content,
		Metadata:  msg.Metadata,
		Timestamp: time.Now(),
	}); err != nil {
		log.Error(err, "failed to store user message")
	}

	// Wrap writer with recording decorator to persist assistant responses
	recWriter := newRecordingWriter(ctx, writer, s.sessionStore, sessionID, log, s.recordingPool)

	// Handle message
	if s.handler != nil {
		if err := safeHandleMessage(s.handler, ctx, sessionID, msg, recWriter, log); err != nil {
			s.sendError(c, sessionID, ErrorCodeInternalError, err.Error())
			return err
		}
	} else {
		// Default echo behavior if no handler
		if err := recWriter.WriteDone("Handler not configured"); err != nil {
			return err
		}
	}

	return nil
}

// ensureSession gets an existing session or creates a new one.
func (s *Server) ensureSession(ctx context.Context, c *Connection, sessionID string, log logr.Logger) (string, error) {
	if sessionID != "" {
		// Try to resume existing session
		sess, err := s.sessionStore.GetSession(ctx, sessionID)
		if err == nil {
			// Refresh TTL
			if err := s.sessionStore.RefreshTTL(ctx, sessionID, s.config.SessionTTL); err != nil {
				log.Error(err, "failed to refresh session TTL")
			}
			return sess.ID, nil
		}
		// Session not found or expired, create new one
		log.Info("session not found, creating new", "requested_id", sessionID)
	}

	// Create new session
	sess, err := s.sessionStore.CreateSession(ctx, session.CreateSessionOptions{
		AgentName:         c.agentName,
		Namespace:         c.namespace,
		TTL:               s.config.SessionTTL,
		PromptPackName:    s.config.PromptPackName,
		PromptPackVersion: s.config.PromptPackVersion,
	})
	if err != nil {
		return "", err
	}

	return sess.ID, nil
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
