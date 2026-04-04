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
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/policy"
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
	SessionAPIURL string        // session-api URL for resolving latest session (list endpoint)
	SessionStore  session.Store // session store for tool-call and provider-call queries
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
	// Set user identity header so memories are stored with a user_id scope.
	// Without this, the memory-api rejects saves (user_id is required).
	headers := http.Header{}
	headers.Set(policy.IstioHeaderUserID, "doctor-smoke-test")
	dialer := websocket.Dialer{HandshakeTimeout: wsHandshakeTimeout}
	conn, _, err := dialer.DialContext(ctx, a.facadeURL(), headers)
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
	// json.Marshal of wsClientMessage (two string fields) cannot fail.
	data, _ := json.Marshal(wsClientMessage{Type: wsMessageTypeMessage, Content: content})
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

// chatWithAgent dials the facade, sends a message, waits for the response, and returns
// the session ID, assembled messages, and any error as a TestResult.
// The connection is opened and closed within the helper; the caller's ctx is not modified.
func (a *AgentChecker) chatWithAgent(ctx context.Context, message string) (sessionID string, msgs []wsServerMessage, fail *doctor.TestResult) {
	chatCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, sid, err := a.dial(chatCtx)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "connection failed"}
		return "", nil, &r
	}
	defer closeConn(conn)

	if err := sendMessage(conn, message); err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "send failed"}
		return sid, nil, &r
	}

	collected, err := collectResponse(chatCtx, conn)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "receive failed"}
		return sid, collected, &r
	}

	return sid, collected, nil
}

// checkChat sends a greeting and verifies a non-empty response is received.
func (a *AgentChecker) checkChat(ctx context.Context) doctor.TestResult {
	sessionID, msgs, fail := a.chatWithAgent(ctx, "Hello, what can you help me with?")
	if fail != nil {
		return *fail
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
	sessionID, _, fail := a.chatWithAgent(ctx, "What is the weather at latitude 51.5, longitude -0.12 right now?")
	if fail != nil {
		return *fail
	}

	// Verify tool calls via session store (server-side tools aren't in WS stream).
	if a.config.SessionStore == nil {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: "no session store available",
		}
	}

	// Resolve the session ID — prefer the latest from session-api list (known persisted).
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

	toolCalls, err := a.config.SessionStore.GetToolCalls(ctx, resolvedID, 0, 0)
	if err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  err.Error(),
			Detail: "failed to fetch tool calls from session store",
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
		if tc.Status == session.ToolCallStatusError {
			errMsg := tc.ErrorMessage
			if errMsg == "" {
				errMsg = toolCallResultString(tc.Result)
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

// toolCallResultString converts a session.ToolCall.Result (any) to a string
// for display in error messages.
func toolCallResultString(result any) string {
	if result == nil {
		return ""
	}
	if s, ok := result.(string); ok {
		return s
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result)
	}
	return string(data)
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
