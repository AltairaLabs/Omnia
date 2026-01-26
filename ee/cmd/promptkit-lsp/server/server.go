/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

// Config holds server configuration.
type Config struct {
	// Addr is the address for the main HTTP/WebSocket server.
	Addr string
	// HealthAddr is the address for health probes.
	HealthAddr string
	// DashboardAPIURL is the base URL for the dashboard API.
	DashboardAPIURL string
	// DevMode enables development mode (disables license validation).
	DevMode bool
}

// Server is the promptkit-lsp server.
type Server struct {
	config     Config
	log        logr.Logger
	httpServer *http.Server
	healthSrv  *http.Server
	upgrader   websocket.Upgrader
	documents  *DocumentStore
	validator  *Validator

	mu          sync.RWMutex
	connections map[*websocket.Conn]*Connection
	shutdown    bool
}

// Connection represents a WebSocket connection with LSP state.
type Connection struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	closed     bool
	workspace  string
	projectID  string
	pendingReq map[int]chan *Response
}

// New creates a new Server instance.
func New(cfg Config, log logr.Logger) (*Server, error) {
	// Create validator with embedded schemas
	validator, err := NewValidator(cfg.DashboardAPIURL, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	s := &Server{
		config:      cfg,
		log:         log.WithName("server"),
		documents:   NewDocumentStore(),
		validator:   validator,
		connections: make(map[*websocket.Conn]*Connection),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}

	return s, nil
}

// Start starts the HTTP server and health probe server.
func (s *Server) Start(ctx context.Context) error {
	// Setup HTTP mux for main server
	mux := http.NewServeMux()
	mux.HandleFunc("/lsp", s.handleWebSocket)
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/compile", s.handleCompile)

	s.httpServer = &http.Server{
		Addr:         s.config.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Setup health probe server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", s.handleHealthz)
	healthMux.HandleFunc("/readyz", s.handleReadyz)

	s.healthSrv = &http.Server{
		Addr:         s.config.HealthAddr,
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Start health server in goroutine
	go func() {
		s.log.Info("starting health probe server", "addr", s.config.HealthAddr)
		if err := s.healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error(err, "health server error")
		}
	}()

	// Start main server
	return s.httpServer.ListenAndServe()
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

	// Close all WebSocket connections
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

	// Shutdown health server
	if err := s.healthSrv.Shutdown(ctx); err != nil {
		s.log.Error(err, "health server shutdown error")
	}

	// Shutdown main server
	return s.httpServer.Shutdown(ctx)
}

// handleWebSocket handles WebSocket upgrade requests for LSP.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	if s.shutdown {
		s.mu.RUnlock()
		http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.mu.RUnlock()

	// Extract workspace and project from query params
	workspace := r.URL.Query().Get("workspace")
	projectID := r.URL.Query().Get("project")

	if workspace == "" || projectID == "" {
		http.Error(w, "workspace and project parameters are required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error(err, "failed to upgrade connection")
		return
	}

	c := &Connection{
		conn:       conn,
		workspace:  workspace,
		projectID:  projectID,
		pendingReq: make(map[int]chan *Response),
	}

	s.mu.Lock()
	s.connections[conn] = c
	s.mu.Unlock()

	s.log.Info("new LSP connection", "workspace", workspace, "project", projectID)

	// Handle connection in goroutine
	go s.handleConnection(r.Context(), c)
}

// handleConnection handles an LSP connection.
func (s *Server) handleConnection(ctx context.Context, c *Connection) {
	defer s.cleanupConnection(c)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.log.Error(err, "websocket read error")
			}
			return
		}

		// Parse JSON-RPC message
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			s.log.Error(err, "failed to parse message")
			continue
		}

		// Handle message
		s.handleMessage(ctx, c, &msg)
	}
}

// cleanupConnection removes a connection from tracking.
func (s *Server) cleanupConnection(c *Connection) {
	s.mu.Lock()
	delete(s.connections, c.conn)
	s.mu.Unlock()

	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	if err := c.conn.Close(); err != nil {
		s.log.Error(err, "error closing connection")
	}

	s.log.Info("LSP connection closed", "workspace", c.workspace, "project", c.projectID)
}

// handleMessage processes an LSP message.
func (s *Server) handleMessage(ctx context.Context, c *Connection, msg *Message) {
	log := s.log.WithValues("method", msg.Method, "id", msg.ID)

	switch msg.Method {
	case "initialize":
		s.handleInitialize(ctx, c, msg)
	case "initialized":
		// Client acknowledgment, no response needed
	case "shutdown":
		s.handleShutdown(ctx, c, msg)
	case "exit":
		_ = c.conn.Close()
	case "textDocument/didOpen":
		s.handleDidOpen(ctx, c, msg)
	case "textDocument/didChange":
		s.handleDidChange(ctx, c, msg)
	case "textDocument/didClose":
		s.handleDidClose(ctx, c, msg)
	case "textDocument/completion":
		s.handleCompletion(ctx, c, msg)
	case "textDocument/hover":
		s.handleHover(ctx, c, msg)
	case "textDocument/definition":
		s.handleDefinition(ctx, c, msg)
	default:
		log.V(1).Info("unhandled method")
		// Send method not found error for requests
		if msg.ID != nil {
			s.sendError(c, msg.ID, -32601, "Method not found", nil)
		}
	}
}

// handleHealthz handles the liveness probe.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReadyz handles the readiness probe.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	// Check if server is shutting down
	s.mu.RLock()
	shutdown := s.shutdown
	s.mu.RUnlock()

	if shutdown {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// sendResponse sends a JSON-RPC response.
func (s *Server) sendResponse(c *Connection, id any, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if err := c.conn.WriteJSON(resp); err != nil {
		s.log.Error(err, "failed to send response")
	}
}

// sendError sends a JSON-RPC error response.
func (s *Server) sendError(c *Connection, id any, code int, message string, data any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ResponseError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if err := c.conn.WriteJSON(resp); err != nil {
		s.log.Error(err, "failed to send error")
	}
}

// sendNotification sends a JSON-RPC notification.
func (s *Server) sendNotification(c *Connection, method string, params any) {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if err := c.conn.WriteJSON(notif); err != nil {
		s.log.Error(err, "failed to send notification")
	}
}
