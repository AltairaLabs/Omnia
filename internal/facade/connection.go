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
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// Connection represents an active WebSocket connection.
type Connection struct {
	conn             *websocket.Conn
	sessionID        string
	agentName        string
	namespace        string
	workspaceName    string
	binaryCapable    bool // Client supports binary WebSocket frames
	mu               sync.Mutex
	closed           bool
	sessionPersisted bool // true once the session has been written to the store

	// User identity fields extracted from Istio-injected headers on WebSocket upgrade.
	userID        string
	userRoles     string
	userEmail     string
	authorization string // Original JWT token for passthrough

	// Rollout cohort tracking fields extracted from HTTP headers on WebSocket upgrade.
	cohortID string
	variant  string

	// rateLimiter enforces per-connection message rate limiting. Nil when disabled.
	rateLimiter *rate.Limiter
}

// handleConnection manages the lifecycle of a WebSocket connection.
func (s *Server) handleConnection(ctx context.Context, c *Connection) {
	log := logctx.LoggerWithContext(s.log, ctx)
	defer s.cleanupConnection(c, log)

	if err := s.configureConnection(c); err != nil {
		log.Error(err, "failed to configure connection")
		return
	}

	// Generate a session ID and send "connected" immediately so the client can
	// start sending messages without a handshake deadlock. The session is NOT
	// persisted to the store here — it will be written on the first message via
	// ensureSession, avoiding empty sessions from connections that never send data.
	sessionID := uuid.New().String()
	c.mu.Lock()
	c.sessionID = sessionID
	c.mu.Unlock()
	if err := s.sendConnected(c, sessionID); err != nil {
		log.Error(err, "failed to send connected message")
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

	s.metrics.ConnectionClosed()

	if c.sessionID != "" && c.sessionPersisted {
		s.metrics.SessionClosed()
		s.submitCompletion(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Read the session to check its current status. If the session
			// already has a terminal status (e.g. "error"), do not overwrite
			// it with "completed".
			sess, err := s.sessionStore.GetSession(ctx, c.sessionID)
			if err != nil {
				log.Error(err, "session lookup for completion failed", "sessionID", c.sessionID)
				return
			}
			if session.IsTerminalStatus(sess.Status) {
				log.V(1).Info("session already terminal, skipping completion",
					"sessionID", c.sessionID, "status", sess.Status)
				return
			}

			if err := s.sessionStore.UpdateSessionStatus(ctx, c.sessionID, session.SessionStatusUpdate{
				SetStatus:  session.SessionStatusCompleted,
				SetEndedAt: time.Now(),
			}); err != nil {
				log.Error(err, "session completion failed", "sessionID", c.sessionID)
			}
		})
	}

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
