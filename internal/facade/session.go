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
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// processMessage handles processing of an incoming client message.
func (s *Server) processMessage(ctx context.Context, c *Connection, msg *ClientMessage, log logr.Logger) error {
	// Get or create session
	sessionID, err := s.ensureSession(ctx, c, msg.SessionID, log)
	if err != nil {
		s.sendError(c, msg.SessionID, ErrorCodeInternalError, "failed to create session")
		return err
	}

	// Enrich context with session ID
	ctx = logctx.WithSessionID(ctx, sessionID)
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
	if msg.Type == MessageTypeUploadRequest {
		return s.handleUploadRequest(ctx, sessionID, msg, writer, log)
	}

	// Store user message (only for regular messages)
	if err := s.sessionStore.AppendMessage(ctx, sessionID, session.Message{
		Role:      session.RoleUser,
		Content:   msg.Content,
		Metadata:  msg.Metadata,
		Timestamp: time.Now(),
	}); err != nil {
		log.Error(err, "failed to store user message")
	}

	// Handle message
	if s.handler != nil {
		if err := s.handler.HandleMessage(ctx, sessionID, msg, writer); err != nil {
			s.sendError(c, sessionID, ErrorCodeInternalError, err.Error())
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
		AgentName: c.agentName,
		Namespace: c.namespace,
		TTL:       s.config.SessionTTL,
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
