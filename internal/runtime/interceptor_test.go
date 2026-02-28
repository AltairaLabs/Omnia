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

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/altairalabs/omnia/pkg/policy"
)

func TestExtractPolicyFromMetadata(t *testing.T) {
	md := metadata.New(map[string]string{
		policy.HeaderAgentName:     "test-agent",
		policy.HeaderNamespace:     "production",
		policy.HeaderSessionID:     "sess-123",
		policy.HeaderRequestID:     "req-456",
		policy.HeaderUserID:        "user@example.com",
		policy.HeaderUserRoles:     "admin,viewer",
		policy.HeaderUserEmail:     "user@example.com",
		policy.HeaderAuthorization: "Bearer token",
		policy.HeaderProvider:      "claude",
		policy.HeaderModel:         "claude-sonnet-4-20250514",
		"x-omnia-claim-team":       "engineering",
		"x-omnia-claim-region":     "us-east",
	})

	ctx := metadata.NewIncomingContext(context.Background(), md)
	enriched := extractPolicyFromMetadata(ctx)

	fields := policy.ExtractPropagationFields(enriched)
	assert.Equal(t, "test-agent", fields.AgentName)
	assert.Equal(t, "production", fields.Namespace)
	assert.Equal(t, "sess-123", fields.SessionID)
	assert.Equal(t, "req-456", fields.RequestID)
	assert.Equal(t, "user@example.com", fields.UserID)
	assert.Equal(t, "admin,viewer", fields.UserRoles)
	assert.Equal(t, "user@example.com", fields.UserEmail)
	assert.Equal(t, "Bearer token", fields.Authorization)
	assert.Equal(t, "claude", fields.Provider)
	assert.Equal(t, "claude-sonnet-4-20250514", fields.Model)
	assert.Equal(t, "engineering", fields.Claims["team"])
	assert.Equal(t, "us-east", fields.Claims["region"])
}

func TestExtractPolicyFromMetadata_NoMetadata(t *testing.T) {
	ctx := context.Background()
	enriched := extractPolicyFromMetadata(ctx)
	fields := policy.ExtractPropagationFields(enriched)
	assert.Empty(t, fields.AgentName)
	assert.Nil(t, fields.Claims)
}

func TestExtractPolicyFromMetadata_PartialMetadata(t *testing.T) {
	md := metadata.New(map[string]string{
		policy.HeaderAgentName: "partial-agent",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	enriched := extractPolicyFromMetadata(ctx)
	fields := policy.ExtractPropagationFields(enriched)
	assert.Equal(t, "partial-agent", fields.AgentName)
	assert.Empty(t, fields.Namespace)
	assert.Nil(t, fields.Claims)
}

func TestExtractClaims(t *testing.T) {
	md := metadata.New(map[string]string{
		"x-omnia-claim-team":   "eng",
		"x-omnia-claim-region": "eu",
		"x-unrelated-header":   "ignored",
		policy.HeaderAgentName: "also-ignored",
	})

	claims := extractClaims(md)
	assert.Equal(t, "eng", claims["team"])
	assert.Equal(t, "eu", claims["region"])
	assert.Len(t, claims, 2)
}

func TestExtractClaims_NoClaims(t *testing.T) {
	md := metadata.New(map[string]string{
		policy.HeaderAgentName: "agent",
	})
	claims := extractClaims(md)
	assert.Nil(t, claims)
}

func TestFirstValue(t *testing.T) {
	md := metadata.New(map[string]string{
		"key": "value",
	})
	assert.Equal(t, "value", firstValue(md, "key"))
	assert.Equal(t, "", firstValue(md, "missing"))
}

func TestPolicyUnaryServerInterceptor(t *testing.T) {
	interceptor := PolicyUnaryServerInterceptor()

	md := metadata.New(map[string]string{
		policy.HeaderAgentName: "intercepted-agent",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedCtx context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		capturedCtx = ctx
		return "response", nil
	}

	resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
	assert.NoError(t, err)
	assert.Equal(t, "response", resp)
	assert.Equal(t, "intercepted-agent", policy.AgentName(capturedCtx))
}

func TestPolicyStreamServerInterceptor(t *testing.T) {
	interceptor := PolicyStreamServerInterceptor()

	md := metadata.New(map[string]string{
		policy.HeaderAgentName: "stream-agent",
		policy.HeaderUserID:    "stream-user",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	mockStream := &mockServerStream{ctx: ctx}

	var capturedCtx context.Context
	handler := func(srv any, stream grpc.ServerStream) error {
		capturedCtx = stream.Context()
		return nil
	}

	err := interceptor(nil, mockStream, &grpc.StreamServerInfo{}, handler)
	assert.NoError(t, err)
	assert.Equal(t, "stream-agent", policy.AgentName(capturedCtx))
	assert.Equal(t, "stream-user", policy.UserID(capturedCtx))
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}
