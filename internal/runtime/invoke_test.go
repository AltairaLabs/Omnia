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
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// invokeTemplatePack mirrors config/samples/dev/deep-research-function.yaml: a
// prompt whose system_template references a required {{topic}} variable with no
// default. It is the pack against which the #1473 input→variable binding is
// asserted — before the fix {{topic}} stayed literal in the rendered prompt.
const invokeTemplatePack = `{
	"id": "tpl-pack",
	"name": "tpl-pack",
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
			"system_template": "You are a research planner investigating: {{topic}}.",
			"variables": [
				{"name": "topic", "type": "string", "required": true}
			]
		}
	}
}`

// Message role literals used when inspecting the captured PredictionRequest.
const (
	roleUser   = "user"
	roleSystem = "system"
)

// recordingProvider captures the PredictionRequest the runtime hands to the
// provider so tests can assert how the prompt was rendered. It embeds the mock
// Provider (supportsStreaming=true) and overrides PredictStream to record the
// request before emitting a trivial one-chunk completion.
type recordingProvider struct {
	*mock.Provider
	mu   sync.Mutex
	last providers.PredictionRequest
}

func newRecordingProvider() *recordingProvider {
	return &recordingProvider{Provider: mock.NewProvider("rec", "rec-model", false)}
}

// responseFormat returns the response format the runtime set on the captured
// PredictionRequest (nil if none). Used by the #1483 output-format tests.
func (r *recordingProvider) responseFormat() *providers.ResponseFormat {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last.ResponseFormat
}

func (r *recordingProvider) PredictStream(_ context.Context, req providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	r.mu.Lock()
	r.last = req
	r.mu.Unlock()

	stop := "stop"
	ch := make(chan providers.StreamChunk, 2)
	ch <- providers.StreamChunk{Content: "ok", Delta: "ok"}
	ch <- providers.StreamChunk{Content: "ok", FinishReason: &stop}
	close(ch)
	return ch, nil
}

// systemText returns the rendered system prompt the provider saw, joining the
// dedicated System field with any system-role messages (PromptKit may deliver
// system context either way depending on NormalizeMessages).
func (r *recordingProvider) systemText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	parts := []string{r.last.System}
	for i := range r.last.Messages {
		if r.last.Messages[i].Role == roleSystem {
			parts = append(parts, r.last.Messages[i].Content)
		}
	}
	return strings.Join(parts, "\n")
}

// userText returns the concatenated content of all user-role messages the
// provider saw, reading both the legacy Content field and text parts (the SDK
// stores AddTextPart content under Parts, leaving Content empty).
func (r *recordingProvider) userText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for i := range r.last.Messages {
		if r.last.Messages[i].Role != roleUser {
			continue
		}
		b.WriteString(r.last.Messages[i].Content)
		for _, p := range r.last.Messages[i].Parts {
			if p.Text != nil {
				b.WriteString(*p.Text)
			}
		}
	}
	return b.String()
}

// newInvokeServerWithProvider builds an Invoke-capable Server backed by the
// given pack body and provider. The provider is injected via WithSDKOptions
// (not WithMockProvider) so the runtime's mock path doesn't override it.
func newInvokeServerWithProvider(t *testing.T, packBody string, prov providers.Provider) *Server {
	t.Helper()
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, packBody))

	return NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithSDKOptions(sdk.WithProvider(prov)),
	)
}

func TestServer_Invoke_BindsInputToTemplateVariables(t *testing.T) {
	rec := newRecordingProvider()
	s := newInvokeServerWithProvider(t, invokeTemplatePack, rec)

	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "bind-1",
		InputJson:    `{"topic":"the 2023 battery storage surge"}`,
	})
	require.NoError(t, err)

	system := rec.systemText()
	assert.Contains(t, system, "the 2023 battery storage surge",
		"input field topic must bind to the {{topic}} template variable")
	assert.NotContains(t, system, "{{topic}}",
		"the template variable must be resolved, not left literal (#1473)")
}

func TestServer_Invoke_KeepsInputJSONAsUserTurn(t *testing.T) {
	rec := newRecordingProvider()
	s := newInvokeServerWithProvider(t, invokeTemplatePack, rec)

	input := `{"topic":"superconductors"}`
	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "bind-2",
		InputJson:    input,
	})
	require.NoError(t, err)

	// Binding does not drop the raw JSON: it is still delivered as the user
	// turn so models that read structured input directly keep working.
	assert.Contains(t, rec.userText(), input,
		"the raw input JSON should remain the user-turn message")
}

func TestServer_Invoke_AppliesJSONSchemaResponseFormat(t *testing.T) {
	rec := newRecordingProvider()
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTemplatePack))

	schema := []byte(`{"type":"object","properties":{"topic":{"type":"string"}}}`)
	s := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithAgentIdentity("deep-research", "default"),
		WithFunctionOutputFormat("function", "json_schema", schema),
		WithSDKOptions(sdk.WithProvider(rec)),
	)

	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "rf-1",
		InputJson:    `{"topic":"x"}`,
	})
	require.NoError(t, err)

	rf := rec.responseFormat()
	require.NotNil(t, rf, "function json_schema mode must set a provider response format")
	assert.Equal(t, providers.ResponseFormatJSONSchema, rf.Type)
	assert.Equal(t, "deep-research", rf.SchemaName)
}

func TestServer_Invoke_TextFormatSetsNoResponseFormat(t *testing.T) {
	rec := newRecordingProvider()
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTemplatePack))

	s := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithAgentIdentity("deep-research", "default"),
		WithFunctionOutputFormat("function", "text", nil),
		WithSDKOptions(sdk.WithProvider(rec)),
	)

	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "rf-2",
		InputJson:    `{"topic":"x"}`,
	})
	require.NoError(t, err)
	assert.Nil(t, rec.responseFormat(), "text mode must not set a response format")
}

// invokeTestPack is the minimal pack body shared by every Invoke test.
const invokeTestPack = `{
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

func newInvokeTestServer(t *testing.T) *Server {
	t.Helper()
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTestPack))

	return NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
	)
}

func TestServer_Invoke_RejectsMissingInvocationID(t *testing.T) {
	s := newInvokeTestServer(t)
	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InputJson: `{"q":"hi"}`,
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invocation_id")
}

func TestServer_Invoke_RejectsEmptyInput(t *testing.T) {
	s := newInvokeTestServer(t)
	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "test-1",
		InputJson:    "",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "input_json")
}

func TestServer_Invoke_HappyPath(t *testing.T) {
	s := newInvokeTestServer(t)
	resp, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "test-happy",
		InputJson:    `{"q":"hello"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "test-happy", resp.GetInvocationId())
	// Mock provider always returns a non-empty response, so output_json
	// should not be empty. The exact content depends on the mock pack
	// configuration.
	assert.NotEmpty(t, resp.GetOutputJson())
}

func TestServer_Invoke_EphemeralConversation(t *testing.T) {
	s := newInvokeTestServer(t)

	// Sanity check: no conversations tracked before Invoke.
	assert.Empty(t, s.conversations, "server should start with no tracked conversations")
	assert.Empty(t, s.unsubscribeFns, "server should start with no tracked unsubscribes")

	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "ephemeral-1",
		InputJson:    `{"q":"ping"}`,
	})
	require.NoError(t, err)

	// The conversation must NOT be tracked in s.conversations — Functions
	// are stateless and the conversation is closed immediately.
	assert.Empty(t, s.conversations,
		"Invoke should not leak conversations into the conversations map")
	assert.Empty(t, s.turnIndices,
		"Invoke should not leak turn indices")
	// createConversation subscribes ~11 event-bus listeners; Invoke must
	// unsubscribe them on completion or s.unsubscribeFns grows unboundedly.
	assert.Empty(t, s.unsubscribeFns,
		"Invoke must drain s.unsubscribeFns for the invocation")
}

func TestServer_Invoke_MultipleCallsStayIndependent(t *testing.T) {
	s := newInvokeTestServer(t)

	ids := []string{"invoke-iter-1", "invoke-iter-2", "invoke-iter-3"}
	for _, id := range ids {
		resp, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
			InvocationId: id,
			InputJson:    `{"q":"hello"}`,
		})
		require.NoError(t, err, "invocation %s failed", id)
		assert.Equal(t, id, resp.GetInvocationId())
	}

	// After three Invoke calls, nothing should be tracked in either the
	// conversation or unsubscribe maps.
	assert.Empty(t, s.conversations,
		"three Invoke calls should not accumulate tracked conversations")
	assert.Empty(t, s.unsubscribeFns,
		"three Invoke calls should not accumulate event-bus subscriptions")
}

func TestServer_Invoke_PackOpenFailure(t *testing.T) {
	// Server with a bogus pack path forces sdk.Open to fail.
	s := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath("/nonexistent/path/to/pack.promptpack"),
		WithPromptName("default"),
		WithMockProvider(true),
	)

	_, err := s.Invoke(context.Background(), &runtimev1.InvocationRequest{
		InvocationId: "bad-pack",
		InputJson:    `{"q":"hi"}`,
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "create conversation",
		"error should mention the failing stage so authors can debug")
}

// streamFromChunks is a tiny test helper that publishes a fixed sequence
// of StreamChunks on a closed channel, mirroring how the SDK exposes
// stream completion. Used by the consumeInvocationStream unit tests.
func streamFromChunks(chunks []sdk.StreamChunk) <-chan sdk.StreamChunk {
	ch := make(chan sdk.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch
}

func TestConsumeInvocationStream_HappyPath(t *testing.T) {
	stream := streamFromChunks([]sdk.StreamChunk{
		{Type: sdk.ChunkText, Text: "hello "},
		{Type: sdk.ChunkText, Text: "world"},
		{Type: sdk.ChunkDone, Message: &sdk.Response{}},
	})
	resp, content, err := consumeInvocationStream(stream, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "hello world", content)
}

func TestConsumeInvocationStream_RejectsClientToolChunk(t *testing.T) {
	stream := streamFromChunks([]sdk.StreamChunk{
		{Type: sdk.ChunkText, Text: "thinking..."},
		{
			Type:       sdk.ChunkClientTool,
			ClientTool: &sdk.PendingClientTool{ToolName: "read_clipboard"},
		},
		// A done chunk follows but we should bail before consuming it.
		{Type: sdk.ChunkDone, Message: &sdk.Response{}},
	})
	_, _, err := consumeInvocationStream(stream, nil)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	assert.Contains(t, st.Message(), "read_clipboard")
	assert.Contains(t, st.Message(), "agent mode")
}

func TestConsumeInvocationStream_ProviderErrorChunkSurfacesAsInternal(t *testing.T) {
	stream := streamFromChunks([]sdk.StreamChunk{
		{Type: sdk.ChunkText, Text: "partial"},
		{Error: errors.New("synthetic provider failure")},
	})
	_, _, err := consumeInvocationStream(stream, nil)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "synthetic provider failure")
}

func TestConsumeInvocationStream_MissingDoneChunkIsInternal(t *testing.T) {
	stream := streamFromChunks([]sdk.StreamChunk{
		{Type: sdk.ChunkText, Text: "no terminator"},
	})
	_, _, err := consumeInvocationStream(stream, nil)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "done chunk")
}

func TestConsumeInvocationStream_MediaChunksAreDropped(t *testing.T) {
	stream := streamFromChunks([]sdk.StreamChunk{
		{Type: sdk.ChunkText, Text: "before "},
		{Type: sdk.ChunkMedia},
		{Type: sdk.ChunkText, Text: "after"},
		{Type: sdk.ChunkDone, Message: &sdk.Response{}},
	})
	_, content, err := consumeInvocationStream(stream, nil)
	require.NoError(t, err)
	assert.Equal(t, "before after", content,
		"media chunks should not appear in the function output")
}
