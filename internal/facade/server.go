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
		ReadBufferSize:  32 * 1024, // 32KB for large message handling
		WriteBufferSize: 32 * 1024, // 32KB for large message handling
		PingInterval:    30 * time.Second,
		PongTimeout:     60 * time.Second,
		WriteTimeout:    10 * time.Second,
		MaxMessageSize:  16 * 1024 * 1024, // 16MB to support base64-encoded images
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
