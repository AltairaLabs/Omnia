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

	// resumeID is the session_id the client asked to resume via ?resume=.
	// Empty when this is a fresh (non-resume) connection.
	resumeID string
	// intentionalClose is set to true when the client explicitly hangs up
	// (e.g. sends a close frame with a normal-closure code) so that the
	// blip-resume path knows NOT to park the audio session.
	intentionalClose bool

	// sessionConsentGrants holds the per-session consent grants captured
	// from the first non-empty ClientMessage.SessionConsentGrants the
	// facade saw on this connection. Subsequent non-empty lists replace
	// the cached value (last-writer-wins). Empty / omitted lists are
	// ignored. nil means "no session-level grants set." Mutex-protected
	// via c.mu.
	sessionConsentGrants []string

	// rateLimiter enforces per-connection message rate limiting. Nil when disabled.
	rateLimiter *rate.Limiter
	// inFlightMessages limits concurrently processed non-tool messages per connection.
	// Nil when disabled.
	inFlightMessages chan struct{}

	// audioSession is the persistent duplex audio stream for this connection.
	// Created lazily on the first inbound BinaryMessageTypeMediaChunk frame
	// via Server.ensureAudioSession. Nil until the first media chunk arrives
	// or when no duplexSinkFactory is configured on the Server.
	// Protected by c.mu.
	audioSession *audioSession
}

func (c *Connection) tryAcquireInFlightMessage() bool {
	if c.inFlightMessages == nil {
		return true
	}
	select {
	case c.inFlightMessages <- struct{}{}:
		return true
	default:
		return false
	}
}

func (c *Connection) releaseInFlightMessage() {
	if c.inFlightMessages == nil {
		return
	}
	select {
	case <-c.inFlightMessages:
	default:
	}
}

// handleConnection manages the lifecycle of a WebSocket connection.
func (s *Server) handleConnection(ctx context.Context, c *Connection) {
	log := logctx.LoggerWithContext(s.log, ctx)
	defer s.cleanupConnection(c, log)

	if err := s.configureConnection(c); err != nil {
		log.Error(err, "failed to configure connection")
		return
	}

	// Attempt to reattach to a parked realtime session named by ?resume=<sid>.
	// On success, bind the connection to the parked session and send connected
	// with resumed=true. On miss (no park, owner mismatch, or no resumeID),
	// fall through to the existing fresh-session path.
	if _, resumed := s.tryReattach(ctx, c); resumed {
		if err := s.sendConnected(c, c.SessionID(), true); err != nil {
			log.Error(err, "failed to send connected message")
			return
		}
	} else {
		sessionID := uuid.New().String()
		c.mu.Lock()
		c.sessionID = sessionID
		c.mu.Unlock()
		if err := s.sendConnected(c, sessionID, false); err != nil {
			log.Error(err, "failed to send connected message")
			return
		}
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

// SessionID returns the connection's current session ID safely.
func (c *Connection) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// SessionPersisted reports whether the session has been written to the store.
func (c *Connection) SessionPersisted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionPersisted
}

// parkOnClose decides whether a closing connection's realtime session is parked
// for blip-resume or torn down. Parked sessions keep their provider socket open
// and their active-session count, expiring via the registry grace timer.
// Returns true when the session was parked (completion should be skipped),
// false when it was torn down or there was no audio session.
func (s *Server) parkOnClose(ctx context.Context, c *Connection) bool {
	c.mu.Lock()
	as := c.audioSession
	c.audioSession = nil
	intentional := c.intentionalClose
	sessionID := c.sessionID
	ownerID := c.userID
	c.mu.Unlock()

	if as == nil {
		return false
	}
	if intentional {
		if err := as.close(); err != nil {
			s.log.Error(err, "audio session close failed", "sessionID", sessionID)
		}
		s.decrementAudioSessions(s.metrics)
		return false
	}
	s.parked.park(ctx, sessionID, ownerID, as)
	s.metrics.RealtimeSessionParked()
	return true
}

// tryReattach binds the connection to a parked realtime session named by
// c.resumeID if one exists and is owned by c.userID. Returns the session and
// true on success; (nil, false) to fall through to a fresh session.
func (s *Server) tryReattach(ctx context.Context, c *Connection) (*audioSession, bool) {
	if c.resumeID == "" {
		return nil, false
	}
	as, ok := s.parked.take(ctx, c.resumeID, c.userID)
	if !ok {
		s.log.V(1).Info("realtime reattach miss", "sessionID", c.resumeID, "reason", "miss_or_owner_mismatch")
		return nil, false
	}
	c.mu.Lock()
	c.sessionID = c.resumeID
	c.audioSession = as
	c.mu.Unlock()
	s.metrics.RealtimeSessionReattached()
	return as, true
}

// cleanupConnection handles connection cleanup when it closes.
func (s *Server) cleanupConnection(c *Connection, log logr.Logger) {
	s.mu.Lock()
	delete(s.connections, c.conn)
	s.mu.Unlock()

	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	parked := s.parkOnClose(context.Background(), c)

	s.metrics.ConnectionClosed()

	// Snapshot session ID once under the mutex; the closure runs in a goroutine
	// and must not race against concurrent writers of c.sessionID.
	sessionID := c.SessionID()
	if !parked && sessionID != "" && c.SessionPersisted() {
		s.metrics.SessionClosed()
		s.submitCompletion(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Read the session to check its current status. If the session
			// already has a terminal status (e.g. "error"), do not overwrite
			// it with "completed".
			sess, err := s.sessionStore.GetSession(ctx, sessionID)
			if err != nil {
				log.Error(err, "session lookup for completion failed", "sessionID", sessionID)
				return
			}
			if session.IsTerminalStatus(sess.Status) {
				log.V(1).Info("session already terminal, skipping completion",
					"sessionID", sessionID, "status", sess.Status)
				return
			}

			if err := s.sessionStore.UpdateSessionStatus(ctx, sessionID, session.SessionStatusUpdate{
				SetStatus:  session.SessionStatusCompleted,
				SetEndedAt: time.Now(),
			}); err != nil {
				log.Error(err, "session completion failed", "sessionID", sessionID)
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
