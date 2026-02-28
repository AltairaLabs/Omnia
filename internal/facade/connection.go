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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/pkg/logctx"
)

// Connection represents an active WebSocket connection.
type Connection struct {
	conn          *websocket.Conn
	sessionID     string
	agentName     string
	namespace     string
	binaryCapable bool // Client supports binary WebSocket frames
	sessionSpan   trace.Span
	mu            sync.Mutex
	closed        bool

	// User identity fields extracted from Istio-injected headers on WebSocket upgrade.
	userID        string
	userRoles     string
	userEmail     string
	authorization string // Original JWT token for passthrough

	// toolCallLimiter enforces per-session tool call limits from AgentPolicy.
	toolCallLimiter *ToolCallLimiter
}

// handleConnection manages the lifecycle of a WebSocket connection.
func (s *Server) handleConnection(ctx context.Context, c *Connection) {
	log := logctx.LoggerWithContext(s.log, ctx)
	defer s.cleanupConnection(c, log)

	if err := s.configureConnection(c); err != nil {
		log.Error(err, "failed to configure connection")
		return
	}

	// Start ping ticker
	pingTicker := time.NewTicker(s.config.PingInterval)
	defer pingTicker.Stop()

	// Handle ping in separate goroutine with cancellable context
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.runPingLoop(connCtx, c, pingTicker)

	// Message read loop
	s.readMessageLoop(connCtx, c, log)
}

// cleanupConnection handles connection cleanup when it closes.
func (s *Server) cleanupConnection(c *Connection, log logr.Logger) {
	s.mu.Lock()
	delete(s.connections, c.conn)
	s.mu.Unlock()

	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	// End session span if tracing was enabled
	if c.sessionSpan != nil {
		c.sessionSpan.End()
	}

	s.metrics.ConnectionClosed()

	if err := c.conn.Close(); err != nil {
		log.Error(err, "error closing connection")
	}
}

// configureConnection sets up connection limits and handlers.
func (s *Server) configureConnection(c *Connection) error {
	c.conn.SetReadLimit(s.config.MaxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(s.config.PongTimeout)); err != nil {
		return err
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(s.config.PongTimeout))
	})
	return nil
}

// runPingLoop sends periodic pings to keep the connection alive.
func (s *Server) runPingLoop(ctx context.Context, c *Connection, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.sendPing(c) {
				return
			}
		}
	}
}

// sendPing sends a ping message to the connection. Returns false if connection should close.
func (s *Server) sendPing(c *Connection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}
	if c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)) != nil {
		return false
	}
	if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
		return false
	}
	return true
}
