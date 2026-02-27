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
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/otlp"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// envAllowedOrigins is the environment variable for configuring allowed WebSocket origins.
const envAllowedOrigins = "OMNIA_ALLOWED_ORIGINS"

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
	// PromptPackName is the PromptPack associated with this agent (from env).
	PromptPackName string
	// PromptPackVersion is the PromptPack version (from env).
	PromptPackVersion string
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
	config          ServerConfig
	upgrader        websocket.Upgrader
	sessionStore    session.Store
	handler         MessageHandler
	metrics         ServerMetrics
	mediaStorage    media.Storage
	tracingProvider *tracing.Provider
	allowedOrigins  []string
	log             logr.Logger

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

// WithTracingProvider sets the tracing provider for the server.
// When set, the server creates spans for sessions and messages.
func WithTracingProvider(p *tracing.Provider) ServerOption {
	return func(s *Server) {
		s.tracingProvider = p
	}
}

// WithAllowedOrigins sets the allowed origins for WebSocket connections.
// Origins should be scheme + host (e.g., "https://example.com").
// When set, only requests from these origins are allowed.
// When empty, the default gorilla/websocket same-origin check is used.
func WithAllowedOrigins(origins []string) ServerOption {
	return func(s *Server) {
		s.allowedOrigins = origins
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
	}

	// Apply options first so allowedOrigins is set before building the upgrader
	for _, opt := range opts {
		opt(s)
	}

	// Load allowed origins from environment if not set via options
	if len(s.allowedOrigins) == 0 {
		s.allowedOrigins = ParseAllowedOrigins(os.Getenv(envAllowedOrigins))
	}

	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     s.checkOrigin,
	}

	return s
}

// checkOrigin validates the Origin header against the allowed origins list.
// If no allowed origins are configured, it uses the default gorilla/websocket
// same-origin check (Origin host must match the Host header).
func (s *Server) checkOrigin(r *http.Request) bool {
	// If no allowlist configured, use same-origin policy
	if len(s.allowedOrigins) == 0 {
		return checkSameOrigin(r)
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header — allow (non-browser clients like curl, gRPC, etc.)
		return true
	}

	for _, allowed := range s.allowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}

	s.log.V(1).Info("rejected WebSocket connection from disallowed origin",
		"origin", origin)
	return false
}

// checkSameOrigin implements the standard same-origin check for WebSocket
// connections. It verifies the Origin header's host matches the request's
// Host header, which is the default gorilla/websocket behavior.
func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// ParseAllowedOrigins parses a comma-separated list of allowed origins.
// Empty strings and whitespace-only entries are filtered out.
func ParseAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) == 0 {
		return nil
	}
	return origins
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

	// Extract agent info from query params, falling back to pod env vars
	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		agentName = os.Getenv("OMNIA_AGENT_NAME")
	}
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = os.Getenv("OMNIA_NAMESPACE")
	}
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

	// Start session span if tracing is enabled
	var sessionSpan trace.Span
	if s.tracingProvider != nil {
		connCtx, sessionSpan = s.tracingProvider.Tracer().Start(connCtx, "facade.session",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String(otlp.AttrOmniaAgentName, agentName),
				attribute.String(otlp.AttrOmniaAgentNamespace, namespace),
				attribute.String(otlp.AttrOmniaPromptPackName, s.config.PromptPackName),
				attribute.String(otlp.AttrOmniaPromptPackVersion, s.config.PromptPackVersion),
				attribute.String(otlp.AttrOmniaPromptPackNamespace, namespace),
			),
		)
		c.sessionSpan = sessionSpan
	}

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
