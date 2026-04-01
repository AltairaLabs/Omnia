package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/doctor"
)

const (
	wsHandshakeTimeout = 10 * time.Second
	wsResponseTimeout  = 60 * time.Second // Ollama can be slow

	// wsMessageTypeMessage is the client message type for a chat message.
	wsMessageTypeMessage = "message"
	// wsMessageTypeChunk is the server message type for a streaming chunk.
	wsMessageTypeChunk = "chunk"
	// wsMessageTypeDone is the server message type for stream completion.
	wsMessageTypeDone = "done"
	// wsMessageTypeToolCall is the server message type for a tool invocation.
	wsMessageTypeToolCall = "tool_call"
	// wsMessageTypeConnected is the server message type sent on connect.
	wsMessageTypeConnected = "connected"
)

// AgentConfig describes the agent to test.
type AgentConfig struct {
	FacadeURL     string // e.g., http://tools-demo.omnia-demo.svc.cluster.local:8080
	AgentName     string
	Namespace     string
	SessionAPIURL string // session-api URL for verifying tool calls after chat
}

// AgentChecker runs WebSocket-based agent checks.
type AgentChecker struct {
	config AgentConfig
	// LastSessionID stores the session ID from the most recent chat check.
	LastSessionID string
}

// NewAgentChecker creates a new AgentChecker with the given configuration.
func NewAgentChecker(config AgentConfig) *AgentChecker {
	return &AgentChecker{config: config}
}

// Checks returns the list of WebSocket agent checks.
func (a *AgentChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "WebSocketConnect", Category: "Agent", Run: a.checkConnect},
		{Name: "SendMessageGetResponse", Category: "Agent", Run: a.checkChat},
		{Name: "AgentUsesTools", Category: "Agent", Run: a.checkToolCalling},
	}
}

// wsClientMessage is the minimal outbound message shape.
type wsClientMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// wsServerMessage is the minimal inbound message shape.
type wsServerMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCall  *wsToolCallInfo `json:"tool_call,omitempty"`
}

// wsToolCallInfo holds the name of a server-side tool call.
type wsToolCallInfo struct {
	Name string `json:"name"`
}

// facadeURL converts the HTTP facade URL to a WebSocket URL with query params.
func (a *AgentChecker) facadeURL() string {
	base := strings.Replace(a.config.FacadeURL, "http://", "ws://", 1)
	base = strings.Replace(base, "https://", "wss://", 1)
	return fmt.Sprintf("%s/ws?agent=%s&namespace=%s", base, a.config.AgentName, a.config.Namespace)
}

// dial opens a WebSocket connection and reads the initial "connected" message.
// Returns the connection and the session ID provided by the server.
func (a *AgentChecker) dial(ctx context.Context) (*websocket.Conn, string, error) {
	dialer := websocket.Dialer{HandshakeTimeout: wsHandshakeTimeout}
	conn, _, err := dialer.DialContext(ctx, a.facadeURL(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("dial: %w", err)
	}

	sessionID, err := readConnected(conn)
	if err != nil {
		conn.Close() //nolint:errcheck
		return nil, "", fmt.Errorf("reading connected message: %w", err)
	}

	return conn, sessionID, nil
}

// readConnected reads messages until a "connected" message is received.
func readConnected(conn *websocket.Conn) (string, error) {
	if err := conn.SetReadDeadline(time.Now().Add(wsHandshakeTimeout)); err != nil {
		return "", err
	}
	for {
		var msg wsServerMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return "", err
		}
		if msg.Type == wsMessageTypeConnected {
			if err := conn.SetReadDeadline(time.Time{}); err != nil {
				return "", err
			}
			return msg.SessionID, nil
		}
	}
}

// sendMessage marshals and writes a chat message to the WebSocket.
func sendMessage(conn *websocket.Conn, content string) error {
	data, err := json.Marshal(wsClientMessage{Type: wsMessageTypeMessage, Content: content})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// collectResponse reads server messages until a "done" message or the context deadline.
// It returns all messages received (not including the initial "connected" message).
func collectResponse(ctx context.Context, conn *websocket.Conn) ([]wsServerMessage, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(wsResponseTimeout)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	var messages []wsServerMessage
	for {
		var msg wsServerMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return messages, fmt.Errorf("reading response: %w", err)
		}
		messages = append(messages, msg)
		if msg.Type == wsMessageTypeDone {
			return messages, nil
		}
	}
}

// closeConn sends a clean WebSocket close frame.
func closeConn(conn *websocket.Conn) {
	_ = conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	conn.Close() //nolint:errcheck
}

// checkConnect verifies that a WebSocket connection can be established.
func (a *AgentChecker) checkConnect(ctx context.Context) doctor.TestResult {
	conn, _, err := a.dial(ctx)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "WebSocket handshake failed",
		}
	}
	closeConn(conn)
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: "WebSocket connected successfully",
	}
}

// checkChat sends a greeting and verifies a non-empty response is received.
func (a *AgentChecker) checkChat(ctx context.Context) doctor.TestResult {
	chatCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, sessionID, err := a.dial(chatCtx)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "connection failed",
		}
	}
	defer closeConn(conn)

	if err := sendMessage(conn, "Hello, what can you help me with?"); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to send message",
		}
	}

	msgs, err := collectResponse(chatCtx, conn)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to receive response",
		}
	}

	text := assembleText(msgs)
	if text == "" {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "empty response from agent",
		}
	}

	if sessionID != "" {
		a.LastSessionID = sessionID
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("received response (%d chars)", len(text)),
	}
}

// checkToolCalling sends a weather prompt and verifies tool calls were recorded.
// Server-side tools (HTTP executors) are not forwarded via WebSocket, so we verify
// by checking the session-api tool-calls endpoint after the chat completes.
func (a *AgentChecker) checkToolCalling(ctx context.Context) doctor.TestResult {
	toolCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, sessionID, err := a.dial(toolCtx)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "connection failed",
		}
	}
	defer closeConn(conn)

	if err := sendMessage(conn, "What is the weather at latitude 51.5, longitude -0.12 right now?"); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to send message",
		}
	}

	if _, err := collectResponse(toolCtx, conn); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to receive response",
		}
	}

	// Verify tool calls via session-api (server-side tools aren't in WS stream).
	if a.config.SessionAPIURL == "" {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: "no session-api URL available",
		}
	}

	// The WS session may not be flushed to session-api yet. Query the most
	// recent session for this namespace to find tool calls.
	time.Sleep(3 * time.Second)
	resolvedID := a.resolveLatestSession(ctx)
	if resolvedID == "" {
		resolvedID = sessionID
	}
	if resolvedID == "" {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no session ID available to check tool calls",
		}
	}

	toolCalls, err := a.fetchToolCalls(ctx, resolvedID)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to fetch tool calls from session-api",
		}
	}

	if len(toolCalls) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("no tool calls recorded in session-api (session=%s)", sessionID),
		}
	}

	names := make([]string, 0, len(toolCalls))
	var errors []string
	for _, tc := range toolCalls {
		names = append(names, tc.Name)
		if tc.Status == "error" {
			errMsg := tc.ErrorMessage
			if errMsg == "" {
				errMsg = tc.Result
			}
			errors = append(errors, fmt.Sprintf("%s: %s", tc.Name, truncate(errMsg, 100)))
		}
	}

	if len(errors) > 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("tool calls failed: %v", errors),
		}
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("tool calls recorded: %v", names),
	}
}

// resolveLatestSession queries session-api for the most recent session in the agent's namespace.
func (a *AgentChecker) resolveLatestSession(ctx context.Context) string {
	url := fmt.Sprintf("%s/api/v1/sessions?namespace=%s&limit=1", a.config.SessionAPIURL, a.config.Namespace)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var result struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Sessions) == 0 {
		return ""
	}
	return result.Sessions[0].ID
}

// toolCallRecord is the shape returned by session-api /tool-calls endpoint.
type toolCallRecord struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Result       string `json:"result,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// fetchToolCalls queries session-api for tool calls in a given session.
func (a *AgentChecker) fetchToolCalls(ctx context.Context, sessionID string) ([]toolCallRecord, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/tool-calls", a.config.SessionAPIURL, sessionID)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // session or tool calls not found yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var calls []toolCallRecord
	if err := json.NewDecoder(resp.Body).Decode(&calls); err != nil {
		return nil, err
	}
	return calls, nil
}

// assembleText concatenates the Content fields from all chunk and done messages.
func assembleText(msgs []wsServerMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		if m.Type == wsMessageTypeChunk || m.Type == wsMessageTypeDone {
			sb.WriteString(m.Content)
		}
	}
	return sb.String()
}

// truncate shortens s to at most n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
