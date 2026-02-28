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

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/policy"
)

func TestSetPolicyHeaders(t *testing.T) {
	ctx := context.Background()
	ctx = policy.WithAgentName(ctx, "test-agent")
	ctx = policy.WithNamespace(ctx, "production")
	ctx = policy.WithUserID(ctx, "user-1")
	ctx = policy.WithProvider(ctx, "claude")
	ctx = policy.WithClaims(ctx, map[string]string{"team": "eng"})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://example.com", nil)
	require.NoError(t, err)

	SetPolicyHeaders(ctx, req)

	assert.Equal(t, "test-agent", req.Header.Get("X-Omnia-Agent-Name"))
	assert.Equal(t, "production", req.Header.Get("X-Omnia-Namespace"))
	assert.Equal(t, "user-1", req.Header.Get("X-Omnia-User-Id"))
	assert.Equal(t, "claude", req.Header.Get("X-Omnia-Provider"))
	assert.Equal(t, "eng", req.Header.Get("X-Omnia-Claim-Team"))
}

func TestSetToolHeaders(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", nil)
	require.NoError(t, err)

	SetToolHeaders(req, "my-tool", "my-registry")

	assert.Equal(t, "my-tool", req.Header.Get("X-Omnia-Tool-Name"))
	assert.Equal(t, "my-registry", req.Header.Get("X-Omnia-Tool-Registry"))
}

func TestSetToolHeaders_Empty(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", nil)
	require.NoError(t, err)

	SetToolHeaders(req, "", "")

	assert.Empty(t, req.Header.Get("X-Omnia-Tool-Name"))
	assert.Empty(t, req.Header.Get("X-Omnia-Tool-Registry"))
}

func TestSetParamHeaders(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", nil)
	require.NoError(t, err)

	args := map[string]any{
		"customer_id": "cust-1",
		"count":       float64(5),
		"nested":      map[string]any{"skip": true},
	}

	SetParamHeaders(req, args)

	assert.Equal(t, "cust-1", req.Header.Get("X-Omnia-Param-CustomerId"))
	assert.Equal(t, "5", req.Header.Get("X-Omnia-Param-Count"))
	assert.Empty(t, req.Header.Get("X-Omnia-Param-Nested"))
}

func TestSetAllOutboundHeaders(t *testing.T) {
	ctx := context.Background()
	ctx = policy.WithAgentName(ctx, "agent")
	ctx = policy.WithUserID(ctx, "user")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://example.com", nil)
	require.NoError(t, err)

	args := map[string]any{"query": "test"}

	SetAllOutboundHeaders(ctx, req, "search-tool", "registry-1", args)

	assert.Equal(t, "agent", req.Header.Get("X-Omnia-Agent-Name"))
	assert.Equal(t, "user", req.Header.Get("X-Omnia-User-Id"))
	assert.Equal(t, "search-tool", req.Header.Get("X-Omnia-Tool-Name"))
	assert.Equal(t, "registry-1", req.Header.Get("X-Omnia-Tool-Registry"))
	assert.Equal(t, "test", req.Header.Get("X-Omnia-Param-Query"))
}

func TestPolicyGRPCMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = policy.WithAgentName(ctx, "agent")
	ctx = policy.WithProvider(ctx, "openai")

	args := map[string]any{"limit": float64(10)}

	md := PolicyGRPCMetadata(ctx, "grpc-tool", "grpc-registry", args)

	assert.Equal(t, "agent", md[policy.HeaderAgentName])
	assert.Equal(t, "openai", md[policy.HeaderProvider])
	assert.Equal(t, "grpc-tool", md[policy.HeaderToolName])
	assert.Equal(t, "grpc-registry", md[policy.HeaderToolRegistry])
	assert.Equal(t, "10", md["x-omnia-param-Limit"])
}

func TestPolicyGRPCMetadata_EmptyContext(t *testing.T) {
	md := PolicyGRPCMetadata(context.Background(), "", "", nil)
	// Should only contain empty entries; tool/registry are empty so not set
	_, hasToolName := md[policy.HeaderToolName]
	assert.False(t, hasToolName)
}

func TestSetPolicyHeaders_EmptyContext(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)

	SetPolicyHeaders(context.Background(), req)

	// No headers should be set on an empty context
	assert.Empty(t, req.Header.Get("X-Omnia-Agent-Name"))
	assert.Empty(t, req.Header.Get("X-Omnia-User-Id"))
}
