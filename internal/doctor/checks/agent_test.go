package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/doctor"
	"github.com/altairalabs/omnia/internal/session"
)

// testSessionID is the session ID returned by the mock facade.
const testSessionID = "sess-test-123"

// wsUpgrader is a permissive upgrader for tests.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// mockFacadeHandler describes how the mock facade should respond.
type mockFacadeHandler struct {
	// responses are the ServerMessages sent after receiving the client message.
	responses []wsServerMessage
	// skipConnected skips sending the initial connected message.
	skipConnected bool
	// closeAfterConnected closes the connection right after the connected message.
	closeAfterConnected bool
}

// serveMockFacade starts an httptest.Server that acts as a WebSocket facade.
func serveMockFacade(t *testing.T, h mockFacadeHandler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		if !h.skipConnected {
			connected := wsServerMessage{Type: wsMessageTypeConnected, SessionID: testSessionID}
			require.NoError(t, conn.WriteJSON(connected))
		}

		if h.closeAfterConnected {
			return
		}

		// Read one client message.
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}

		// Send responses.
		for _, resp := range h.responses {
			if err := conn.WriteJSON(resp); err != nil {
				return
			}
		}
	}))
	return srv
}

// httpToWS converts an http test server URL to a ws URL.
func httpToWS(url string) string {
	return strings.Replace(url, "http://", "ws://", 1)
}

// newCheckerForServer creates an AgentChecker pointing at the test server.
func newCheckerForServer(srv *httptest.Server) *AgentChecker {
	return NewAgentChecker(AgentConfig{
		FacadeURL: srv.URL,
		AgentName: "test-agent",
		Namespace: "test-ns",
	})
}

// MockStore is a minimal session.Store mock for doctor check tests.
// Only GetToolCalls and GetProviderCalls are used; other methods panic.
type MockStore struct {
	ToolCalls        []session.ToolCall
	ToolCallsErr     error
	ProviderCalls    []session.ProviderCall
	ProviderCallsErr error
}

func (m *MockStore) CreateSession(_ context.Context, _ session.CreateSessionOptions) (*session.Session, error) {
	panic("not used")
}
func (m *MockStore) GetSession(_ context.Context, _ string) (*session.Session, error) {
	panic("not used")
}
func (m *MockStore) DeleteSession(_ context.Context, _ string) error { panic("not used") }
func (m *MockStore) AppendMessage(_ context.Context, _ string, _ session.Message) error {
	panic("not used")
}
func (m *MockStore) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	panic("not used")
}
func (m *MockStore) RefreshTTL(_ context.Context, _ string, _ time.Duration) error {
	panic("not used")
}
func (m *MockStore) UpdateSessionStatus(_ context.Context, _ string, _ session.SessionStatusUpdate) error {
	panic("not used")
}
func (m *MockStore) RecordToolCall(_ context.Context, _ string, _ session.ToolCall) error {
	panic("not used")
}
func (m *MockStore) RecordProviderCall(_ context.Context, _ string, _ session.ProviderCall) error {
	panic("not used")
}
func (m *MockStore) GetToolCalls(_ context.Context, _ string, _, _ int) ([]session.ToolCall, error) {
	return m.ToolCalls, m.ToolCallsErr
}
func (m *MockStore) GetProviderCalls(_ context.Context, _ string, _, _ int) ([]session.ProviderCall, error) {
	return m.ProviderCalls, m.ProviderCallsErr
}
func (m *MockStore) RecordEvalResult(_ context.Context, _ string, _ session.EvalResult) error {
	panic("not used")
}
func (m *MockStore) RecordRuntimeEvent(_ context.Context, _ string, _ session.RuntimeEvent) error {
	panic("not used")
}
func (m *MockStore) GetRuntimeEvents(_ context.Context, _ string, _, _ int) ([]session.RuntimeEvent, error) {
	panic("not used")
}
func (m *MockStore) Close() error { return nil }

var _ session.Store = (*MockStore)(nil)

// --- Tests for facadeURL ---

func TestFacadeURL_HTTP(t *testing.T) {
	c := NewAgentChecker(AgentConfig{FacadeURL: "http://host:8080", AgentName: "my-agent", Namespace: "my-ns"})
	got := c.facadeURL()
	assert.Equal(t, "ws://host:8080/ws?agent=my-agent&namespace=my-ns", got)
}

func TestFacadeURL_HTTPS(t *testing.T) {
	c := NewAgentChecker(AgentConfig{FacadeURL: "https://host:443", AgentName: "a", Namespace: "b"})
	got := c.facadeURL()
	assert.Equal(t, "wss://host:443/ws?agent=a&namespace=b", got)
}

// --- checkConnect ---

func TestCheckConnect_Pass(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{})
	defer srv.Close()

	result := newCheckerForServer(srv).checkConnect(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckConnect_Fail_BadURL(t *testing.T) {
	c := NewAgentChecker(AgentConfig{FacadeURL: "http://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	result := c.checkConnect(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.NotEmpty(t, result.Error)
}

func TestCheckConnect_Fail_NoConnectedMessage(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{skipConnected: true, closeAfterConnected: true})
	defer srv.Close()

	result := newCheckerForServer(srv).checkConnect(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- checkChat ---

func TestCheckChat_Pass(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeChunk, Content: "Hello! I can help with"},
			{Type: wsMessageTypeChunk, Content: " many things."},
			{Type: wsMessageTypeDone, SessionID: testSessionID},
		},
	})
	defer srv.Close()

	checker := newCheckerForServer(srv)
	result := checker.checkChat(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Equal(t, testSessionID, checker.LastSessionID)
}

func TestCheckChat_Fail_EmptyResponse(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: ""},
		},
	})
	defer srv.Close()

	result := newCheckerForServer(srv).checkChat(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "empty response")
}

func TestCheckChat_Fail_ConnectionError(t *testing.T) {
	c := NewAgentChecker(AgentConfig{FacadeURL: "http://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	result := c.checkChat(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckChat_Fail_Timeout(t *testing.T) {
	// Server sends connected but never responds to the chat message.
	srv := serveMockFacade(t, mockFacadeHandler{closeAfterConnected: false}) // no responses
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result := newCheckerForServer(srv).checkChat(ctx)
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- checkToolCalling ---

func TestCheckToolCalling_Pass(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "The weather in London is 15°C."},
		},
	})
	defer facadeSrv.Close()

	c := newCheckerForServer(facadeSrv)
	c.config.SessionStore = &MockStore{
		ToolCalls: []session.ToolCall{
			{Name: "search_places", Status: session.ToolCallStatusSuccess},
			{Name: "get_weather", Status: session.ToolCallStatusSuccess},
		},
	}
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "search_places")
}

func TestCheckToolCalling_Fail_NoToolCalls(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "I can't do that."},
		},
	})
	defer facadeSrv.Close()

	c := newCheckerForServer(facadeSrv)
	c.config.SessionStore = &MockStore{ToolCalls: []session.ToolCall{}}
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no tool calls")
}

func TestCheckToolCalling_Skip_NoStore(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "Done."},
		},
	})
	defer facadeSrv.Close()

	c := newCheckerForServer(facadeSrv)
	// No SessionStore set
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

func TestCheckToolCalling_Fail_ConnectionError(t *testing.T) {
	c := NewAgentChecker(AgentConfig{FacadeURL: "http://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckToolCalling_Fail_ToolCallErrors(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "Here's the weather."},
		},
	})
	defer facadeSrv.Close()

	c := newCheckerForServer(facadeSrv)
	c.config.SessionStore = &MockStore{
		ToolCalls: []session.ToolCall{
			{Name: "get_weather", Status: session.ToolCallStatusError, ErrorMessage: "validation failed: latitude invalid"},
		},
	}
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "validation failed")
}

func TestCheckToolCalling_Fail_StoreError(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "Done."},
		},
	})
	defer facadeSrv.Close()

	c := newCheckerForServer(facadeSrv)
	c.config.SessionStore = &MockStore{ToolCallsErr: assert.AnError}
	result := c.checkToolCalling(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "failed to fetch tool calls")
}

func TestResolveLatestSession_Pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[{"id":"session-abc"}]}`))
	}))
	defer srv.Close()

	c := NewAgentChecker(AgentConfig{SessionAPIURL: srv.URL, Namespace: "test"})
	id := c.resolveLatestSession(context.Background())
	assert.Equal(t, "session-abc", id)
}

func TestResolveLatestSession_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	}))
	defer srv.Close()

	c := NewAgentChecker(AgentConfig{SessionAPIURL: srv.URL, Namespace: "test"})
	id := c.resolveLatestSession(context.Background())
	assert.Empty(t, id)
}

// --- helper unit tests ---

func TestAssembleText(t *testing.T) {
	msgs := []wsServerMessage{
		{Type: wsMessageTypeChunk, Content: "foo"},
		{Type: wsMessageTypeToolCall},
		{Type: wsMessageTypeChunk, Content: "bar"},
		{Type: wsMessageTypeDone, Content: "baz"},
	}
	assert.Equal(t, "foobarbaz", assembleText(msgs))
}

func TestToolCallResultString(t *testing.T) {
	assert.Equal(t, "", toolCallResultString(nil))
	assert.Equal(t, "hello", toolCallResultString("hello"))
	assert.Equal(t, `{"key":"val"}`, toolCallResultString(map[string]string{"key": "val"}))
	assert.Equal(t, "42", toolCallResultString(42))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 10))
	assert.Equal(t, "ab...", truncate("abcde", 2))
}

func TestChecks_ReturnsThreeChecks(t *testing.T) {
	c := NewAgentChecker(AgentConfig{})
	checks := c.Checks()
	require.Len(t, checks, 3)
	assert.Equal(t, "WebSocketConnect", checks[0].Name)
	assert.Equal(t, "SendMessageGetResponse", checks[1].Name)
	assert.Equal(t, "AgentUsesTools", checks[2].Name)
}

func TestSendMessage_WritesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		_, raw, err := conn.ReadMessage()
		require.NoError(t, err)

		var msg wsClientMessage
		require.NoError(t, json.Unmarshal(raw, &msg))
		assert.Equal(t, wsMessageTypeMessage, msg.Type)
		assert.Equal(t, "hello world", msg.Content)
	}))
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(httpToWS(srv.URL), nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	require.NoError(t, sendMessage(conn, "hello world"))
}
