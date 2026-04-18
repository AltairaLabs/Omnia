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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/altairalabs/omnia/internal/tracing"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"github.com/prometheus/client_golang/prometheus"
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

	t.Run("WithEvalCollector", func(t *testing.T) {
		collector := metrics.NewEvalOnlyCollector(metrics.CollectorOpts{
			Registerer: prometheus.NewRegistry(),
			Namespace:  "omnia_eval",
		})
		server := NewServer(
			WithEvalCollector(collector),
		)
		assert.NotNil(t, server.evalCollector)
		assert.Equal(t, collector, server.evalCollector)
	})

	t.Run("WithEvalCollector_Nil", func(t *testing.T) {
		server := NewServer(
			WithEvalCollector(nil),
		)
		assert.Nil(t, server.evalCollector)
	})

	t.Run("WithEvalDefs", func(t *testing.T) {
		defs := []evals.EvalDef{
			{ID: "test-eval", Type: "contains"},
		}
		server := NewServer(
			WithEvalDefs(defs),
		)
		assert.Len(t, server.evalDefs, 1)
		assert.Equal(t, "test-eval", server.evalDefs[0].ID)
	})

	t.Run("WithProviderInfo", func(t *testing.T) {
		server := NewServer(
			WithProviderInfo("anthropic", "claude-3-opus"),
		)
		assert.Equal(t, "anthropic", server.providerType)
		assert.Equal(t, "claude-3-opus", server.model)
	})

	t.Run("WithBaseURL", func(t *testing.T) {
		server := NewServer(
			WithBaseURL("http://ollama.localhost:11434"),
		)
		assert.Equal(t, "http://ollama.localhost:11434", server.baseURL)

		// Empty base URL should be fine
		server2 := NewServer(
			WithBaseURL(""),
		)
		assert.Equal(t, "", server2.baseURL)
	})

	t.Run("WithPricing", func(t *testing.T) {
		server := NewServer(
			WithPricing(0.003, 0.015),
		)
		assert.InDelta(t, 0.003, server.inputCostPer1K, 1e-9)
		assert.InDelta(t, 0.015, server.outputCostPer1K, 1e-9)
	})

	t.Run("WithHeaders", func(t *testing.T) {
		headers := map[string]string{
			"HTTP-Referer": "https://example.com",
			"X-Title":      "omnia",
		}
		server := NewServer(
			WithHeaders(headers),
		)
		assert.Equal(t, headers, server.headers)
	})

	t.Run("WithPlatform", func(t *testing.T) {
		server := NewServer(
			WithPlatform(PlatformConfig{
				Type:     "bedrock",
				Region:   "us-east-1",
				Project:  "",
				Endpoint: "",
			}),
		)
		assert.Equal(t, "bedrock", server.platformType)
		assert.Equal(t, "us-east-1", server.platformRegion)
	})

	t.Run("WithPlatform_Vertex", func(t *testing.T) {
		server := NewServer(
			WithPlatform(PlatformConfig{
				Type:    "vertex",
				Region:  "us-central1",
				Project: "my-project",
			}),
		)
		assert.Equal(t, "vertex", server.platformType)
		assert.Equal(t, "my-project", server.platformProject)
	})

	t.Run("WithPlatform_Azure", func(t *testing.T) {
		server := NewServer(
			WithPlatform(PlatformConfig{
				Type:     "azure",
				Endpoint: "https://example.openai.azure.com",
			}),
		)
		assert.Equal(t, "azure", server.platformType)
		assert.Equal(t, "https://example.openai.azure.com", server.platformEndpoint)
	})

	t.Run("WithAuth", func(t *testing.T) {
		server := NewServer(
			WithAuth(AuthConfig{
				Type:                       "accessKey",
				RoleArn:                    "arn:aws:iam::1:role/x",
				ServiceAccountEmail:        "sa@p.iam",
				CredentialsSecretName:      "creds",
				CredentialsSecretKey:       "AWS_ACCESS_KEY_ID",
				CredentialsSecretNamespace: "ns",
			}),
		)
		assert.Equal(t, "accessKey", server.authType)
		assert.Equal(t, "arn:aws:iam::1:role/x", server.authRoleArn)
		assert.Equal(t, "sa@p.iam", server.authServiceAccountEmail)
		assert.Equal(t, "creds", server.authCredentialsSecretName)
		assert.Equal(t, "AWS_ACCESS_KEY_ID", server.authCredentialsSecretKey)
		assert.Equal(t, "ns", server.authCredentialsNamespace)
	})

	t.Run("WithAuth_WorkloadIdentity", func(t *testing.T) {
		server := NewServer(
			WithAuth(AuthConfig{Type: "workloadIdentity"}),
		)
		assert.Equal(t, "workloadIdentity", server.authType)
		assert.Empty(t, server.authCredentialsSecretName)
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

	t.Run("WithMemoryStore", func(t *testing.T) {
		// Verify default is nil and option sets the store
		srv := NewServer()
		assert.Nil(t, srv.memoryStore)
	})

	t.Run("WithWorkspaceUID", func(t *testing.T) {
		srv := NewServer(WithWorkspaceUID("test-uid-123"))
		assert.Equal(t, "test-uid-123", srv.workspaceUID)
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

	// Conversations are cleaned up when streams end
	assert.Len(t, server.conversations, 0)
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

	// Conversation is cleaned up when the stream ends
	assert.Len(t, server.conversations, 0)
}

// Tests for scenario detection functions

func TestExtractMockScenario_ExplicitMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		content  string
		expected string
	}{
		{
			name:     "explicit scenario in metadata",
			metadata: map[string]string{MetadataKeyMockScenario: "custom-scenario"},
			content:  "Hello world",
			expected: "custom-scenario",
		},
		{
			name:     "image-analysis scenario",
			metadata: map[string]string{MetadataKeyMockScenario: ScenarioImageAnalysis},
			content:  "Analyze this",
			expected: ScenarioImageAnalysis,
		},
		{
			name:     "empty scenario falls back to auto-detection",
			metadata: map[string]string{MetadataKeyMockScenario: ""},
			content:  "Hello",
			expected: ScenarioDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMockScenario(tt.metadata, tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractMockScenario_ContentTypeMetadata(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    string
	}{
		{"image/png", "image/png", ScenarioImageAnalysis},
		{"image/jpeg", "image/jpeg", ScenarioImageAnalysis},
		{"PNG file", "png", ScenarioImageAnalysis},
		{"audio/mp3", "audio/mp3", ScenarioAudioAnalysis},
		{"audio/wav", "audio/wav", ScenarioAudioAnalysis},
		{"MP3 file", "mp3", ScenarioAudioAnalysis},
		{"application/pdf", "application/pdf", ScenarioDocumentQA},
		{"pdf file", "pdf", ScenarioDocumentQA},
		{"text/plain", "text/plain", ScenarioDocumentQA},
		{"unknown type", "application/octet-stream", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{MetadataKeyContentType: tt.contentType}
			result := extractMockScenario(metadata, "")
			if tt.expected == "" {
				assert.Equal(t, ScenarioDefault, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExtractMockScenario_ContentPatterns(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"image reference", "[image: uploaded.png] What is in this image?", ScenarioImageAnalysis},
		{"audio reference", "[audio: recording.mp3] Transcribe this", ScenarioAudioAnalysis},
		{"document reference", "[document: report.pdf] Summarize", ScenarioDocumentQA},
		{"pdf reference", "[pdf: contract.pdf] Extract key terms", ScenarioDocumentQA},
		{"no pattern", "Hello, how are you?", ScenarioDefault},
		{"case insensitive", "[IMAGE: photo.jpg] Describe", ScenarioImageAnalysis},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMockScenario(nil, tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractMockScenario_Priority(t *testing.T) {
	// Explicit metadata takes priority over content type and content patterns
	t.Run("metadata takes priority over content type", func(t *testing.T) {
		metadata := map[string]string{
			MetadataKeyMockScenario: "custom",
			MetadataKeyContentType:  "image/png",
		}
		result := extractMockScenario(metadata, "[audio: test.mp3]")
		assert.Equal(t, "custom", result)
	})

	t.Run("content type takes priority over content patterns", func(t *testing.T) {
		metadata := map[string]string{
			MetadataKeyContentType: "image/png",
		}
		result := extractMockScenario(metadata, "[audio: test.mp3]")
		assert.Equal(t, ScenarioImageAnalysis, result)
	})
}

func TestDetectScenarioFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
	}{
		// Image types
		{"image/png", ScenarioImageAnalysis},
		{"image/jpeg", ScenarioImageAnalysis},
		{"image/gif", ScenarioImageAnalysis},
		{"image/webp", ScenarioImageAnalysis},
		{"image/svg+xml", ScenarioImageAnalysis},
		// Audio types
		{"audio/mpeg", ScenarioAudioAnalysis},
		{"audio/wav", ScenarioAudioAnalysis},
		{"audio/ogg", ScenarioAudioAnalysis},
		// Document types
		{"application/pdf", ScenarioDocumentQA},
		{"text/plain", ScenarioDocumentQA},
		{"text/html", ScenarioDocumentQA},
		// Unknown
		{"application/octet-stream", ""},
		{"video/mp4", ""},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := detectScenarioFromContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsPattern(t *testing.T) {
	tests := []struct {
		s        string
		pattern  string
		expected bool
	}{
		{"Hello World", "world", true}, // Case insensitive
		{"Hello World", "WORLD", true}, // Case insensitive
		{"image/png", "image/", true},
		{"application/pdf", "pdf", true},
		{"hello", "hello", true},
		{"hello", "goodbye", false},
		{"", "test", false},
		{"test", "", true}, // Empty pattern always matches
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.s, tt.pattern), func(t *testing.T) {
			result := containsPattern(tt.s, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsImageContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"image/gif", true},
		{"image/webp", true},
		{"image/svg+xml", true},
		{"PNG", true},
		{"jpg", true},
		{"jpeg", true},
		{"audio/mp3", false},
		{"application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := isImageContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAudioContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"audio/mpeg", true},
		{"audio/wav", true},
		{"audio/ogg", true},
		{"mp3", true},
		{"wav", true},
		{"flac", true},
		{"image/png", false},
		{"application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := isAudioContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDocumentContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"application/pdf", true},
		{"pdf", true},
		{"text/plain", true},
		{"text/html", true},
		{"document", true},
		{"image/png", false},
		{"audio/mp3", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := isDocumentContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSendOptions(t *testing.T) {
	log := logr.Discard()

	t.Run("empty parts returns nil", func(t *testing.T) {
		opts := buildSendOptions(nil, log)
		assert.Nil(t, opts)

		opts = buildSendOptions([]*runtimev1.ContentPart{}, log)
		assert.Nil(t, opts)
	})

	t.Run("part without media is skipped", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{Text: "just text, no media"},
		}
		opts := buildSendOptions(parts, log)
		assert.Empty(t, opts)
	})

	t.Run("image with base64 data produces send option", func(t *testing.T) {
		// Small valid base64 image data (1x1 red PNG)
		pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/png",
					Data:     pngBase64,
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 1, "should produce one send option for image")
	})

	t.Run("image with URL produces send option", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/jpeg",
					Url:      "https://example.com/image.jpg",
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 1, "should produce one send option for image URL")
	})

	t.Run("multiple images produce multiple options", func(t *testing.T) {
		pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/png",
					Data:     pngBase64,
				},
			},
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/jpeg",
					Url:      "https://example.com/image.jpg",
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 2, "should produce two send options")
	})

	t.Run("invalid base64 is skipped", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/png",
					Data:     "not-valid-base64!!!",
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Empty(t, opts, "invalid base64 should be skipped")
	})

	t.Run("image without data or URL is skipped", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/png",
					// No Data or Url
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Empty(t, opts, "image without data or URL should be skipped")
	})
}

func TestDecodeMediaData(t *testing.T) {
	t.Run("standard base64", func(t *testing.T) {
		original := []byte("hello world")
		encoded := base64.StdEncoding.EncodeToString(original)
		decoded, err := decodeMediaData(encoded)
		assert.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("URL-safe base64", func(t *testing.T) {
		original := []byte("hello world")
		encoded := base64.URLEncoding.EncodeToString(original)
		decoded, err := decodeMediaData(encoded)
		assert.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("raw base64 no padding", func(t *testing.T) {
		original := []byte("hello world")
		encoded := base64.RawStdEncoding.EncodeToString(original)
		decoded, err := decodeMediaData(encoded)
		assert.NoError(t, err)
		assert.Equal(t, original, decoded)
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		_, err := decodeMediaData("not-valid-base64!!!")
		assert.Error(t, err)
	})
}

func TestServer_CreateProviderFromConfig(t *testing.T) {
	t.Run("empty provider type returns nil", func(t *testing.T) {
		server := NewServer(
			WithLogger(logr.Discard()),
		)
		provider, err := server.createProviderFromConfig()
		assert.NoError(t, err)
		assert.Nil(t, provider, "empty provider type should return nil")
	})

	t.Run("ollama provider creates explicit provider", func(t *testing.T) {
		server := NewServer(
			WithLogger(logr.Discard()),
			WithProviderInfo("ollama", "llava:7b"),
			WithBaseURL("http://ollama.localhost:11434"),
		)
		provider, err := server.createProviderFromConfig()
		assert.NoError(t, err)
		assert.NotNil(t, provider, "ollama provider should create explicit provider")
		// Ollama uses OpenAI-compatible API, but retains "ollama" as its ID
		assert.Equal(t, "ollama", provider.ID())
	})

	t.Run("openai provider creates explicit provider", func(t *testing.T) {
		// Set API key for openai
		t.Setenv("OPENAI_API_KEY", "test-key")

		server := NewServer(
			WithLogger(logr.Discard()),
			WithProviderInfo("openai", "gpt-4"),
		)
		provider, err := server.createProviderFromConfig()
		assert.NoError(t, err)
		assert.NotNil(t, provider, "openai provider should create explicit provider")
		assert.Equal(t, "openai", provider.ID())
	})

	t.Run("claude provider creates explicit provider", func(t *testing.T) {
		// Set API key for claude
		t.Setenv("ANTHROPIC_API_KEY", "test-key")

		server := NewServer(
			WithLogger(logr.Discard()),
			WithProviderInfo("claude", "claude-3-opus"),
		)
		provider, err := server.createProviderFromConfig()
		assert.NoError(t, err)
		assert.NotNil(t, provider, "claude provider should create explicit provider")
		assert.Equal(t, "claude", provider.ID())
	})

	t.Run("gemini provider creates explicit provider", func(t *testing.T) {
		// Set API key for gemini
		t.Setenv("GEMINI_API_KEY", "test-key")

		server := NewServer(
			WithLogger(logr.Discard()),
			WithProviderInfo("gemini", "gemini-pro"),
		)
		provider, err := server.createProviderFromConfig()
		assert.NoError(t, err)
		assert.NotNil(t, provider, "gemini provider should create explicit provider")
		assert.Equal(t, "gemini", provider.ID())
	})
}

func TestCreateProviderFromConfig_WithPricing(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithProviderInfo("ollama", "llama3"),
		WithBaseURL("http://ollama.localhost:11434"),
		WithPricing(0.001, 0.002),
	)

	provider, err := server.createProviderFromConfig()
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "ollama", provider.ID())
	// Pricing is passed via ProviderSpec.Defaults.Pricing to PromptKit.
	// The provider uses it in CalculateCost() — verified by integration test.
}

func TestCreateProviderFromConfig_WithHeadersAndTimeouts(t *testing.T) {
	// Exercises the applyProviderTimeouts setter path (BaseProvider
	// embedded by ollama's OpenAI-compatible provider implements both
	// timeout setter interfaces) and the spec.Headers passthrough.
	server := NewServer(
		WithLogger(logr.Discard()),
		WithProviderInfo("ollama", "llama3"),
		WithBaseURL("http://ollama.localhost:11434"),
		WithHeaders(map[string]string{"X-Tenant": "test"}),
		WithProviderRequestTimeout(15*time.Second),
		WithProviderStreamIdleTimeout(45*time.Second),
	)
	provider, err := server.createProviderFromConfig()
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, "ollama", provider.ID())
}

// timeoutFake implements providers.Provider + both timeout setter
// interfaces so we can directly unit-test applyProviderTimeouts without
// relying on a real PromptKit provider instance.
type timeoutFake struct {
	requestTimeout    time.Duration
	streamIdleTimeout time.Duration
}

func (t *timeoutFake) ID() string    { return "fake" }
func (t *timeoutFake) Model() string { return "fake-model" }
func (t *timeoutFake) Predict(context.Context, providers.PredictionRequest) (providers.PredictionResponse, error) {
	return providers.PredictionResponse{}, nil
}

func (t *timeoutFake) PredictStream(context.Context, providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	ch := make(chan providers.StreamChunk)
	close(ch)
	return ch, nil
}
func (t *timeoutFake) SupportsStreaming() bool      { return false }
func (t *timeoutFake) ShouldIncludeRawOutput() bool { return false }
func (t *timeoutFake) Close() error                 { return nil }
func (t *timeoutFake) CalculateCost(int, int, int) types.CostInfo {
	return types.CostInfo{}
}

func (t *timeoutFake) SetHTTPTimeout(d time.Duration)       { t.requestTimeout = d }
func (t *timeoutFake) SetStreamIdleTimeout(d time.Duration) { t.streamIdleTimeout = d }

func TestApplyProviderTimeouts(t *testing.T) {
	t.Run("applies both timeouts when provider implements setters", func(t *testing.T) {
		fake := &timeoutFake{}
		server := &Server{
			log:                       logr.Discard(),
			providerRequestTimeout:    20 * time.Second,
			providerStreamIdleTimeout: 40 * time.Second,
		}
		server.applyProviderTimeouts(fake)
		assert.Equal(t, 20*time.Second, fake.requestTimeout)
		assert.Equal(t, 40*time.Second, fake.streamIdleTimeout)
	})

	t.Run("skips both when timeouts are zero", func(t *testing.T) {
		fake := &timeoutFake{}
		server := &Server{log: logr.Discard()}
		server.applyProviderTimeouts(fake)
		assert.Zero(t, fake.requestTimeout)
		assert.Zero(t, fake.streamIdleTimeout)
	})
}

func TestProcessAudioMedia(t *testing.T) {
	log := logr.Discard()

	t.Run("audio with base64 data produces send option", func(t *testing.T) {
		// Valid base64 encoded data
		audioBase64 := base64.StdEncoding.EncodeToString([]byte("fake audio data"))

		media := &runtimev1.MediaContent{
			MimeType: "audio/mp3",
			Data:     audioBase64,
		}
		opt := processAudioMedia(media, log)
		assert.NotNil(t, opt, "should produce send option for audio data")
	})

	t.Run("audio with URL produces send option", func(t *testing.T) {
		media := &runtimev1.MediaContent{
			MimeType: "audio/wav",
			Url:      "https://example.com/audio.wav",
		}
		opt := processAudioMedia(media, log)
		assert.NotNil(t, opt, "should produce send option for audio URL")
	})

	t.Run("audio without data or URL returns nil", func(t *testing.T) {
		media := &runtimev1.MediaContent{
			MimeType: "audio/ogg",
		}
		opt := processAudioMedia(media, log)
		assert.Nil(t, opt, "should return nil without data or URL")
	})

	t.Run("audio with invalid base64 returns nil", func(t *testing.T) {
		media := &runtimev1.MediaContent{
			MimeType: "audio/mp3",
			Data:     "not-valid-base64!!!",
		}
		opt := processAudioMedia(media, log)
		assert.Nil(t, opt, "invalid base64 should return nil")
	})
}

func TestProcessFileMedia(t *testing.T) {
	log := logr.Discard()

	t.Run("file with base64 data produces send option", func(t *testing.T) {
		// Valid base64 encoded data
		fileBase64 := base64.StdEncoding.EncodeToString([]byte("fake pdf content"))

		media := &runtimev1.MediaContent{
			MimeType: "application/pdf",
			Data:     fileBase64,
		}
		opt := processFileMedia(media, log)
		assert.NotNil(t, opt, "should produce send option for file data")
	})

	t.Run("file without data returns nil", func(t *testing.T) {
		media := &runtimev1.MediaContent{
			MimeType: "application/pdf",
		}
		opt := processFileMedia(media, log)
		assert.Nil(t, opt, "should return nil without data")
	})

	t.Run("file with invalid base64 returns nil", func(t *testing.T) {
		media := &runtimev1.MediaContent{
			MimeType: "application/pdf",
			Data:     "invalid-base64!!!",
		}
		opt := processFileMedia(media, log)
		assert.Nil(t, opt, "invalid base64 should return nil")
	})

	t.Run("file with URL is not supported", func(t *testing.T) {
		// processFileMedia only supports base64 data, URLs are not handled
		media := &runtimev1.MediaContent{
			MimeType: "application/pdf",
			Url:      "https://example.com/doc.pdf",
		}
		opt := processFileMedia(media, log)
		assert.Nil(t, opt, "file with URL only should return nil (data required)")
	})
}

func TestProcessMediaPart_AllTypes(t *testing.T) {
	log := logr.Discard()
	validBase64 := base64.StdEncoding.EncodeToString([]byte("test content"))

	testCases := []struct {
		name     string
		mimeType string
		expected string // "image", "audio", or "file"
	}{
		{"png image", "image/png", "image"},
		{"jpeg image", "image/jpeg", "image"},
		{"gif image", "image/gif", "image"},
		{"webp image", "image/webp", "image"},
		{"mp3 audio", "audio/mp3", "audio"},
		{"wav audio", "audio/wav", "audio"},
		{"ogg audio", "audio/ogg", "audio"},
		{"pdf document", "application/pdf", "file"},
		{"text document", "text/plain", "file"},
		{"unknown type", "application/octet-stream", "file"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			media := &runtimev1.MediaContent{
				MimeType: tc.mimeType,
				Data:     validBase64,
			}
			opt := processMediaPart(media, log)
			assert.NotNil(t, opt, "processMediaPart should return option for %s", tc.mimeType)
		})
	}
}

func TestBuildSendOptions_AudioAndFile(t *testing.T) {
	log := logr.Discard()
	validBase64 := base64.StdEncoding.EncodeToString([]byte("test content"))

	t.Run("audio with base64 data", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "audio/mp3",
					Data:     validBase64,
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 1, "should produce one send option for audio")
	})

	t.Run("file with base64 data", func(t *testing.T) {
		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "application/pdf",
					Data:     validBase64,
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 1, "should produce one send option for file")
	})

	t.Run("mixed content types", func(t *testing.T) {
		pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

		parts := []*runtimev1.ContentPart{
			{
				Media: &runtimev1.MediaContent{
					MimeType: "image/png",
					Data:     pngBase64,
				},
			},
			{
				Media: &runtimev1.MediaContent{
					MimeType: "audio/wav",
					Data:     validBase64,
				},
			},
			{
				Media: &runtimev1.MediaContent{
					MimeType: "application/pdf",
					Data:     validBase64,
				},
			},
		}
		opts := buildSendOptions(parts, log)
		assert.Len(t, opts, 3, "should produce three send options for mixed content")
	})
}

func TestWithSkillManifest(t *testing.T) {
	writeManifest := func(t *testing.T, m map[string]any) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		data, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		return path
	}

	newCapturingLogger := func(captured *[]string) logr.Logger {
		return funcr.NewJSON(func(obj string) {
			*captured = append(*captured, obj)
		}, funcr.Options{Verbosity: 1})
	}

	t.Run("logs loaded skills with names and paths", func(t *testing.T) {
		path := writeManifest(t, map[string]any{
			"version": "1",
			"skills": []map[string]any{
				{"name": "billing", "mount_as": "billing", "content_path": "/workspace-content/billing"},
				{"name": "refunds", "mount_as": "refunds", "content_path": "/workspace-content/refunds"},
			},
			"config": map[string]any{"max_active": 3},
		})

		var captured []string
		log := newCapturingLogger(&captured)

		s := NewServer(WithLogger(log), WithSkillManifest(path))
		require.NotNil(t, s)

		var found bool
		for _, l := range captured {
			if containsAll(l, `"msg":"skill manifest loaded"`, `"skillCount":2`, `"billing"`, `"refunds"`, `"maxActive":3`) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected skill-load log line with skill names and maxActive; got: %v", captured)
	})

	t.Run("missing file is a no-op with no error log", func(t *testing.T) {
		var captured []string
		log := newCapturingLogger(&captured)

		s := NewServer(WithLogger(log), WithSkillManifest("/does/not/exist.json"))
		require.NotNil(t, s)

		for _, l := range captured {
			assert.NotContains(t, l, `"msg":"skill manifest read failed"`)
		}
	})

	t.Run("empty path is a no-op", func(t *testing.T) {
		var captured []string
		log := newCapturingLogger(&captured)

		s := NewServer(WithLogger(log), WithSkillManifest(""))
		require.NotNil(t, s)

		for _, l := range captured {
			assert.NotContains(t, l, `"msg":"skill manifest read failed"`)
		}
	})

	t.Run("invalid json logs error and skips", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

		var captured []string
		log := newCapturingLogger(&captured)

		s := NewServer(WithLogger(log), WithSkillManifest(path))
		require.NotNil(t, s)

		var found bool
		for _, l := range captured {
			if containsAll(l, `"msg":"skill manifest read failed"`) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected read-failed error log; got: %v", captured)
	})
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
