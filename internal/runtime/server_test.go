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

package runtime

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/runtime/tracing"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

func TestNewServer(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath("/test/pack.json"),
		WithPromptName("test"),
	)

	require.NotNil(t, server)
	assert.Equal(t, "/test/pack.json", server.packPath)
	assert.Equal(t, "test", server.promptName)
	assert.True(t, server.healthy)
}

func TestServer_Health(t *testing.T) {
	server := NewServer()

	// Initially healthy
	resp, err := server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "ready", resp.Status)

	// Set unhealthy
	server.SetHealthy(false)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Equal(t, "not ready", resp.Status)

	// Set healthy again
	server.SetHealthy(true)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
}

func TestServer_Close(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
	)

	// Close should work even with no conversations
	err := server.Close()
	assert.NoError(t, err)
}

func TestServerOptions(t *testing.T) {
	t.Run("WithToolsConfig", func(t *testing.T) {
		server := NewServer(
			WithToolsConfig("/path/to/tools.yaml"),
		)
		assert.Equal(t, "/path/to/tools.yaml", server.toolsConfigPath)
	})

	t.Run("WithTracingProvider", func(t *testing.T) {
		// Passing nil is valid for testing
		server := NewServer(
			WithTracingProvider(nil),
		)
		assert.Nil(t, server.tracingProvider)
	})

	t.Run("WithMetrics", func(t *testing.T) {
		// Create metrics and set them
		metrics := NewMetrics("test-agent", "test-ns")
		server := NewServer(
			WithMetrics(metrics),
		)
		assert.NotNil(t, server.metrics)
		assert.Equal(t, metrics, server.metrics)
	})

	t.Run("WithProviderInfo", func(t *testing.T) {
		server := NewServer(
			WithProviderInfo("anthropic", "claude-3-opus"),
		)
		assert.Equal(t, "anthropic", server.providerType)
		assert.Equal(t, "claude-3-opus", server.model)
	})

	t.Run("WithStateStore", func(t *testing.T) {
		// Create a mock state store (just test the option sets the field)
		server := NewServer(
			WithStateStore(nil), // nil is valid for testing option behavior
		)
		// The option should have appended SDK options
		assert.NotNil(t, server)
		assert.Len(t, server.sdkOptions, 1)
	})

	t.Run("WithSDKOptions", func(t *testing.T) {
		server := NewServer(
			WithSDKOptions(), // Empty options
		)
		assert.NotNil(t, server)
	})

	t.Run("WithMockProvider", func(t *testing.T) {
		server := NewServer(
			WithMockProvider(true),
		)
		assert.True(t, server.mockProvider)

		server2 := NewServer(
			WithMockProvider(false),
		)
		assert.False(t, server2.mockProvider)
	})

	t.Run("WithMockConfigPath", func(t *testing.T) {
		server := NewServer(
			WithMockConfigPath("/path/to/mock.yaml"),
		)
		assert.Equal(t, "/path/to/mock.yaml", server.mockConfigPath)
	})

	t.Run("WithModel", func(t *testing.T) {
		server := NewServer(
			WithModel("claude-3-opus-20240229"),
		)
		// WithModel adds to sdkOptions, verify it was added
		assert.Len(t, server.sdkOptions, 1)

		// Empty model should not add an option
		server2 := NewServer(
			WithModel(""),
		)
		assert.Len(t, server2.sdkOptions, 0)
	})

	t.Run("AllOptionsCombined", func(t *testing.T) {
		server := NewServer(
			WithLogger(logr.Discard()),
			WithPackPath("/test/pack.json"),
			WithPromptName("assistant"),
			WithModel("claude-3-opus-20240229"),
			WithMockProvider(true),
			WithMockConfigPath("/mock.yaml"),
		)
		assert.NotNil(t, server)
		assert.Equal(t, "/test/pack.json", server.packPath)
		assert.Equal(t, "assistant", server.promptName)
		assert.True(t, server.mockProvider)
		assert.Equal(t, "/mock.yaml", server.mockConfigPath)
		// Model option was added
		assert.Len(t, server.sdkOptions, 1)
	})
}

// mockConverseStream implements RuntimeService_ConverseServer for testing.
type mockConverseStream struct {
	runtimev1.RuntimeService_ConverseServer
	ctx          context.Context
	recvMessages []*runtimev1.ClientMessage
	recvIndex    int
	sentMessages []*runtimev1.ServerMessage
	recvErr      error
	sendErr      error
}

func newMockStream(ctx context.Context, messages []*runtimev1.ClientMessage) *mockConverseStream {
	return &mockConverseStream{
		ctx:          ctx,
		recvMessages: messages,
		sentMessages: make([]*runtimev1.ServerMessage, 0),
	}
}

func (m *mockConverseStream) Context() context.Context {
	return m.ctx
}

func (m *mockConverseStream) Recv() (*runtimev1.ClientMessage, error) {
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	if m.recvIndex >= len(m.recvMessages) {
		return nil, context.Canceled // Simulate stream end
	}
	msg := m.recvMessages[m.recvIndex]
	m.recvIndex++
	return msg, nil
}

func (m *mockConverseStream) Send(msg *runtimev1.ServerMessage) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func TestServer_Converse_RecvError(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMockProvider(true),
	)

	stream := newMockStream(context.Background(), nil)
	stream.recvErr = context.Canceled

	err := server.Converse(stream)
	assert.Error(t, err)
}

func TestServer_Converse_EOF(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMockProvider(true),
	)

	// Empty stream - should return nil on EOF
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{})

	err := server.Converse(stream)
	// context.Canceled is returned when no more messages
	assert.Error(t, err)
}

func TestServer_Converse_ProcessMessageError(t *testing.T) {
	// Create a temp prompt pack file
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	// Create a minimal valid prompt pack
	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	// Send a message - should process with mock provider
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session", Content: "Hello"},
	})

	err = server.Converse(stream)
	// Should error because stream ends after processing
	assert.Error(t, err)

	// Should have sent responses (chunk and done, or error)
	assert.NotEmpty(t, stream.sentMessages)
}

func TestServer_Converse_SendError(t *testing.T) {
	// Create a temp prompt pack file
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session", Content: "Hello"},
	})
	stream.sendErr = context.DeadlineExceeded

	err = server.Converse(stream)
	// Stream ends, but no error from Converse itself (error sent to client)
	assert.Error(t, err)
}

func TestServer_Close_WithConversations(t *testing.T) {
	// Create a temp prompt pack file
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	// Create a conversation by sending a message
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "session-1", Content: "Hello"},
	})
	_ = server.Converse(stream)

	// Now close - should close the conversation
	err = server.Close()
	assert.NoError(t, err)

	// Verify conversations map is cleared
	assert.Empty(t, server.conversations)
}

func TestServer_GetOrCreateConversation_InvalidPack(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath("/nonexistent/pack.json"),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	// Try to get conversation with invalid pack path
	_, err := server.getOrCreateConversation(context.Background(), "test-session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open pack")
}

func TestServer_GetOrCreateConversation_Success(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	// First call - creates conversation
	conv1, err := server.getOrCreateConversation(context.Background(), "session-1")
	require.NoError(t, err)
	require.NotNil(t, conv1)

	// Second call - returns existing
	conv2, err := server.getOrCreateConversation(context.Background(), "session-1")
	require.NoError(t, err)
	assert.Equal(t, conv1, conv2) // Same pointer
}

func TestServer_GetOrCreateConversation_MockConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	mockConfigPath := tmpDir + "/mock.yaml"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	// Create mock config file
	mockConfig := `responses:
  - pattern: ".*"
    response: "This is a mock response"
`
	err = writeTestFile(t, mockConfigPath, mockConfig)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithMockConfigPath(mockConfigPath),
	)

	// Should create conversation with mock config
	conv, err := server.getOrCreateConversation(context.Background(), "session-1")
	require.NoError(t, err)
	require.NotNil(t, conv)
}

func TestServer_GetOrCreateConversation_InvalidMockConfig(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithMockConfigPath("/nonexistent/mock.yaml"),
	)

	// Should fail due to invalid mock config path
	_, err = server.getOrCreateConversation(context.Background(), "session-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load mock config")
}

// writeTestFile is a helper to write test files.
func writeTestFile(t *testing.T, path, content string) error {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func TestNewMetrics(t *testing.T) {
	metrics := NewMetrics("test-agent", "test-namespace")
	require.NotNil(t, metrics)
}

func TestServer_InitializeTools_NoConfig(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		// No tools config path set
	)

	err := server.InitializeTools(context.Background())
	assert.NoError(t, err)
	assert.False(t, server.toolsInitialized)
}

func TestServer_InitializeTools_InvalidConfig(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithToolsConfig("/nonexistent/tools.yaml"),
	)

	err := server.InitializeTools(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load tools config")
}

func TestServer_InitializeTools_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	toolsPath := tmpDir + "/tools.yaml"

	// Create a minimal valid tools config (empty handlers list)
	toolsConfig := `handlers: []
`
	err := writeTestFile(t, toolsPath, toolsConfig)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithToolsConfig(toolsPath),
	)

	err = server.InitializeTools(context.Background())
	assert.NoError(t, err)
	assert.True(t, server.toolsInitialized)
}

func TestServer_Close_WithToolManager(t *testing.T) {
	tmpDir := t.TempDir()
	toolsPath := tmpDir + "/tools.yaml"

	toolsConfig := `handlers: []
`
	err := writeTestFile(t, toolsPath, toolsConfig)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithToolsConfig(toolsPath),
	)

	// Initialize tools
	err = server.InitializeTools(context.Background())
	require.NoError(t, err)

	// Close should work
	err = server.Close()
	assert.NoError(t, err)
}

func TestServer_Close_WithTracingProvider(t *testing.T) {
	// Create a disabled tracing provider (no-op)
	provider, err := tracing.NewProvider(context.Background(), tracing.Config{
		Enabled: false,
	})
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithTracingProvider(provider),
	)

	// Close should shutdown tracing provider
	err = server.Close()
	assert.NoError(t, err)
}

func TestServer_Converse_WithTracingProvider(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	// Create a disabled tracing provider (no-op)
	provider, err := tracing.NewProvider(context.Background(), tracing.Config{
		Enabled: false,
	})
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithTracingProvider(provider),
	)
	defer func() { _ = server.Close() }()

	// Send a message - should process with tracing enabled
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session-tracing", Content: "Hello with tracing"},
	})

	err = server.Converse(stream)
	assert.Error(t, err) // Stream ends after message

	// Should have sent responses
	assert.NotEmpty(t, stream.sentMessages)
}

func TestServer_Converse_WithProviderInfo(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	// Test with provider info set (metrics skipped to avoid registration issues)
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithProviderInfo("mock", "mock-model"),
	)
	defer func() { _ = server.Close() }()

	// Verify provider info is set
	assert.Equal(t, "mock", server.providerType)
	assert.Equal(t, "mock-model", server.model)

	// Send a message - should process normally
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "test-session-provider", Content: "Hello with provider info"},
	})

	err = server.Converse(stream)
	assert.Error(t, err) // Stream ends after message

	// Should have sent responses
	assert.NotEmpty(t, stream.sentMessages)
}

func TestServer_Converse_MultipleSessions(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)
	defer func() { _ = server.Close() }()

	// Test multiple different sessions
	for i := 0; i < 3; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
			{SessionId: sessionID, Content: fmt.Sprintf("Hello from session %d", i)},
		})

		err = server.Converse(stream)
		assert.Error(t, err) // Stream ends after message
		assert.NotEmpty(t, stream.sentMessages)
	}

	// Verify all sessions are tracked
	assert.Len(t, server.conversations, 3)
}

func TestServer_Converse_ResumeSession(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)
	defer func() { _ = server.Close() }()

	sessionID := "resume-session"

	// First message creates the session
	stream1 := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: sessionID, Content: "First message"},
	})
	_ = server.Converse(stream1)

	// Second message reuses the same session
	stream2 := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: sessionID, Content: "Second message"},
	})
	_ = server.Converse(stream2)

	// Should still only have one conversation for this session
	assert.Len(t, server.conversations, 1)
}
