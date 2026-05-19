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
	"testing"

	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

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
