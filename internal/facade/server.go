/*
Copyright 2025-2026.

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
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/time/rate"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/policy"
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
	// MaxConnections is the maximum number of concurrent WebSocket connections.
	// 0 means unlimited (not recommended for production).
	MaxConnections int
	// SessionTTL is the default TTL for new sessions.
	SessionTTL time.Duration
	// PromptPackName is the PromptPack associated with this agent (from env).
	PromptPackName string
	// PromptPackVersion is the PromptPack version (from env).
	PromptPackVersion string
	// MessageRateLimit is the maximum sustained messages per second per connection.
	// 0 disables rate limiting.
	MessageRateLimit float64
	// MessageRateBurst is the maximum burst size for per-connection rate limiting.
	MessageRateBurst int
	// WorkspaceName is the workspace this agent belongs to (for session metadata).
	WorkspaceName string
}

// DefaultServerConfig returns a ServerConfig with default values.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ReadBufferSize:   64 * 1024, // 64KB to reduce reallocation for larger messages
		WriteBufferSize:  64 * 1024, // 64KB to reduce reallocation for larger messages
		PingInterval:     30 * time.Second,
		PongTimeout:      60 * time.Second,
		WriteTimeout:     10 * time.Second,
		MaxMessageSize:   16 * 1024 * 1024, // 16MB to support base64-encoded images
		MaxConnections:   500,
		SessionTTL:       24 * time.Hour,
		MessageRateLimit: 50,
		MessageRateBurst: 100,
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

// ClientToolRouter routes client-side tool results to the active handler.
// Implemented by handlers that support bidirectional client tool execution.
type ClientToolRouter interface {
	// SendToolResult delivers a client tool result to the handler waiting for it.
	// Returns true if the result was routed to an active handler, false otherwise.
	SendToolResult(sessionID string, result *ClientToolResultInfo) bool
	// AckToolCall acknowledges receipt of a tool call, signaling the client is working on it.
	AckToolCall(sessionID string, callID string)
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
	recordingPool   *RecordingPool
	allowedOrigins  []string
	policyFetcher   PolicyFetcher
	// authChain, when non-empty, runs every configured Validator against
	// the upgrade request in order and admits on the first match. On
	// admit the identity flows into PropagationFields.Identity and the
	// flat UserID / UserRoles / UserEmail fields.
	//
	// PR 3 flipped the chain-wide ErrNoCredential behaviour from
	// "proceed unauthenticated" (PR 1 default) to "return 401 before
	// Upgrade", closing pen-test finding C-3.
	//
	// Empty chain still proceeds when allowUnauthenticated is true (the
	// default) so dev/test binaries without any validator configured
	// keep working; set WithAllowUnauthenticated(false) to reject those
	// as well.
	authChain auth.Chain
	// allowUnauthenticated controls the empty-chain fallback. Default
	// true for back-compat with dev/test; production always has at
	// least the mgmt-plane validator in the chain, so this flag is a
	// no-op in deployed setups.
	allowUnauthenticated bool
	log                  logr.Logger

	mu           sync.RWMutex
	connections  map[*websocket.Conn]*Connection
	shutdown     bool
	completionWg sync.WaitGroup
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

// WithRecordingPool sets the recording worker pool for async session recording.
func WithRecordingPool(p *RecordingPool) ServerOption {
	return func(s *Server) {
		s.recordingPool = p
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

// WithPolicyFetcher sets the policy fetcher used to retrieve the effective
// recording policy per connection. When nil, all recording is enabled (default).
func WithPolicyFetcher(f PolicyFetcher) ServerOption {
	return func(s *Server) {
		s.policyFetcher = f
	}
}

// WithMgmtPlaneValidator configures the server to run a single mgmt-plane
// Validator. Convenience wrapper around WithAuthChain — exists to keep
// the PR 1a/c API stable while the wider chain machinery (PR 2b+) lands.
//
// Identical semantics to WithAuthChain(auth.Chain{v}): the validator
// runs first; ErrNoCredential falls through to the unauthenticated
// upgrade path (PR 1 default); invalid/expired returns 401. Combine with
// WithAuthChain instead of this option once data-plane validators are in
// the mix.
func WithMgmtPlaneValidator(v auth.Validator) ServerOption {
	return WithAuthChain(auth.Chain{v})
}

// WithAuthChain configures the server to run the supplied auth chain on
// every upgrade. Admit attaches the resulting identity to the
// connection's PropagationFields. ErrNoCredential (no validator admits)
// now returns 401 before Upgrade — PR 3 flipped this from the
// behaviour-preserving default of proceeding unauthenticated, closing
// pen-test finding C-3.
//
// Validator order matters — the first validator that admits wins, so
// list the most specific credential style first. The conventional order
// shipped by cmd/agent is sharedToken → apiKeys → oidc → edgeTrust →
// mgmt-plane.
//
// Empty chain still proceeds unauthenticated to keep the dev/test path
// working when no validator can be constructed (no mgmt-plane key, no
// externalAuth CRD). Set WithAllowUnauthenticated(false) at the server
// to also reject those requests.
func WithAuthChain(chain auth.Chain) ServerOption {
	return func(s *Server) {
		s.authChain = chain
	}
}

// WithAllowUnauthenticated controls the fallback behaviour when the
// auth chain is empty (no validators configured). Defaults to true so
// standalone dev/test binaries without a k8s client or mgmt-plane key
// still accept WebSocket upgrades. Production deployments going through
// cmd/agent always have at least the mgmt-plane validator in the chain,
// so this flag does not affect them — they 401 on missing credentials
// regardless.
//
// Set to false to reject every unauthenticated upgrade including the
// empty-chain case. Useful for integration tests that want to prove the
// strict default.
func WithAllowUnauthenticated(allow bool) ServerOption {
	return func(s *Server) {
		s.allowUnauthenticated = allow
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
		// Default true so dev/test binaries keep working without an
		// auth chain configured. Production deployments always have
		// at least mgmt-plane in the chain so this flag is a no-op
		// for them — the PR 3 flip applies via the chain's 401 on
		// ErrNoCredential regardless of this bool.
		allowUnauthenticated: true,
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

// submitCompletion runs a task through the recording pool if available,
// otherwise as a tracked goroutine. All tasks are waited on during Shutdown.
// After Shutdown has been called, new tasks are silently dropped to avoid
// a WaitGroup Add/Wait race.
func (s *Server) submitCompletion(task func()) {
	if s.recordingPool != nil {
		s.recordingPool.Submit(task)
		return
	}
	s.mu.RLock()
	if s.shutdown {
		s.mu.RUnlock()
		return
	}
	s.completionWg.Add(1)
	s.mu.RUnlock()
	go func() {
		defer s.completionWg.Done()
		task()
	}()
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
		"originHash", logging.HashID(origin))
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

// authenticateRequest runs the configured auth chain against the request.
// Returns the admitted identity, or an error when no validator admits.
//
// PR 3 flipped the ErrNoCredential branch from "proceed unauthenticated"
// to "return 401". Empty chain still proceeds iff allowUnauthenticated
// is true (default); production deployments always have at least the
// mgmt-plane validator in the chain so the empty-chain path is a
// dev/test escape hatch.
//
// Returning (nil, nil) means "proceed without identity" — callers
// should treat this as the unauthenticated-but-allowed path. Returning
// a non-nil error means "reject" — callers translate to 401.
func (s *Server) authenticateRequest(r *http.Request) (*policy.AuthenticatedIdentity, error) {
	if len(s.authChain) == 0 {
		if s.allowUnauthenticated {
			return nil, nil
		}
		return nil, auth.ErrNoCredential
	}
	id, err := s.authChain.Run(r.Context(), r)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, auth.ErrNoCredential):
		// PR 3: no validator admitted. Production = 401. The PR 1
		// back-compat pass-through is gone.
		return nil, err
	default:
		// ErrInvalidCredential / ErrExpired / anything else — surface as
		// a rejection so the caller returns 401.
		return nil, err
	}
}

// ServeHTTP handles WebSocket upgrade requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	if s.shutdown {
		s.mu.RUnlock()
		http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	if s.config.MaxConnections > 0 && len(s.connections) >= s.config.MaxConnections {
		s.mu.RUnlock()
		http.Error(w, "connection limit reached", http.StatusServiceUnavailable)
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
	workspaceName := r.URL.Query().Get("workspace")
	if workspaceName == "" {
		workspaceName = s.config.WorkspaceName
	}

	// Check if client requests binary frame support
	binaryCapable := r.URL.Query().Get("binary") == "true"

	if agentName == "" {
		http.Error(w, "agent parameter is required", http.StatusBadRequest)
		return
	}

	// Run the auth chain (PR 1: mgmt-plane validator only). On admit the
	// returned identity takes precedence over Istio-injected headers for
	// user fields; on unambiguous reject (invalid/expired) 401 here and
	// skip the upgrade entirely.
	authIdentity, authErr := s.authenticateRequest(r)
	if authErr != nil {
		s.log.V(1).Info("auth rejected upgrade",
			"reason", authErr.Error(),
			"status", http.StatusUnauthorized,
		)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract user identity. When an auth validator admitted the request
	// its Identity is the source of truth; otherwise fall back to the
	// Istio-injected headers (preserved for deployments that currently
	// rely on the chart's authentication.enabled=true gate).
	var (
		rawUserID string
		userRoles string
		userEmail string
	)
	if authIdentity != nil {
		// Mgmt-plane JWTs identify the *dashboard operator* (the human
		// using "Try this agent"), not the end user whose memories /
		// sessions we're scoping. The dashboard always sends a device_id
		// query param on the WS upgrade — when the auth came from the
		// management plane, scope to that so memories saved during a
		// debug session show up in the user's "My Memories" view.
		// (The operator pseudonym still flows separately into audit
		// logs via authIdentity.Subject.)
		if authIdentity.Origin == policy.OriginManagementPlane {
			if dev := r.URL.Query().Get("device_id"); dev != "" {
				rawUserID = dev
			} else {
				rawUserID = authIdentity.EndUser
			}
		} else {
			rawUserID = authIdentity.EndUser
		}
		userRoles = authIdentity.Role
		userEmail = authIdentity.Claims["email"]
	} else {
		rawUserID = r.Header.Get(policy.IstioHeaderUserID)
		userRoles = r.Header.Get(policy.IstioHeaderUserRoles)
		userEmail = r.Header.Get(policy.IstioHeaderUserEmail)
	}
	if rawUserID == "" {
		rawUserID = r.URL.Query().Get("device_id")
	}
	userID := identity.PseudonymizeID(rawUserID)
	s.log.V(1).Info("user identity extracted",
		"hasRawUserID", rawUserID != "",
		"hasUserID", userID != "",
		"hasAuthIdentity", authIdentity != nil,
		"headerName", policy.IstioHeaderUserID,
	)
	authorization := r.Header.Get("Authorization")
	cohortID := r.Header.Get(policy.HeaderCohortID)
	variant := r.Header.Get(policy.HeaderVariant)

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
		workspaceName: workspaceName,
		binaryCapable: binaryCapable,
		userID:        userID,
		userRoles:     userRoles,
		userEmail:     userEmail,
		authorization: authorization,
		cohortID:      cohortID,
		variant:       variant,
	}
	if s.config.MessageRateLimit > 0 {
		c.rateLimiter = rate.NewLimiter(rate.Limit(s.config.MessageRateLimit), s.config.MessageRateBurst)
	}

	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.connections[conn] = c
	s.mu.Unlock()

	// Record connection metrics
	s.metrics.ConnectionOpened()

	// Create enriched context with connection info
	connCtx := logctx.WithAgent(context.Background(), agentName)
	connCtx = logctx.WithNamespace(connCtx, namespace)
	connCtx = logctx.WithRequestID(connCtx, uuid.New().String())

	// Extract W3C trace context (traceparent/tracestate) from upgrade headers.
	// If no traceparent header is present, the context is unchanged (no-op).
	connCtx = otel.GetTextMapPropagator().Extract(connCtx, propagation.HeaderCarrier(r.Header))

	// Store policy propagation fields for gRPC metadata forwarding.
	// When an auth validator admitted the request we attach the Identity
	// too — downstream in-process code can inspect it, but it does not
	// travel over gRPC (the flat UserID/UserRoles/UserEmail/Claims carry
	// what runtime needs, via ToOutboundHeaders).
	//
	// Also propagate the validator's claim map so downstream HTTP tool
	// calls see X-Omnia-Claim-<name> headers — ToolPolicy's requiredClaims
	// contract relies on this regardless of which validator admitted.
	var validatorClaims map[string]string
	if authIdentity != nil && len(authIdentity.Claims) > 0 {
		validatorClaims = authIdentity.Claims
	}
	connCtx = policy.WithPropagationFields(connCtx, &policy.PropagationFields{
		AgentName:     agentName,
		Namespace:     namespace,
		RequestID:     logctx.RequestID(connCtx),
		UserID:        userID,
		UserRoles:     userRoles,
		UserEmail:     userEmail,
		Authorization: authorization,
		Claims:        validatorClaims,
		Identity:      authIdentity,
	})

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

	// Drain the recording pool so in-flight writes complete
	if s.recordingPool != nil {
		s.recordingPool.Close()
	}

	// Wait for completion goroutines not routed through the pool
	s.completionWg.Wait()

	return nil
}

// ConnectionCount returns the number of active connections.
func (s *Server) ConnectionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

// HasMediaStorage reports whether media storage has been wired into the
// server via WithMediaStorage. Used by wiring tests in cmd/agent to assert
// that cmd/agent/websocket.go passes the storage through to the facade
// (otherwise the WebSocket upload_request flow fails with mediaStorage==nil).
func (s *Server) HasMediaStorage() bool {
	return s.mediaStorage != nil
}
