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
	"testing"

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
	// Duration should be reported (>= 0; mock is fast but non-zero).
	assert.GreaterOrEqual(t, resp.GetDurationMs(), int32(0))
}

func TestServer_Invoke_EphemeralConversation(t *testing.T) {
	s := newInvokeTestServer(t)

	// Sanity check: no conversations tracked before Invoke.
	assert.Empty(t, s.conversations, "server should start with no tracked conversations")

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

	// After three Invoke calls, nothing should be tracked in the
	// conversation map.
	assert.Empty(t, s.conversations,
		"three Invoke calls should not accumulate tracked conversations")
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
}
