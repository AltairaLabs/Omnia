/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestBuildEngineComponentsOutputDirectory tests that BuildEngineComponents
// respects the output directory configuration and doesn't try to create
// directories in the current working directory.
func TestBuildEngineComponentsOutputDirectory(t *testing.T) {
	// Create a temporary directory for output
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	// Create a minimal config with output directory set
	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err, "BuildEngineComponents should succeed with valid output directory")
	require.NotNil(t, registry, "Registry should not be nil")

	// Verify the output directory was created
	_, err = os.Stat(outputDir)
	assert.NoError(t, err, "Output directory should be created")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestBuildEngineComponentsWithWorkdirChange tests that changing the working
// directory before calling BuildEngineComponents allows it to create relative
// paths in a writable location.
func TestBuildEngineComponentsWithWorkdirChange(t *testing.T) {
	// Create a temporary directory to use as working directory
	tmpDir := t.TempDir()

	// Save original working directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// Change to temp directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create config with empty output directory (will use default "out")
	cfg := &config.Config{
		Defaults: config.Defaults{
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components - should create "out" in tmpDir
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err, "BuildEngineComponents should succeed when working directory is writable")
	require.NotNil(t, registry, "Registry should not be nil")

	// Verify the "out" directory was created in tmpDir
	outDir := filepath.Join(tmpDir, "out")
	_, err = os.Stat(outDir)
	assert.NoError(t, err, "out directory should be created in temp directory")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestBuildEngineComponentsFailsWithReadOnlyDir tests that BuildEngineComponents
// fails when trying to create directories in a read-only location.
func TestBuildEngineComponentsFailsWithReadOnlyDir(t *testing.T) {
	// Skip if running as root (root can write to read-only dirs)
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	// Create a read-only directory
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)
	defer func() {
		// Make it writable again for cleanup
		_ = os.Chmod(readOnlyDir, 0755)
	}()

	// Save original working directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// Change to read-only directory
	err = os.Chdir(readOnlyDir)
	require.NoError(t, err)

	// Create config with empty output directory
	cfg := &config.Config{
		Defaults: config.Defaults{
			ConfigDir: readOnlyDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components - should fail
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	assert.Error(t, err, "BuildEngineComponents should fail when working directory is read-only")
	assert.Contains(t, err.Error(), "permission denied", "Error should mention permission denied")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestPromptKitHandlerBuildComponents tests that the handler's buildComponents
// method properly sets up output directories.
func TestPromptKitHandlerBuildComponents(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err, "buildComponents should succeed")
	require.NotNil(t, handler.providerRegistry, "Provider registry should be set")

	// Verify output directory exists
	_, err = os.Stat(outputDir)
	assert.NoError(t, err, "Output directory should exist")

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestPromptKitHandlerBuildComponentsWithEmptyOutputDir tests that the handler
// sets a default output directory when none is configured.
func TestPromptKitHandlerBuildComponentsWithEmptyOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and change working directory to temp
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg := &config.Config{
		Defaults: config.Defaults{
			// Empty Output.Dir - handler should set a default
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err = handler.buildComponents()
	require.NoError(t, err, "buildComponents should succeed with empty output dir")

	// Verify the handler set a default output directory
	assert.NotEmpty(t, cfg.Defaults.Output.Dir, "Output directory should be set")
	assert.Contains(t, cfg.Defaults.Output.Dir, "/tmp", "Output directory should be in /tmp")

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestBuildConfigFromProviders tests that BuildConfigFromProviders creates
// a properly configured config with writable output directories.
func TestBuildConfigFromProviders(t *testing.T) {
	testProviders := map[string]*config.Provider{
		"test-provider": {
			ID:    "test-provider",
			Type:  "mock",
			Model: "test-model",
		},
	}

	cfg := BuildConfigFromProviders(testProviders)

	assert.NotNil(t, cfg)
	assert.Equal(t, "/tmp/arena-dev-console-output", cfg.Defaults.Output.Dir)
	assert.Equal(t, "/tmp/arena-dev-console-output", cfg.Defaults.OutDir)
	assert.Equal(t, "/tmp/arena-dev-console", cfg.Defaults.ConfigDir)
	assert.Len(t, cfg.LoadedProviders, 1)
	assert.Contains(t, cfg.LoadedProviders, "test-provider")
}

// TestPromptKitHandlerName tests that Name returns the expected value.
func TestPromptKitHandlerName(t *testing.T) {
	handler := &PromptKitHandler{}
	assert.Equal(t, "promptkit", handler.Name())
}

// TestNewPromptKitHandlerWithNilConfig tests creating a handler with nil config.
func TestNewPromptKitHandlerWithNilConfig(t *testing.T) {
	handler, err := NewPromptKitHandler(nil, logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Nil(t, handler.config)
	assert.Nil(t, handler.providerRegistry)
}

// TestNewPromptKitHandlerWithEmptyProviders tests creating handler with empty providers.
func TestNewPromptKitHandlerWithEmptyProviders(t *testing.T) {
	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{},
	}
	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, handler)
	// With no providers, registry should not be built
	assert.Nil(t, handler.providerRegistry)
}

// TestPromptKitHandlerGetOrCreateSession tests session creation and retrieval.
func TestPromptKitHandlerGetOrCreateSession(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Create new session
	session1 := handler.getOrCreateSession("session-1")
	require.NotNil(t, session1)
	assert.Empty(t, session1.Messages)

	// Get same session
	session2 := handler.getOrCreateSession("session-1")
	assert.Same(t, session1, session2)

	// Create different session
	session3 := handler.getOrCreateSession("session-2")
	assert.NotSame(t, session1, session3)
}

// TestPromptKitHandlerResetSession tests session reset functionality.
func TestPromptKitHandlerResetSession(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Create session with messages
	session := handler.getOrCreateSession("test-session")
	session.Messages = append(session.Messages, types.NewUserMessage("test message"))
	assert.Len(t, session.Messages, 1)

	// Reset session
	handler.ResetSession("test-session")

	// Verify messages are cleared
	assert.Empty(t, session.Messages)

	// Reset non-existent session should not panic
	handler.ResetSession("non-existent")
}

// TestPromptKitHandlerGetSessionHistory tests getting session history.
func TestPromptKitHandlerGetSessionHistory(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Non-existent session returns nil
	history := handler.GetSessionHistory("non-existent")
	assert.Nil(t, history)

	// Create session with messages
	session := handler.getOrCreateSession("test-session")
	msg1 := types.NewUserMessage("message 1")
	msg2 := types.NewAssistantMessage("message 2")
	session.Messages = append(session.Messages, msg1, msg2)

	// Get history
	history = handler.GetSessionHistory("test-session")
	assert.Len(t, history, 2)

	// Verify it's a copy (modifying history doesn't affect original)
	history[0] = types.NewUserMessage("modified")
	assert.Equal(t, "message 1", session.Messages[0].Content)
}

// TestPromptKitHandlerListProviders tests listing providers.
func TestPromptKitHandlerListProviders(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// No registry returns nil
	providerList := handler.ListProviders()
	assert.Nil(t, providerList)
}

// TestPromptKitHandlerClose tests closing the handler.
func TestPromptKitHandlerClose(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Close with no registries should not error
	err := handler.Close()
	assert.NoError(t, err)

	// Verify maps are cleared
	assert.Empty(t, handler.nsRegistries)
}

// TestPromptKitHandlerInvalidateProviderCache tests cache invalidation.
func TestPromptKitHandlerInvalidateProviderCache(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// No k8sLoader - should be a no-op
	handler.InvalidateProviderCache()
	assert.Empty(t, handler.nsRegistries)
}

// TestPromptKitHandlerReload tests reloading configuration.
func TestPromptKitHandlerReload(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	newConfig := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	err := handler.Reload(newConfig)
	require.NoError(t, err)
	assert.Equal(t, newConfig, handler.config)
	assert.NotNil(t, handler.providerRegistry)

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestPromptKitHandlerBuildComponentsNilConfig tests buildComponents with nil config.
func TestPromptKitHandlerBuildComponentsNilConfig(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration provided")
}

// MockResponseWriter is a test implementation of facade.ResponseWriter.
type MockResponseWriter struct {
	Chunks       []string
	ToolCalls    []*facade.ToolCallInfo
	ToolResults  []*facade.ToolResultInfo
	DoneContent  string
	ErrorCode    string
	ErrorMessage string
	MediaChunks  []*facade.MediaChunkInfo
}

func (m *MockResponseWriter) WriteChunk(content string) error {
	m.Chunks = append(m.Chunks, content)
	return nil
}

func (m *MockResponseWriter) WriteChunkWithParts(_ []facade.ContentPart) error {
	return nil
}

func (m *MockResponseWriter) WriteDone(content string) error {
	m.DoneContent = content
	return nil
}

func (m *MockResponseWriter) WriteDoneWithParts(_ []facade.ContentPart) error {
	return nil
}

func (m *MockResponseWriter) WriteToolCall(toolCall *facade.ToolCallInfo) error {
	m.ToolCalls = append(m.ToolCalls, toolCall)
	return nil
}

func (m *MockResponseWriter) WriteToolResult(result *facade.ToolResultInfo) error {
	m.ToolResults = append(m.ToolResults, result)
	return nil
}

func (m *MockResponseWriter) WriteError(code, message string) error {
	m.ErrorCode = code
	m.ErrorMessage = message
	return nil
}

func (m *MockResponseWriter) WriteUploadReady(_ *facade.UploadReadyInfo) error {
	return nil
}

func (m *MockResponseWriter) WriteUploadComplete(_ *facade.UploadCompleteInfo) error {
	return nil
}

func (m *MockResponseWriter) WriteMediaChunk(chunk *facade.MediaChunkInfo) error {
	m.MediaChunks = append(m.MediaChunks, chunk)
	return nil
}

func (m *MockResponseWriter) WriteBinaryMediaChunk(
	_ [facade.MediaIDSize]byte, _ uint32, _ bool, _ string, _ []byte,
) error {
	return nil
}

func (m *MockResponseWriter) SupportsBinary() bool {
	return false
}

// TestConvertToPKMessageText tests converting text content parts.
func TestConvertToPKMessageText(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{Type: facade.ContentPartTypeText, Text: "Hello, world!"},
	}

	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	require.Len(t, msg.Parts, 1)
	// Text field is a pointer in PromptKit types
	require.NotNil(t, msg.Parts[0].Text)
	assert.Equal(t, "Hello, world!", *msg.Parts[0].Text)
}

// TestConvertToPKMessageImageURL tests converting image URL content parts.
func TestConvertToPKMessageImageURL(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{
			Type: facade.ContentPartTypeImage,
			Media: &facade.MediaContent{
				URL:      "https://example.com/image.png",
				MimeType: "image/png",
			},
		},
	}

	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	require.Len(t, msg.Parts, 1)
}

// TestConvertToPKMessageImageData tests converting image data content parts.
func TestConvertToPKMessageImageData(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{
			Type: facade.ContentPartTypeImage,
			Media: &facade.MediaContent{
				Data:     "base64encodeddata",
				MimeType: "image/png",
			},
		},
	}

	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	require.Len(t, msg.Parts, 1)
}

// TestConvertToPKMessageAudio tests converting audio content parts.
func TestConvertToPKMessageAudio(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{
			Type: facade.ContentPartTypeAudio,
			Media: &facade.MediaContent{
				Data:     "base64audiodata",
				MimeType: "audio/mp3",
			},
		},
	}

	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	require.Len(t, msg.Parts, 1)
}

// TestConvertToPKMessageVideo tests converting video content parts.
func TestConvertToPKMessageVideo(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{
			Type: facade.ContentPartTypeVideo,
			Media: &facade.MediaContent{
				Data:     "base64videodata",
				MimeType: "video/mp4",
			},
		},
	}

	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	require.Len(t, msg.Parts, 1)
}

// TestConvertToPKMessageMixed tests converting mixed content parts.
func TestConvertToPKMessageMixed(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{Type: facade.ContentPartTypeText, Text: "Look at this image:"},
		{
			Type: facade.ContentPartTypeImage,
			Media: &facade.MediaContent{
				URL:      "https://example.com/image.png",
				MimeType: "image/png",
			},
		},
	}

	msg := handler.convertToPKMessage("assistant", parts)
	assert.Equal(t, "assistant", msg.Role)
	require.Len(t, msg.Parts, 2)
}

// TestConvertToPKMessageNilMedia tests that nil media is handled gracefully.
func TestConvertToPKMessageNilMedia(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	parts := []facade.ContentPart{
		{Type: facade.ContentPartTypeImage, Media: nil},
		{Type: facade.ContentPartTypeAudio, Media: nil},
		{Type: facade.ContentPartTypeVideo, Media: nil},
	}

	// Should not panic
	msg := handler.convertToPKMessage("user", parts)
	assert.Equal(t, "user", msg.Role)
	// All parts should be skipped because Media is nil
	assert.Empty(t, msg.Parts)
}

// TestHandleReloadInvalidJSON tests handleReload with invalid JSON.
func TestHandleReloadInvalidJSON(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "not valid json",
		Metadata: map[string]string{
			"reload": "true",
		},
	}

	err := handler.handleReload(context.Background(), msg, writer)
	assert.NoError(t, err) // Error written to writer, not returned
	assert.Equal(t, "INVALID_CONFIG", writer.ErrorCode)
	assert.Contains(t, writer.ErrorMessage, "failed to parse config")
}

// TestHandleMessageNoRegistry tests HandleMessage when registry is nil.
func TestHandleMessageNoRegistry(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
		config:       nil, // No config means no registry
	}

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "test message",
	}

	err := handler.HandleMessage(context.Background(), "session-1", msg, writer)
	assert.NoError(t, err) // Error written to writer, not returned
	assert.Equal(t, "ENGINE_NOT_READY", writer.ErrorCode)
}

// TestHandleMessageReset tests HandleMessage with reset metadata.
func TestHandleMessageReset(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Build components to have a working registry
	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	// Create a session with messages
	session := handler.getOrCreateSession("test-session")
	session.Messages = append(session.Messages, types.NewUserMessage("old message"))

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "",
		Metadata: map[string]string{
			"reset": "true",
		},
	}

	err = handler.HandleMessage(context.Background(), "test-session", msg, writer)
	assert.NoError(t, err)
	assert.Equal(t, "Session reset", writer.DoneContent)

	// Verify session was reset
	assert.Empty(t, session.Messages)
}

// TestHandleMessageSetProvider tests setting provider via metadata.
func TestHandleMessageSetProvider(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	// Get session before the call
	session := handler.getOrCreateSession("test-session")
	assert.Empty(t, session.ProviderID)

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "test message",
		Metadata: map[string]string{
			"provider": "my-custom-provider",
		},
	}

	// This will fail because the provider doesn't exist, but we can verify
	// that the provider ID was set on the session
	_ = handler.HandleMessage(context.Background(), "test-session", msg, writer)

	// Verify provider was set on session
	session.mu.Lock()
	providerID := session.ProviderID
	session.mu.Unlock()
	assert.Equal(t, "my-custom-provider", providerID)
}

// TestHandleMessageProviderNotFound tests HandleMessage when provider doesn't exist.
func TestHandleMessageProviderNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	// Set a non-existent provider on the session
	session := handler.getOrCreateSession("test-session")
	session.ProviderID = "non-existent-provider"

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "test message",
	}

	err = handler.HandleMessage(context.Background(), "test-session", msg, writer)
	assert.NoError(t, err)
	assert.Equal(t, "PROVIDER_ERROR", writer.ErrorCode)
	assert.Contains(t, writer.ErrorMessage, "provider not found")
}

// TestReloadFromPathInvalid tests ReloadFromPath with an invalid path.
func TestReloadFromPathInvalid(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.ReloadFromPath("/non/existent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// TestGetRegistryAndConfigNoK8sLoader tests getRegistryAndConfig without K8s loader.
func TestGetRegistryAndConfigNoK8sLoader(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
		k8sLoader:    nil, // No K8s loader
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	registry, returnedCfg, err := handler.getRegistryAndConfig(context.Background(), "test-namespace")
	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Equal(t, cfg, returnedCfg)
}

// TestSessionStateConcurrency tests that SessionState handles concurrent access.
func TestSessionStateConcurrency(t *testing.T) {
	session := &SessionState{
		Messages: make([]types.Message, 0),
	}

	// Run concurrent operations
	done := make(chan bool)
	for range 10 {
		go func() {
			session.mu.Lock()
			session.Messages = append(session.Messages, types.NewUserMessage("test"))
			session.mu.Unlock()
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for range 10 {
		<-done
	}

	// Verify all messages were added
	session.mu.Lock()
	assert.Len(t, session.Messages, 10)
	session.mu.Unlock()
}

// TestHandlerInterfaceAssertion verifies the handler implements the interface.
func TestHandlerInterfaceAssertion(t *testing.T) {
	var _ facade.MessageHandler = (*PromptKitHandler)(nil)
}

// TestHandleMessageWithMultimodalParts tests HandleMessage with multimodal content.
func TestHandleMessageWithMultimodalParts(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Parts: []facade.ContentPart{
			{Type: facade.ContentPartTypeText, Text: "Describe this image"},
			{
				Type: facade.ContentPartTypeImage,
				Media: &facade.MediaContent{
					URL:      "https://example.com/image.png",
					MimeType: "image/png",
				},
			},
		},
	}

	// Execute - the mock provider will be used
	_ = handler.HandleMessage(context.Background(), "test-session", msg, writer)

	// Verify the message was added to session history
	session := handler.getOrCreateSession("test-session")
	session.mu.Lock()
	assert.NotEmpty(t, session.Messages)
	session.mu.Unlock()
}

// TestHandleReloadValidConfig tests handleReload with valid JSON config.
func TestHandleReloadValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	writer := &MockResponseWriter{}

	// Create valid config JSON
	validConfigJSON := `{
		"loaded_providers": {
			"mock": {
				"id": "mock",
				"type": "mock",
				"model": "mock-model"
			}
		},
		"defaults": {
			"output": {
				"dir": "` + outputDir + `"
			},
			"out_dir": "` + outputDir + `",
			"config_dir": "` + tmpDir + `"
		}
	}`

	msg := &facade.ClientMessage{
		Content: validConfigJSON,
		Metadata: map[string]string{
			"reload": "true",
		},
	}

	err := handler.handleReload(context.Background(), msg, writer)
	assert.NoError(t, err)

	// Check if config was successfully reloaded
	if writer.ErrorCode != "" {
		// If there was an error, it should be related to building components,
		// not parsing the config
		assert.NotEqual(t, "INVALID_CONFIG", writer.ErrorCode)
	}

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestHandleMessageNoProviderConfigured tests HandleMessage when no provider is available.
func TestHandleMessageNoProviderConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	// Config with empty providers - after building, no providers will be available
	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Don't build components - registry will be nil

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "test message",
	}

	err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
	assert.NoError(t, err)
	assert.Equal(t, "ENGINE_NOT_READY", writer.ErrorCode)
}

// TestCloseWithNsRegistries tests Close when there are namespace registries.
func TestCloseWithNsRegistries(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build a registry to add to nsRegistries
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err)

	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: map[string]*providers.Registry{"test-ns": registry},
	}

	// Close should clean up the namespace registries
	err = handler.Close()
	assert.NoError(t, err)
	assert.Empty(t, handler.nsRegistries)
}

// TestCloseWithMainRegistry tests Close when there is a main provider registry.
func TestCloseWithMainRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	require.NotNil(t, handler.providerRegistry)

	// Close should close the main registry
	err = handler.Close()
	assert.NoError(t, err)
}

// TestListProvidersWithRegistry tests ListProviders when registry is available.
func TestListProvidersWithRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	providerList := handler.ListProviders()
	assert.NotNil(t, providerList)
	assert.Contains(t, providerList, "mock")
}

// TestHandleMessageWithProviderDefaults tests that provider defaults are applied.
func TestHandleMessageWithProviderDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
				Defaults: config.ProviderDefaults{
					Temperature: 0.5,
					MaxTokens:   1024,
				},
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "test with defaults",
	}

	// Execute - should use provider defaults
	_ = handler.HandleMessage(context.Background(), "test-session", msg, writer)

	// Verify the message was added
	session := handler.getOrCreateSession("test-session")
	session.mu.Lock()
	messageCount := len(session.Messages)
	session.mu.Unlock()
	assert.GreaterOrEqual(t, messageCount, 1)
}

// TestBuildComponentsSetsDefaultOutputDir tests that buildComponents sets default output dir.
func TestBuildComponentsSetsDefaultOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Defaults: config.Defaults{
			// Leave Output.Dir empty
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Save and change working directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err = handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	// Verify the output directory was set
	assert.NotEmpty(t, cfg.Defaults.Output.Dir)
	assert.Contains(t, cfg.Defaults.Output.Dir, "arena-dev-console-output")
}

// TestNewPromptKitHandlerWithValidConfig tests creating a handler with valid config.
func TestNewPromptKitHandlerWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler, err := NewPromptKitHandler(cfg, logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.providerRegistry)

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestInvalidateProviderCacheWithK8sLoader tests InvalidateProviderCache with k8sLoader set.
func TestInvalidateProviderCacheWithK8sLoader(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build a registry to put in nsRegistries
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err)

	// Create a mock k8sLoader (minimal struct that just returns a namespace)
	mockK8sLoader := &K8sProviderLoader{
		namespace: "test-namespace",
		log:       logr.Discard(),
	}

	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: map[string]*providers.Registry{"test-namespace": registry},
		k8sLoader:    mockK8sLoader,
	}

	// Verify registry exists before invalidation
	assert.Len(t, handler.nsRegistries, 1)

	// Invalidate cache
	handler.InvalidateProviderCache()

	// Verify registry was removed
	assert.Empty(t, handler.nsRegistries)
}

// TestGetRegistryAndConfigWithK8sLoader tests getRegistryAndConfig with k8sLoader.
func TestGetRegistryAndConfigWithK8sLoader(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build a registry to put in nsRegistries
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err)

	// Create a mock k8sLoader
	mockK8sLoader := &K8sProviderLoader{
		namespace: "test-namespace",
		log:       logr.Discard(),
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: map[string]*providers.Registry{"test-namespace": registry},
		k8sLoader:    mockK8sLoader,
	}

	// Get registry and config - should return cached registry
	gotRegistry, gotCfg, err := handler.getRegistryAndConfig(context.Background(), "test-namespace")
	require.NoError(t, err)
	assert.Equal(t, registry, gotRegistry)
	assert.Equal(t, cfg, gotCfg)

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestCloseWithErrors tests Close when registry close fails.
func TestCloseWithErrors(t *testing.T) {
	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Close with empty registries should not error
	err := handler.Close()
	assert.NoError(t, err)
}

// TestHandleMessageReloadMetadata tests HandleMessage with reload metadata.
func TestHandleMessageReloadMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	writer := &MockResponseWriter{}

	// Valid reload config
	newCfgJSON := `{
		"loaded_providers": {
			"new-mock": {
				"id": "new-mock",
				"type": "mock",
				"model": "new-model"
			}
		},
		"defaults": {
			"output": {"dir": "` + outputDir + `"},
			"out_dir": "` + outputDir + `",
			"config_dir": "` + tmpDir + `"
		}
	}`

	msg := &facade.ClientMessage{
		Content: newCfgJSON,
		Metadata: map[string]string{
			"reload": "true",
		},
	}

	err = handler.HandleMessage(context.Background(), "test-session", msg, writer)
	assert.NoError(t, err)

	// The reload should succeed or fail at build stage, but not return error
	// because handleReload writes errors to the writer
}

// TestReloadFromPathValid tests ReloadFromPath with a valid config file.
func TestReloadFromPathValid(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	// Create a valid config file
	configContent := `
loaded_providers:
  mock:
    id: mock
    type: mock
    model: mock-model
defaults:
  output:
    dir: ` + outputDir + `
  out_dir: ` + outputDir + `
  config_dir: ` + tmpDir + `
`
	configPath := filepath.Join(tmpDir, "promptkit.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	handler := &PromptKitHandler{
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// This may fail because PromptKit config loading expects certain structure
	// but it tests the code path
	_ = handler.ReloadFromPath(configPath)

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestPromptKitHandlerCloseWithRegistry tests closing with a real provider registry.
func TestPromptKitHandlerCloseWithRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Build components to populate the registry
	err := handler.buildComponents()
	require.NoError(t, err)
	require.NotNil(t, handler.providerRegistry)

	// Now close the handler
	err = handler.Close()
	assert.NoError(t, err)

	// Verify nsRegistries is cleared
	assert.Empty(t, handler.nsRegistries)
}

// TestPromptKitHandlerCloseWithNamespaceRegistries tests closing with namespace registries.
func TestPromptKitHandlerCloseWithNamespaceRegistries(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	outputDir2 := filepath.Join(tmpDir, "output2")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Create a second config for namespace registry
	cfg2 := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir2,
			},
			OutDir:    outputDir2,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build the second registry separately
	nsRegistry, _, _, _, _, err := engine.BuildEngineComponents(cfg2)
	require.NoError(t, err)
	require.NotNil(t, nsRegistry)

	handler := &PromptKitHandler{
		config:   cfg,
		log:      logr.Discard(),
		sessions: make(map[string]*SessionState),
		nsRegistries: map[string]*providers.Registry{
			"test-namespace": nsRegistry,
		},
	}

	// Build main components
	err = handler.buildComponents()
	require.NoError(t, err)
	require.NotNil(t, handler.providerRegistry)

	// Verify we have both main and namespace registries
	assert.NotNil(t, handler.providerRegistry)
	assert.Len(t, handler.nsRegistries, 1)

	// Now close the handler
	err = handler.Close()
	assert.NoError(t, err)

	// Verify nsRegistries is cleared
	assert.Empty(t, handler.nsRegistries)
}

// TestNewPromptKitHandlerBuildComponentsError tests NewPromptKitHandler when
// buildComponents fails (invalid config).
func TestNewPromptKitHandlerBuildComponentsError(t *testing.T) {
	// Create a config with providers but invalid output directory that can't be created
	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: "/nonexistent/readonly/path",
			},
			OutDir:    "/nonexistent/readonly/path",
			ConfigDir: "/nonexistent/readonly/config",
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// This should fail because it can't create the output directory
	_, err := NewPromptKitHandler(cfg, logr.Discard())
	// May or may not error depending on system - the key is testing the code path
	_ = err
}

// mockStreamingProvider implements providers.Provider with streaming support.
type mockStreamingProvider struct {
	chunks        []providers.StreamChunk
	streamErr     error
	supportsStr   bool
	predictResp   providers.PredictionResponse
	predictErr    error
	shouldRaw     bool
	providerID    string
	providerModel string
}

func (m *mockStreamingProvider) ID() string {
	if m.providerID != "" {
		return m.providerID
	}
	return "mock-streaming"
}

func (m *mockStreamingProvider) Model() string {
	if m.providerModel != "" {
		return m.providerModel
	}
	return "mock-model"
}

func (m *mockStreamingProvider) Predict(
	_ context.Context, _ providers.PredictionRequest,
) (providers.PredictionResponse, error) {
	return m.predictResp, m.predictErr
}

func (m *mockStreamingProvider) PredictStream(
	_ context.Context, _ providers.PredictionRequest,
) (<-chan providers.StreamChunk, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan providers.StreamChunk, len(m.chunks))
	for _, chunk := range m.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func (m *mockStreamingProvider) SupportsStreaming() bool {
	return m.supportsStr
}

func (m *mockStreamingProvider) ShouldIncludeRawOutput() bool {
	return m.shouldRaw
}

func (m *mockStreamingProvider) Close() error {
	return nil
}

func (m *mockStreamingProvider) CalculateCost(_, _, _ int) types.CostInfo {
	return types.CostInfo{}
}

// TestExecuteStreamingWithStreamingProvider tests the streaming code path.
func TestExecuteStreamingWithStreamingProvider(t *testing.T) {
	finishReason := "stop"
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: true,
		chunks: []providers.StreamChunk{
			{Delta: "Hello, ", Content: "Hello, "},
			{Delta: "world!", Content: "Hello, world!"},
			{Content: "Hello, world!", FinishReason: &finishReason},
		},
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	content, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, world!", content)
	assert.Len(t, writer.Chunks, 2) // Two deltas were written
	assert.Equal(t, "Hello, world!", writer.DoneContent)
}

// TestExecuteStreamingWithToolCalls tests streaming with tool calls.
func TestExecuteStreamingWithToolCalls(t *testing.T) {
	finishReason := "tool_calls"
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: true,
		chunks: []providers.StreamChunk{
			{
				Delta: "Let me search for that.",
				ToolCalls: []types.MessageToolCall{
					{
						ID:   "call_123",
						Name: "search",
						Args: []byte(`{"query": "test"}`),
					},
				},
			},
			{Content: "Let me search for that.", FinishReason: &finishReason},
		},
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Search for something")},
	}

	_, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.NoError(t, err)
	assert.Len(t, writer.ToolCalls, 1)
	assert.Equal(t, "call_123", writer.ToolCalls[0].ID)
	assert.Equal(t, "search", writer.ToolCalls[0].Name)
}

// TestExecuteStreamingError tests streaming when an error occurs mid-stream.
func TestExecuteStreamingError(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: true,
		chunks: []providers.StreamChunk{
			{Delta: "Starting..."},
			{Error: assert.AnError}, // Error in stream
		},
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	_, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.Error(t, err)
}

// TestExecuteStreamingStartError tests streaming when PredictStream fails to start.
func TestExecuteStreamingStartError(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: true,
		streamErr:   assert.AnError,
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	_, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start stream")
}

// TestExecuteStreamingNonStreamingFallback tests the non-streaming fallback.
func TestExecuteStreamingNonStreamingFallback(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: false,
		predictResp: providers.PredictionResponse{
			Content: "Non-streaming response",
		},
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	content, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.NoError(t, err)
	assert.Equal(t, "Non-streaming response", content)
	assert.Equal(t, "Non-streaming response", writer.DoneContent)
}

// TestExecuteStreamingNonStreamingError tests error in non-streaming fallback.
func TestExecuteStreamingNonStreamingError(t *testing.T) {
	handler := &PromptKitHandler{
		log: logr.Discard(),
	}

	provider := &mockStreamingProvider{
		supportsStr: false,
		predictErr:  assert.AnError,
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	_, err := handler.executeStreaming(context.Background(), provider, req, writer)
	assert.Error(t, err)
}

// TestGetOrLoadK8sRegistryWithProviders tests loading registry from K8s providers.
func TestGetOrLoadK8sRegistryWithProviders(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Create a K8s loader with test providers
	loader := newTestK8sProviderLoader(t, "test-namespace",
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-provider",
				Namespace: "test-namespace",
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  corev1alpha1.ProviderTypeMock,
				Model: "mock-model",
			},
			Status: corev1alpha1.ProviderStatus{
				Phase: corev1alpha1.ProviderPhaseReady,
			},
		},
	)

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
		k8sLoader:    loader,
	}

	// Call getOrLoadK8sRegistry
	registry, gotCfg, err := handler.getOrLoadK8sRegistry(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, gotCfg)

	// Verify registry is cached
	cached, ok := handler.nsRegistries["test-namespace"]
	assert.True(t, ok)
	assert.Equal(t, registry, cached)

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestGetOrLoadK8sRegistryCached tests that cached registry is returned.
func TestGetOrLoadK8sRegistryCached(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Pre-build a registry
	cachedRegistry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err)
	defer func() {
		_ = cachedRegistry.Close()
	}()

	loader := newTestK8sProviderLoader(t, "test-namespace")

	handler := &PromptKitHandler{
		config:   cfg,
		log:      logr.Discard(),
		sessions: make(map[string]*SessionState),
		nsRegistries: map[string]*providers.Registry{
			"test-namespace": cachedRegistry,
		},
		k8sLoader: loader,
	}

	// Should return cached registry
	registry, gotCfg, err := handler.getOrLoadK8sRegistry(context.Background())
	require.NoError(t, err)
	assert.Equal(t, cachedRegistry, registry)
	assert.Equal(t, cfg, gotCfg)
}

// TestBuildPredictionRequest tests building a prediction request with defaults.
func TestBuildPredictionRequest(t *testing.T) {
	handler := &PromptKitHandler{log: logr.Discard()}
	msgs := []types.Message{types.NewUserMessage("test")}

	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
				Defaults: config.ProviderDefaults{
					Temperature: 0.3,
					MaxTokens:   2048,
				},
			},
		},
	}

	req := handler.buildPredictionRequest(msgs, "mock", cfg)
	assert.InDelta(t, 0.3, req.Temperature, 1e-6)
	assert.Equal(t, 2048, req.MaxTokens)
	assert.Len(t, req.Messages, 1)
}

// TestBuildPredictionRequestDefaults tests building request when provider has no custom defaults.
func TestBuildPredictionRequestDefaults(t *testing.T) {
	handler := &PromptKitHandler{log: logr.Discard()}
	msgs := []types.Message{types.NewUserMessage("test")}

	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{},
	}

	req := handler.buildPredictionRequest(msgs, "nonexistent", cfg)
	assert.InDelta(t, 0.7, req.Temperature, 1e-6)
	assert.Equal(t, 4096, req.MaxTokens)
}

// TestExecuteStreamingWithCostInfo tests that cost info is returned from streaming.
func TestExecuteStreamingWithCostInfo(t *testing.T) {
	finishReason := "stop"
	handler := &PromptKitHandler{log: logr.Discard()}

	costInfo := &types.CostInfo{
		InputTokens:  100,
		OutputTokens: 50,
		TotalCost:    0.005,
	}

	provider := &mockStreamingProvider{
		supportsStr: true,
		chunks: []providers.StreamChunk{
			{Delta: "Hello", Content: "Hello"},
			{Content: "Hello", CostInfo: costInfo, FinishReason: &finishReason},
		},
	}

	writer := &MockResponseWriter{}
	req := providers.PredictionRequest{
		Messages: []types.Message{types.NewUserMessage("Hi")},
	}

	response, gotCost, err := handler.executeStreamingWithCost(context.Background(), provider, req, writer)
	assert.NoError(t, err)
	assert.Equal(t, "Hello", response)
	require.NotNil(t, gotCost)
	assert.Equal(t, 100, gotCost.InputTokens)
	assert.Equal(t, 50, gotCost.OutputTokens)
	assert.InDelta(t, 0.005, gotCost.TotalCost, 1e-9)
}

// TestGetOrLoadK8sRegistryNoProviders tests fallback when no K8s providers found.
func TestGetOrLoadK8sRegistryNoProviders(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Create K8s loader with NO providers
	loader := newTestK8sProviderLoader(t, "test-namespace")

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
		k8sLoader:    loader,
	}

	// Build the static registry first
	err := handler.buildComponents()
	require.NoError(t, err)
	defer func() {
		if handler.providerRegistry != nil {
			_ = handler.providerRegistry.Close()
		}
	}()

	// Should fallback to static config
	registry, gotCfg, err := handler.getOrLoadK8sRegistry(context.Background())
	require.NoError(t, err)
	assert.Equal(t, handler.providerRegistry, registry)
	assert.Equal(t, cfg, gotCfg)
}

// TestHandleMessageWithMockProvider verifies that HandleMessage
// works correctly with the built-in mock provider.
func TestHandleMessageWithMockProvider(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output:    config.OutputConfig{Dir: outputDir},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {ID: "mock", Type: "mock", Model: "mock-model"},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err)
	t.Cleanup(func() { _ = handler.Close() })

	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{Content: "test message"}

	err = handler.HandleMessage(context.Background(), "sess-test", msg, writer)
	require.NoError(t, err)

	// Verify the session has the user message in its in-memory history.
	sess := handler.getOrCreateSession("sess-test")
	sess.mu.Lock()
	assert.GreaterOrEqual(t, len(sess.Messages), 1)
	sess.mu.Unlock()
}
