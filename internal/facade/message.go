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
	"encoding/json"
	"fmt"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/pkg/logging"
)

// readMessageLoop reads and processes messages from the connection.
func (s *Server) readMessageLoop(ctx context.Context, c *Connection, log logr.Logger) {
	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			s.logCloseError(err, log)
			return
		}

		s.metrics.MessageReceived()

		// Enforce per-connection rate limit
		if c.rateLimiter != nil && !c.rateLimiter.Allow() {
			log.V(1).Info("message rate limited")
			s.sendError(c, "", ErrorCodeRateLimited, "rate limit exceeded")
			continue
		}

		// Handle based on WebSocket message type
		if messageType == websocket.BinaryMessage {
			s.handleBinaryMessage(ctx, c, message, log)
		} else {
			s.handleClientMessage(ctx, c, message, log)
		}
	}
}

// logCloseError logs unexpected close errors.
func (s *Server) logCloseError(err error, log logr.Logger) {
	if websocket.IsUnexpectedCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		log.Error(err, "unexpected close error")
	}
}

// handleClientMessage parses and processes a single client message.
func (s *Server) handleClientMessage(ctx context.Context, c *Connection, message []byte, log logr.Logger) {
	var clientMsg ClientMessage
	if err := json.Unmarshal(message, &clientMsg); err != nil {
		log.Error(err, "failed to unmarshal message", "contentLength", logging.ContentLength(string(message)))
		s.sendError(c, "", ErrorCodeInvalidMessage, "invalid message format")
		return
	}

	if s.handleToolMessage(ctx, c, &clientMsg, log) {
		return
	}
	if !c.tryAcquireInFlightMessage() {
		log.V(1).Info("in-flight message limit exceeded")
		s.sendError(c, c.sessionID, ErrorCodeRateLimited, "too many in-flight requests")
		return
	}

	s.metrics.RequestStarted()

	// Process the message asynchronously so the read loop can continue
	// reading tool_result messages while HandleMessage blocks waiting
	// for client tool responses.
	go s.processAndRecordMessage(ctx, c, &clientMsg, log)
}

// handleToolMessage routes tool-related messages (ACK, NACK, result) to the handler.
// Returns true if the message was handled, false if it should be processed normally.
func (s *Server) handleToolMessage(ctx context.Context, c *Connection, clientMsg *ClientMessage, log logr.Logger) bool {
	// Route tool call ACK to the active handler
	if clientMsg.Type == MessageTypeToolCallAck && clientMsg.ToolCallAck != nil {
		if router, ok := s.handler.(ClientToolRouter); ok {
			router.AckToolCall(c.SessionID(), clientMsg.ToolCallAck.CallID)
		}
		return true
	}

	// Route tool call NACK — convert to a rejection tool_result
	if clientMsg.Type == MessageTypeToolCallNack && clientMsg.ToolCallNack != nil {
		result := &ClientToolResultInfo{
			CallID: clientMsg.ToolCallNack.CallID,
			Error:  clientMsg.ToolCallNack.Reason,
		}
		sessionID := c.SessionID()
		if router, ok := s.handler.(ClientToolRouter); ok {
			router.SendToolResult(sessionID, result)
		}
		s.recordClientToolResult(ctx, sessionID, result, log)
		return true
	}

	// Route client-side tool results to the active handler
	if clientMsg.Type == MessageTypeToolResult && clientMsg.ToolResult != nil {
		sessionID := c.SessionID()
		if router, ok := s.handler.(ClientToolRouter); ok {
			if router.SendToolResult(sessionID, clientMsg.ToolResult) {
				s.recordClientToolResult(ctx, sessionID, clientMsg.ToolResult, log)
				return true
			}
		}
		s.sendError(c, sessionID, ErrorCodeInvalidMessage, "no pending tool call")
		return true
	}

	return false
}

// recordClientToolResult persists a client-side tool result in the session store
// so that tool calls always have a corresponding resolution in the session history.
func (s *Server) recordClientToolResult(ctx context.Context, sessionID string, result *ClientToolResultInfo, log logr.Logger) {
	if s.sessionStore == nil || sessionID == "" {
		return
	}
	var content string
	if result.Error != "" {
		content = result.Error
	} else if result.Result != nil {
		data, err := json.Marshal(result.Result)
		if err != nil {
			content = fmt.Sprintf("%v", result.Result)
		} else {
			content = string(data)
		}
	}

	metadata := map[string]string{
		"type": "tool_result",
	}
	if result.Error != "" {
		metadata["is_error"] = "true"
	}

	msg := session.Message{
		ID:         uuid.New().String(),
		Role:       session.RoleSystem,
		Content:    content,
		ToolCallID: result.CallID,
		Timestamp:  time.Now(),
		Metadata:   metadata,
	}
	storeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.sessionStore.AppendMessage(storeCtx, sessionID, msg); err != nil {
		log.Error(err, "failed to record client tool result", "sessionID", sessionID, "callID", result.CallID)
	}
}

// processAndRecordMessage processes a client message and records metrics.
// Runs in a goroutine from handleClientMessage so the WebSocket read loop
// stays alive to receive tool_result messages during client tool execution.
func (s *Server) processAndRecordMessage(ctx context.Context, c *Connection, msg *ClientMessage, log logr.Logger) {
	defer c.releaseInFlightMessage()

	startTime := time.Now()

	err := s.processMessage(ctx, c, msg, log)

	duration := time.Since(startTime).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		log.Error(err, "error processing message")
	}
	handlerName := "none"
	if s.handler != nil {
		handlerName = s.handler.Name()
	}
	s.metrics.RequestCompleted(ctx, status, duration, handlerName)
}

// handleBinaryMessage decodes and processes a binary WebSocket frame.
func (s *Server) handleBinaryMessage(ctx context.Context, c *Connection, data []byte, log logr.Logger) {
	frame, err := DecodeBinaryFrame(data)
	if err != nil {
		log.Error(err, "failed to decode binary frame")
		s.sendError(c, "", ErrorCodeInvalidMessage, "invalid binary frame: "+err.Error())
		return
	}

	log.V(1).Info("received binary frame",
		"messageType", frame.Header.MessageType.String(),
		"sequence", frame.Header.Sequence,
		"payloadLen", frame.Header.PayloadLen,
	)

	switch frame.Header.MessageType {
	case BinaryMessageTypeMediaChunk:
		s.routeMediaChunk(ctx, c, frame, log)
	case BinaryMessageTypeUpload:
		// Binary upload handling could be added here in the future
		log.Info("binary upload not yet implemented")
		s.sendError(c, "", ErrorCodeInvalidMessage, "binary upload not yet implemented")
	default:
		log.Error(nil, "unknown binary message type", "type", frame.Header.MessageType)
		s.sendError(c, "", ErrorCodeInvalidMessage, "unknown binary message type")
	}
}

// routeMediaChunk routes an inbound BinaryMessageTypeMediaChunk frame to the
// connection's persistent audio session, creating it lazily on the first frame.
func (s *Server) routeMediaChunk(ctx context.Context, c *Connection, frame *BinaryFrame, log logr.Logger) {
	as := s.ensureAudioSession(ctx, c, log)
	if as == nil {
		// ensureAudioSession already logged and/or sent an error — nothing more to do.
		return
	}
	if err := as.handleInboundFrame(frame); err != nil {
		log.Error(err, "audio session forward failed", "sessionID", c.sessionID)
		s.sendError(c, c.sessionID, ErrorCodeInvalidMessage, "audio forward failed: "+err.Error())
	}
}

// ensureAudioSession returns the connection's audio session, creating it
// lazily on the first call using the server's duplexSinkFactory.
//
// Returns nil (without panicking) when no factory is configured — audio
// streaming is not enabled for this server instance. In that case a graceful
// error is sent to the client so the browser receives a clear diagnostic
// rather than a silent drop.
//
// NOTE: codec/sample-rate/channels defaults (pcm/16000/1) are used for the
// initial Start call. Future work (Task N) can parse BinaryMediaChunkMetadata
// from the first frame and pass the negotiated parameters here instead.
func (s *Server) ensureAudioSession(ctx context.Context, c *Connection, log logr.Logger) *audioSession {
	c.mu.Lock()
	if c.audioSession != nil {
		as := c.audioSession
		c.mu.Unlock()
		return as
	}
	c.mu.Unlock()

	if s.duplexSinkFactory == nil {
		log.V(1).Info("audio frame dropped",
			"reason", "duplexSinkFactory not configured",
			"sessionID", c.sessionID,
		)
		return nil
	}

	// Build the writer for relaying audio back to the client.
	w := &connResponseWriter{conn: c, sessionID: c.sessionID, server: s}
	sink := s.duplexSinkFactory(c.sessionID, w)

	as := newAudioSession(c.sessionID, sink, w)

	// Default audio parameters. TODO: parse from BinaryMediaChunkMetadata when
	// the client negotiation protocol is defined (future task).
	startParams := &AudioSessionStart{Codec: "pcm", SampleRate: 16000, Channels: 1}
	if err := as.start(ctx, startParams); err != nil {
		log.Error(err, "audio session start failed", "sessionID", c.sessionID)
		s.sendError(c, c.sessionID, ErrorCodeInvalidMessage, "audio session start failed: "+err.Error())
		return nil
	}

	c.mu.Lock()
	// Double-check: another goroutine may have raced us.
	if c.audioSession != nil {
		c.mu.Unlock()
		// Discard the one we just created; close its sink.
		_ = as.close()
		return c.audioSession
	}
	c.audioSession = as
	c.mu.Unlock()

	log.V(1).Info("audio session started", "sessionID", c.sessionID)
	return as
}

// sendBinaryFrame sends a binary WebSocket frame to the connection.
// Uses a pooled buffer for encoding to reduce GC pressure on the streaming path.
func (s *Server) sendBinaryFrame(c *Connection, frame *BinaryFrame) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.conn == nil {
		return nil
	}

	bp, err := frame.EncodePooled()
	if err != nil {
		return err
	}
	defer PutPooledBuf(bp)

	if err := c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
		return err
	}

	if err := c.conn.WriteMessage(websocket.BinaryMessage, *bp); err != nil {
		return err
	}

	// Clear the deadline so idle connections aren't killed
	if err := c.conn.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}

	s.metrics.MessageSent()
	return nil
}
