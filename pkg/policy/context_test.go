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

package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithAndExtractPropagationFields(t *testing.T) {
	ctx := context.Background()
	fields := &PropagationFields{
		AgentName:     "my-agent",
		Namespace:     "production",
		SessionID:     "sess-123",
		RequestID:     "req-456",
		UserID:        "user@example.com",
		UserRoles:     "admin,viewer",
		UserEmail:     "user@example.com",
		Authorization: "Bearer token123",
		Provider:      "claude",
		Model:         "claude-sonnet-4-20250514",
		Claims:        map[string]string{"team": "engineering", "region": "us-east"},
	}

	ctx = WithPropagationFields(ctx, fields)
	extracted := ExtractPropagationFields(ctx)

	assert.Equal(t, fields.AgentName, extracted.AgentName)
	assert.Equal(t, fields.Namespace, extracted.Namespace)
	assert.Equal(t, fields.SessionID, extracted.SessionID)
	assert.Equal(t, fields.RequestID, extracted.RequestID)
	assert.Equal(t, fields.UserID, extracted.UserID)
	assert.Equal(t, fields.UserRoles, extracted.UserRoles)
	assert.Equal(t, fields.UserEmail, extracted.UserEmail)
	assert.Equal(t, fields.Authorization, extracted.Authorization)
	assert.Equal(t, fields.Provider, extracted.Provider)
	assert.Equal(t, fields.Model, extracted.Model)
	assert.Equal(t, fields.Claims, extracted.Claims)
}

func TestWithPropagationFields_NilFields(t *testing.T) {
	ctx := context.Background()
	result := WithPropagationFields(ctx, nil)
	assert.Equal(t, ctx, result)
}

func TestWithPropagationFields_EmptyFields(t *testing.T) {
	ctx := context.Background()
	result := WithPropagationFields(ctx, &PropagationFields{})
	extracted := ExtractPropagationFields(result)
	assert.Equal(t, "", extracted.AgentName)
	assert.Nil(t, extracted.Claims)
}

func TestIndividualGetters(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgentName(ctx, "agent-1")
	ctx = WithNamespace(ctx, "ns-1")
	ctx = WithSessionID(ctx, "sess-1")
	ctx = WithRequestID(ctx, "req-1")
	ctx = WithUserID(ctx, "user-1")
	ctx = WithUserRoles(ctx, "role-1")
	ctx = WithUserEmail(ctx, "email@test.com")
	ctx = WithAuthorization(ctx, "Bearer abc")
	ctx = WithProvider(ctx, "openai")
	ctx = WithModel(ctx, "gpt-4o")
	ctx = WithClaims(ctx, map[string]string{"key": "val"})

	assert.Equal(t, "agent-1", AgentName(ctx))
	assert.Equal(t, "ns-1", Namespace(ctx))
	assert.Equal(t, "sess-1", SessionID(ctx))
	assert.Equal(t, "req-1", RequestID(ctx))
	assert.Equal(t, "user-1", UserID(ctx))
	assert.Equal(t, "role-1", UserRoles(ctx))
	assert.Equal(t, "Bearer abc", Authorization(ctx))
	assert.Equal(t, "openai", Provider(ctx))
	assert.Equal(t, "gpt-4o", Model(ctx))
	assert.Equal(t, map[string]string{"key": "val"}, Claims(ctx))
}

func TestGettersOnEmptyContext(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", AgentName(ctx))
	assert.Equal(t, "", SessionID(ctx))
	assert.Equal(t, "", UserID(ctx))
	assert.Nil(t, Claims(ctx))
}

func TestToOutboundHeaders(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgentName(ctx, "my-agent")
	ctx = WithNamespace(ctx, "prod")
	ctx = WithUserID(ctx, "user-1")
	ctx = WithProvider(ctx, "claude")
	ctx = WithClaims(ctx, map[string]string{"team": "eng"})

	headers := ToOutboundHeaders(ctx)

	assert.Equal(t, "my-agent", headers[HeaderAgentName])
	assert.Equal(t, "prod", headers[HeaderNamespace])
	assert.Equal(t, "user-1", headers[HeaderUserID])
	assert.Equal(t, "claude", headers[HeaderProvider])
	assert.Equal(t, "eng", headers["x-omnia-claim-team"])

	// Empty fields should not be in headers
	_, hasSession := headers[HeaderSessionID]
	assert.False(t, hasSession)
}

func TestToGRPCMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgentName(ctx, "agent")
	ctx = WithModel(ctx, "model-1")

	md := ToGRPCMetadata(ctx)
	assert.Equal(t, "agent", md[HeaderAgentName])
	assert.Equal(t, "model-1", md[HeaderModel])
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"customer_id", "CustomerId"},
		{"first-name", "FirstName"},
		{"simple", "Simple"},
		{"already_Pascal_Case", "AlreadyPascalCase"},
		{"a_b_c", "ABC"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ToPascalCase(tt.input))
		})
	}
}

func TestPromoteScalarParams(t *testing.T) {
	args := map[string]any{
		"customer_id": "cust-123",
		"count":       float64(42),
		"enabled":     true,
		"nested_obj":  map[string]any{"key": "val"},
		"list":        []any{"a", "b"},
		"float_val":   float64(3.14),
	}

	headers := PromoteScalarParams(args)

	assert.Equal(t, "cust-123", headers["x-omnia-param-CustomerId"])
	assert.Equal(t, "42", headers["x-omnia-param-Count"])
	assert.Equal(t, "true", headers["x-omnia-param-Enabled"])
	assert.Equal(t, "3.14", headers["x-omnia-param-FloatVal"])

	// Nested objects and arrays should not be promoted
	_, hasNested := headers["x-omnia-param-NestedObj"]
	assert.False(t, hasNested)
	_, hasList := headers["x-omnia-param-List"]
	assert.False(t, hasList)
}

func TestPromoteScalarParams_EmptyArgs(t *testing.T) {
	headers := PromoteScalarParams(nil)
	assert.Empty(t, headers)
}

func TestPromoteScalarParams_JsonNumber(t *testing.T) {
	args := map[string]any{
		"amount": json.Number("99.99"),
	}
	headers := PromoteScalarParams(args)
	assert.Equal(t, "99.99", headers["x-omnia-param-Amount"])
}

func TestScalarToString(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
		ok       bool
	}{
		{"string", "hello", "hello", true},
		{"bool", true, "true", true},
		{"float64", float64(1.5), "1.5", true},
		{"float32", float32(2.5), "2.5", true},
		{"int", 42, "42", true},
		{"int64", int64(100), "100", true},
		{"json.Number", json.Number("7"), "7", true},
		{"nil", nil, "", false},
		{"map", map[string]any{}, "", false},
		{"slice", []string{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := scalarToString(tt.value)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestHeaderConstants(t *testing.T) {
	// Verify all header constants are lowercase (required for gRPC metadata).
	require.Equal(t, "x-omnia-agent-name", HeaderAgentName)
	require.Equal(t, "x-omnia-namespace", HeaderNamespace)
	require.Equal(t, "x-omnia-session-id", HeaderSessionID)
	require.Equal(t, "x-omnia-request-id", HeaderRequestID)
	require.Equal(t, "x-omnia-user-id", HeaderUserID)
	require.Equal(t, "x-omnia-user-roles", HeaderUserRoles)
	require.Equal(t, "authorization", HeaderAuthorization)
	require.Equal(t, "x-omnia-provider", HeaderProvider)
	require.Equal(t, "x-omnia-model", HeaderModel)
}

func TestExtractPropagationFields_WrongType(t *testing.T) {
	// Ensure type mismatch returns empty strings
	ctx := context.WithValue(context.Background(), ContextKeyAgentName, 12345)
	assert.Equal(t, "", AgentName(ctx))
}
