package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// TestServer_MockScriptedToolCall_DispatchesExecutor reproduces the
// policy-broker E2E gap: a mock provider scripts a tool_calls turn for a
// server-side ("http") tool defined only in the ToolRegistry (NOT declared in
// the pack prompt's allowed_tools). The runtime must dispatch the tool to the
// backend, not return the mock's defaultResponse text.
func TestServer_MockScriptedToolCall_DispatchesExecutor(t *testing.T) {
	// --- Echo upstream backend: records that it was hit. ---
	var echoHits atomic.Int32
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		echoHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer echo.Close()

	tmpDir := t.TempDir()

	// --- Pack with NO allowed_tools declared on the prompt (mirrors the E2E). ---
	packPath := filepath.Join(tmpDir, "pack.promptpack")
	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": { "version": "v1", "syntax": "{{variable}}" },
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test agent that uses the echo tool."
			}
		}
	}`
	require.NoError(t, writeTestFile(t, packPath, packContent))

	// --- Mock config: turn 1 scripts an echo tool_call; fallback is text. ---
	mockPath := filepath.Join(tmpDir, "mock-responses.yaml")
	mockConfig := `defaultResponse: "fallback text"
scenarios:
  default:
    turns:
      1:
        type: tool_calls
        content: ""
        tool_calls:
          - name: echo
            arguments:
              amount: 100
      2:
        content: "Request processed."
`
	require.NoError(t, writeTestFile(t, mockPath, mockConfig))

	// --- Tools config: an http handler named "echo" pointing at the backend. ---
	toolsPath := filepath.Join(tmpDir, "tools.yaml")
	toolsConfig := `handlers:
  - name: echo
    type: http
    httpConfig:
      endpoint: "` + echo.URL + `"
      method: POST
      contentType: application/json
    tool:
      name: echo
      description: An echo tool that accepts an amount and forwards it upstream
      inputSchema:
        type: object
        properties:
          amount:
            type: number
        required: [amount]
    timeout: "10s"
`
	require.NoError(t, writeTestFile(t, toolsPath, toolsConfig))

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithMockConfigPath(mockPath),
		WithToolsConfig(toolsPath),
	)
	require.NoError(t, server.InitializeTools(context.Background()))
	t.Cleanup(func() { _ = server.Close() })

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "sess-1", Content: "Please process this transaction using the echo tool."},
	})

	_ = server.Converse(stream)

	if got := echoHits.Load(); got == 0 {
		t.Fatalf("echo backend was NEVER hit: the scripted mock tool_call did not dispatch the server tool (got defaultResponse instead)")
	}
}
