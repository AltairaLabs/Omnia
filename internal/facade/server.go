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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// ServerConfig contains configuration for the WebSocket server.
type ServerConfig struct {
	// ReadBufferSize is the size of the read buffer.
	ReadBufferSize int
	// WriteBufferSize is the size of the write buffer.
	WriteBufferSize int
	// PingInterval is how often to send ping messages.
	PingInterval time.Duration
	// PongTimeout is how long to wait for a pong response.
	PongTimeout time.Duration
	// WriteTimeout is the timeout for write operations.
	WriteTimeout time.Duration
	// MaxMessageSize is the maximum message size.
	MaxMessageSize int64
	// SessionTTL is the default TTL for new sessions.
	SessionTTL time.Duration
}

// DefaultServerConfig returns a ServerConfig with default values.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    30 * time.Second,
		PongTimeout:     60 * time.Second,
		WriteTimeout:    10 * time.Second,
		MaxMessageSize:  512 * 1024, // 512KB
		SessionTTL:      24 * time.Hour,
	}
}

// MessageHandler handles incoming client messages.
type MessageHandler interface {
	// Name returns the handler name for metrics labeling.
	Name() string
	// HandleMessage processes a client message and streams responses.
	// The handler should send responses via the ResponseWriter.
	HandleMessage(ctx context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter) error
}

// ResponseWriter allows sending responses back to the client.
type ResponseWriter interface {
	// WriteChunk sends a chunk of the response.
	WriteChunk(content string) error
	// WriteChunkWithParts sends a chunk with multi-modal content parts.
	WriteChunkWithParts(parts []ContentPart) error
	// WriteDone signals the response is complete.
	WriteDone(content string) error
	// WriteDoneWithParts signals completion with multi-modal content parts.
	WriteDoneWithParts(parts []ContentPart) error
	// WriteToolCall notifies of a tool call.
	WriteToolCall(toolCall *ToolCallInfo) error
	// WriteToolResult sends a tool result.
	WriteToolResult(result *ToolResultInfo) error
	// WriteError sends an error message.
	WriteError(code, message string) error
	// WriteUploadReady sends upload URL information to the client.
	WriteUploadReady(uploadReady *UploadReadyInfo) error
	// WriteUploadComplete notifies the client that an upload is complete.
	WriteUploadComplete(uploadComplete *UploadCompleteInfo) error
	// WriteMediaChunk sends a streaming media chunk to the client.
	// Used for streaming audio/video responses where playback can begin
	// before the entire media is generated.
	WriteMediaChunk(mediaChunk *MediaChunkInfo) error
	// WriteBinaryMediaChunk sends a streaming media chunk as a binary frame.
	// Falls back to base64 JSON if the client doesn't support binary frames.
	WriteBinaryMediaChunk(mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) error
	// SupportsBinary returns true if the client supports binary WebSocket frames.
	SupportsBinary() bool
}

// Server is a WebSocket server for agent communication.
type Server struct {
	config       ServerConfig
	upgrader     websocket.Upgrader
	sessionStore session.Store
	handler      MessageHandler
	metrics      ServerMetrics
	mediaStorage media.Storage
	log          logr.Logger

	mu          sync.RWMutex
	connections map[*websocket.Conn]*Connection
	shutdown    bool
}

// Connection represents an active WebSocket connection.
type Connection struct {
	conn          *websocket.Conn
	sessionID     string
	agentName     string
	namespace     string
	binaryCapable bool // Client supports binary WebSocket frames
	mu            sync.Mutex
	closed        bool
}

// ServerOption is a functional option for configuring the server.
type ServerOption func(*Server)

// WithMetrics sets the metrics collector for the server.
func WithMetrics(m ServerMetrics) ServerOption {
	return func(s *Server) {
		s.metrics = m
	}
}

// WithMediaStorage sets the media storage for the server.
// When set, the server can handle upload_request messages from clients.
func WithMediaStorage(ms media.Storage) ServerOption {
	return func(s *Server) {
		s.mediaStorage = ms
	}
}

// NewServer creates a new WebSocket server.
func NewServer(cfg ServerConfig, store session.Store, handler MessageHandler, log logr.Logger, opts ...ServerOption) *Server {
	s := &Server{
		config:       cfg,
		sessionStore: store,
		handler:      handler,
		metrics:      &NoOpMetrics{}, // Default to no-op
		log:          log.WithName("websocket-server"),
		connections:  make(map[*websocket.Conn]*Connection),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now; can be customized
				return true
			},
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ServeHTTP handles WebSocket upgrade requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	if s.shutdown {
		s.mu.RUnlock()
		http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.mu.RUnlock()

	// Extract agent info from query params or headers
	agentName := r.URL.Query().Get("agent")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	// Check if client requests binary frame support
	binaryCapable := r.URL.Query().Get("binary") == "true"

	if agentName == "" {
		http.Error(w, "agent parameter is required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error(err, "failed to upgrade connection")
		return
	}

	// Create connection wrapper
	c := &Connection{
		conn:          conn,
		agentName:     agentName,
		namespace:     namespace,
		binaryCapable: binaryCapable,
	}

	s.mu.Lock()
	s.connections[conn] = c
	s.mu.Unlock()

	// Record connection metrics
	s.metrics.ConnectionOpened()

	// Create enriched context with connection info
	connCtx := logctx.WithAgent(context.Background(), agentName)
	connCtx = logctx.WithNamespace(connCtx, namespace)
	connCtx = logctx.WithRequestID(connCtx, uuid.New().String())

	log := logctx.LoggerWithContext(s.log, connCtx)
	log.Info("new connection")

	// Handle connection in goroutine
	go s.handleConnection(connCtx, c)
}

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

func (s *Server) sendMessage(c *Connection, msg *ServerMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
		return err
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return err
	}

	// Record message sent
	s.metrics.MessageSent()
	return nil
}

func (s *Server) sendError(c *Connection, sessionID, code, message string) {
	if err := s.sendMessage(c, NewErrorMessage(sessionID, code, message)); err != nil {
		s.log.Error(err, "failed to send error message")
	}
}

func (s *Server) sendConnected(c *Connection, sessionID string) error {
	if c.binaryCapable {
		return s.sendMessage(c, NewConnectedMessageWithCapabilities(sessionID, &ConnectionCapabilities{
			BinaryFrames:    true,
			MaxPayloadSize:  int(s.config.MaxMessageSize),
			ProtocolVersion: BinaryVersion,
		}))
	}
	return s.sendMessage(c, NewConnectedMessage(sessionID))
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdown = true
	connections := make([]*websocket.Conn, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	s.mu.Unlock()

	// Close all connections
	for _, conn := range connections {
		if err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"),
			time.Now().Add(time.Second),
		); err != nil {
			s.log.Error(err, "error sending close message")
		}
		if err := conn.Close(); err != nil {
			s.log.Error(err, "error closing connection")
		}
	}

	return nil
}

// ConnectionCount returns the number of active connections.
func (s *Server) ConnectionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

// connResponseWriter implements ResponseWriter for a connection.
type connResponseWriter struct {
	conn      *Connection
	sessionID string
	server    *Server
}

func (w *connResponseWriter) WriteChunk(content string) error {
	return w.server.sendMessage(w.conn, NewChunkMessage(w.sessionID, content))
}

func (w *connResponseWriter) WriteChunkWithParts(parts []ContentPart) error {
	return w.server.sendMessage(w.conn, NewChunkMessageWithParts(w.sessionID, parts))
}

func (w *connResponseWriter) WriteDone(content string) error {
	return w.server.sendMessage(w.conn, NewDoneMessage(w.sessionID, content))
}

func (w *connResponseWriter) WriteDoneWithParts(parts []ContentPart) error {
	return w.server.sendMessage(w.conn, NewDoneMessageWithParts(w.sessionID, parts))
}

func (w *connResponseWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	return w.server.sendMessage(w.conn, NewToolCallMessage(w.sessionID, toolCall))
}

func (w *connResponseWriter) WriteToolResult(result *ToolResultInfo) error {
	return w.server.sendMessage(w.conn, NewToolResultMessage(w.sessionID, result))
}

func (w *connResponseWriter) WriteError(code, message string) error {
	return w.server.sendMessage(w.conn, NewErrorMessage(w.sessionID, code, message))
}

func (w *connResponseWriter) WriteUploadReady(uploadReady *UploadReadyInfo) error {
	return w.server.sendMessage(w.conn, NewUploadReadyMessage(w.sessionID, uploadReady))
}

func (w *connResponseWriter) WriteUploadComplete(uploadComplete *UploadCompleteInfo) error {
	return w.server.sendMessage(w.conn, NewUploadCompleteMessage(w.sessionID, uploadComplete))
}

func (w *connResponseWriter) WriteMediaChunk(mediaChunk *MediaChunkInfo) error {
	return w.server.sendMessage(w.conn, NewMediaChunkMessage(w.sessionID, mediaChunk))
}

func (w *connResponseWriter) SupportsBinary() bool {
	return w.conn.binaryCapable
}

func (w *connResponseWriter) WriteBinaryMediaChunk(mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) error {
	if !w.SupportsBinary() {
		// Fallback to base64 JSON for clients that don't support binary
		return w.WriteMediaChunk(&MediaChunkInfo{
			MediaID:  MediaIDToString(mediaID),
			Sequence: int(sequence),
			IsLast:   isLast,
			Data:     base64.StdEncoding.EncodeToString(payload),
			MimeType: mimeType,
		})
	}

	frame, err := NewMediaChunkFrame(w.sessionID, mediaID, sequence, isLast, mimeType, payload)
	if err != nil {
		return err
	}

	return w.server.sendBinaryFrame(w.conn, frame)
}
