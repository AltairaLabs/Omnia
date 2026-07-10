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
	"net/http"
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
		UserEmail:     "user@example.com",
		Authorization: "Bearer token123",
		Provider:      "claude",
		Model:         "claude-sonnet-4-20250514",
		Origin:        OriginAPIKey,
		Workspace:     "acme",
		Claims:        map[string]string{"team": "engineering", "region": "us-east"},
	}

	ctx = WithPropagationFields(ctx, fields)
	extracted := ExtractPropagationFields(ctx)

	assert.Equal(t, fields.AgentName, extracted.AgentName)
	assert.Equal(t, fields.Namespace, extracted.Namespace)
	assert.Equal(t, fields.SessionID, extracted.SessionID)
	assert.Equal(t, fields.RequestID, extracted.RequestID)
	assert.Equal(t, fields.UserID, extracted.UserID)
	assert.Equal(t, fields.UserEmail, extracted.UserEmail)
	assert.Equal(t, fields.Authorization, extracted.Authorization)
	assert.Equal(t, fields.Provider, extracted.Provider)
	assert.Equal(t, fields.Model, extracted.Model)
	assert.Equal(t, fields.Origin, extracted.Origin)
	assert.Equal(t, fields.Workspace, extracted.Workspace)
	assert.Equal(t, fields.Claims, extracted.Claims)
}

// TestOriginWorkspace_RoundTripThroughMetadata asserts identity.origin and
// identity.workspace survive the context -> outbound gRPC metadata hop (#1769).
// Before the fix these keys had no header mapping, so they were silently
// dropped at the facade->runtime boundary and never reached the broker.
func TestOriginWorkspace_RoundTripThroughMetadata(t *testing.T) {
	ctx := WithPropagationFields(context.Background(), &PropagationFields{
		Origin:    OriginManagementPlane,
		Workspace: "acme",
	})

	md := ToGRPCMetadata(ctx)
	assert.Equal(t, OriginManagementPlane, md[HeaderOrigin],
		"origin must be emitted on the wire so the broker sees identity.origin")
	assert.Equal(t, "acme", md[HeaderWorkspace],
		"workspace must be emitted on the wire so the broker sees identity.workspace")
}

func TestPropagationFields_RoundTripsIdentity(t *testing.T) {
	t.Parallel()
	id := &AuthenticatedIdentity{
		Origin:    OriginManagementPlane,
		Subject:   "admin@example.com",
		EndUser:   "admin@example.com",
		Workspace: "default",
		Agent:     "test-agent",
		Claims:    map[string]string{"tier": "pro"},
	}
	fields := &PropagationFields{
		AgentName: "test-agent",
		Identity:  id,
	}
	ctx := WithPropagationFields(context.Background(), fields)

	// ExtractPropagationFields rehydrates Identity.
	extracted := ExtractPropagationFields(ctx)
	require.NotNil(t, extracted.Identity)
	assert.Equal(t, id.Subject, extracted.Identity.Subject)

	// IdentityFromContext returns the same pointer (no cloning — Identity
	// is immutable from the consumer's perspective, so aliasing is fine).
	assert.Same(t, id, IdentityFromContext(ctx))
}

func TestWithIdentity_Nil(t *testing.T) {
	t.Parallel()
	// WithIdentity(ctx, nil) must be a no-op: calling it should not
	// attach a typed-nil sentinel that breaks downstream consumers.
	ctx := context.Background()
	out := WithIdentity(ctx, nil)
	assert.Equal(t, ctx, out)
	assert.Nil(t, IdentityFromContext(out))
}

func TestIdentityFromContext_MissingReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, IdentityFromContext(context.Background()))
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
	ctx = WithUserEmail(ctx, "email@test.com")
	ctx = WithAuthorization(ctx, "Bearer abc")
	ctx = WithProvider(ctx, "openai")
	ctx = WithModel(ctx, "gpt-4o")
	ctx = WithOrigin(ctx, OriginAPIKey)
	ctx = WithWorkspace(ctx, "acme")
	ctx = WithClaims(ctx, map[string]string{"key": "val"})

	assert.Equal(t, "agent-1", AgentName(ctx))
	assert.Equal(t, "ns-1", Namespace(ctx))
	assert.Equal(t, "sess-1", SessionID(ctx))
	assert.Equal(t, "req-1", RequestID(ctx))
	assert.Equal(t, "user-1", UserID(ctx))
	assert.Equal(t, "Bearer abc", Authorization(ctx))
	assert.Equal(t, "openai", Provider(ctx))
	assert.Equal(t, "gpt-4o", Model(ctx))
	assert.Equal(t, OriginAPIKey, Origin(ctx))
	assert.Equal(t, "acme", Workspace(ctx))
	assert.Equal(t, map[string]string{"key": "val"}, Claims(ctx))
}

func TestGettersOnEmptyContext(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", AgentName(ctx))
	assert.Equal(t, "", SessionID(ctx))
	assert.Equal(t, "", UserID(ctx))
	assert.Equal(t, "", Origin(ctx))
	assert.Equal(t, "", Workspace(ctx))
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

// TestOutboundHeaders_NeverLeakAuthorization is a security regression guard:
// the caller's inbound bearer token, held in-process via WithAuthorization,
// must NEVER be re-emitted as an outbound header to a tool. Doing so leaks the
// user's credential to arbitrary third-party upstreams AND overwrites a tool's
// own authSecretRef credential (the runtime applies that first). Identity still
// travels via X-Omnia-Claim-*; the raw token stays readable in-process only,
// for a future on-behalf-of exchange.
func TestOutboundHeaders_NeverLeakAuthorization(t *testing.T) {
	ctx := context.Background()
	ctx = WithAuthorization(ctx, "Bearer caller-jwt")
	ctx = WithUserID(ctx, "user-1")
	ctx = WithClaims(ctx, map[string]string{"team": "eng"})

	headers := ToOutboundHeaders(ctx)
	md := ToGRPCMetadata(ctx)

	_, httpHas := headers[HeaderAuthorization]
	assert.False(t, httpHas, "outbound HTTP headers must not carry the caller's Authorization token")
	_, grpcHas := md[HeaderAuthorization]
	assert.False(t, grpcHas, "outbound gRPC metadata must not carry the caller's Authorization token")

	// Identity still propagates safely, and the token remains available
	// in-process (never emitted) for a future on-behalf-of exchange.
	assert.Equal(t, "user-1", headers[HeaderUserID])
	assert.Equal(t, "eng", headers["x-omnia-claim-team"])
	assert.Equal(t, "Bearer caller-jwt", Authorization(ctx),
		"token must stay readable in-process for future OBO, just never sent to a tool")
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
	require.Equal(t, "authorization", HeaderAuthorization)
	require.Equal(t, "x-omnia-provider", HeaderProvider)
	require.Equal(t, "x-omnia-model", HeaderModel)
}

func TestExtractPropagationFields_WrongType(t *testing.T) {
	// Ensure type mismatch returns empty strings
	ctx := context.WithValue(context.Background(), ContextKeyAgentName, 12345)
	assert.Equal(t, "", AgentName(ctx))
}

func TestConsentGrantsRoundTrip(t *testing.T) {
	grants := []string{"memory", "analytics"}
	ctx := WithConsentGrants(context.Background(), grants)
	got := ConsentGrantsFromContext(ctx)
	require.Equal(t, grants, got)
}

func TestConsentGrantsFromContextEmpty(t *testing.T) {
	got := ConsentGrantsFromContext(context.Background())
	assert.Nil(t, got)
}

func TestToGRPCMetadataIncludesConsentGrants(t *testing.T) {
	ctx := WithConsentGrants(context.Background(), []string{"memory", "analytics"})
	headers := ToGRPCMetadata(ctx)
	assert.Equal(t, "memory,analytics", headers[HeaderConsentGrants])
}

func TestToGRPCMetadataOmitsConsentGrantsWhenEmpty(t *testing.T) {
	headers := ToGRPCMetadata(context.Background())
	_, ok := headers[HeaderConsentGrants]
	assert.False(t, ok, "consent grants header should be absent when no grants set")
}

func TestWithPropagationFieldsConsentGrants(t *testing.T) {
	fields := &PropagationFields{
		ConsentGrants: []string{"memory"},
	}
	ctx := WithPropagationFields(context.Background(), fields)
	got := ConsentGrantsFromContext(ctx)
	require.Equal(t, []string{"memory"}, got)
}

func TestExtractPropagationFieldsConsentGrants(t *testing.T) {
	grants := []string{"memory", "analytics"}
	ctx := WithConsentGrants(context.Background(), grants)
	fields := ExtractPropagationFields(ctx)
	assert.Equal(t, grants, fields.ConsentGrants)
}

func TestWithConsentLayer_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if got := ConsentLayerFromContext(ctx); got != "" {
		t.Errorf("empty context: got %q, want \"\"", got)
	}
	ctx = WithConsentLayer(ctx, "session")
	if got := ConsentLayerFromContext(ctx); got != "session" {
		t.Errorf("after WithConsentLayer: got %q, want \"session\"", got)
	}
}

func TestPropagationFields_RoundTripsConsentLayer(t *testing.T) {
	ctx := WithPropagationFields(context.Background(), &PropagationFields{
		ConsentGrants: []string{"memory:identity"},
		ConsentLayer:  "per-message",
	})
	got := ExtractPropagationFields(ctx)
	if got.ConsentLayer != "per-message" {
		t.Errorf("ConsentLayer = %q, want \"per-message\"", got.ConsentLayer)
	}
}

func TestToOutboundHeaders_IncludesConsentLayer(t *testing.T) {
	ctx := WithConsentLayer(context.Background(), "session")
	headers := ToOutboundHeaders(ctx)
	if headers[HeaderConsentLayer] != "session" {
		t.Errorf("headers[%q] = %q, want \"session\"", HeaderConsentLayer, headers[HeaderConsentLayer])
	}
}

func TestToOutboundHeaders_OmitsEmptyConsentLayer(t *testing.T) {
	headers := ToOutboundHeaders(context.Background())
	if v, present := headers[HeaderConsentLayer]; present {
		t.Errorf("HeaderConsentLayer present without value: %q", v)
	}
}

func TestCanonicalClaimHeader(t *testing.T) {
	cases := []struct {
		name  string
		claim string
		want  string
	}{
		{"lowercase single segment", "tier", "X-Omnia-Claim-Tier"},
		{"already title-cased", "Team", "X-Omnia-Claim-Team"},
		{"hyphenated segments title-cased", "customer-id", "X-Omnia-Claim-Customer-Id"},
		{"underscore is not a separator", "customer_id", "X-Omnia-Claim-Customer_id"},
		{"upper input normalized", "TEAM", "X-Omnia-Claim-Team"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, CanonicalClaimHeader(tc.claim))
		})
	}
}

// TestCanonicalClaimHeader_MatchesWireCanonicalization is the regression guard
// for #1766: the canonical key a read path looks up MUST equal the key a claim
// header lands under once emitted through an http.Request. If the emit prefix
// and the canonical helper ever diverge, this fails.
func TestCanonicalClaimHeader_MatchesWireCanonicalization(t *testing.T) {
	for _, claim := range []string{"tier", "Team", "customer-id", "customer_id"} {
		ctx := WithClaims(context.Background(), map[string]string{claim: "v"})
		emitted := ToOutboundHeaders(ctx)

		req := &http.Request{Header: http.Header{}}
		for k, v := range emitted {
			req.Header.Set(k, v)
		}

		wireKey := ""
		for k := range req.Header {
			wireKey = k
		}
		assert.Equal(t, CanonicalClaimHeader(claim), wireKey,
			"claim %q: helper key must match the on-wire canonical key", claim)
	}
}
