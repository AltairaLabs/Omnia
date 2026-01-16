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
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
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
		log.Error(err, "failed to unmarshal message", "raw", string(message))
		s.sendError(c, "", ErrorCodeInvalidMessage, "invalid message format")
		return
	}

	s.metrics.RequestStarted()
	startTime := time.Now()

	err := s.processMessage(ctx, c, &clientMsg, log)

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
	s.metrics.RequestCompleted(status, duration, handlerName)
}

// handleBinaryMessage decodes and processes a binary WebSocket frame.
func (s *Server) handleBinaryMessage(_ context.Context, c *Connection, data []byte, log logr.Logger) {
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
	case BinaryMessageTypeUpload:
		// Binary upload handling could be added here in the future
		log.Info("binary upload not yet implemented")
		s.sendError(c, "", ErrorCodeInvalidMessage, "binary upload not yet implemented")
	default:
		log.Error(nil, "unknown binary message type", "type", frame.Header.MessageType)
		s.sendError(c, "", ErrorCodeInvalidMessage, "unknown binary message type")
	}
}

// sendBinaryFrame sends a binary WebSocket frame to the connection.
func (s *Server) sendBinaryFrame(c *Connection, frame *BinaryFrame) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	data, err := frame.Encode()
	if err != nil {
		return err
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
		return err
	}

	if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return err
	}

	s.metrics.MessageSent()
	return nil
}
