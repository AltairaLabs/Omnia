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
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT creates a minimal JWT with the given payload claims.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadB64 + ".signature"
}

func TestExtractClaimsFromJWT_BasicClaims(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"team":        "engineering",
		"region":      "us-east",
		"customer_id": "cust-123",
		"extra":       "ignored",
	})

	rules := []ClaimMappingRule{
		{Claim: "team", Header: "X-Omnia-Claim-Team"},
		{Claim: "region", Header: "X-Omnia-Claim-Region"},
		{Claim: "customer_id", Header: "X-Omnia-Claim-Customer-Id"},
	}

	result, err := ExtractClaimsFromJWT("Bearer "+token, rules)
	require.NoError(t, err)

	assert.Equal(t, "engineering", result["X-Omnia-Claim-Team"])
	assert.Equal(t, "us-east", result["X-Omnia-Claim-Region"])
	assert.Equal(t, "cust-123", result["X-Omnia-Claim-Customer-Id"])
	assert.Len(t, result, 3)
}

func TestExtractClaimsFromJWT_NestedClaims(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"org": map[string]any{
			"team": "platform",
			"nested": map[string]any{
				"level": "deep",
			},
		},
	})

	rules := []ClaimMappingRule{
		{Claim: "org.team", Header: "X-Omnia-Claim-Team"},
		{Claim: "org.nested.level", Header: "X-Omnia-Claim-Level"},
	}

	result, err := ExtractClaimsFromJWT(token, rules)
	require.NoError(t, err)

	assert.Equal(t, "platform", result["X-Omnia-Claim-Team"])
	assert.Equal(t, "deep", result["X-Omnia-Claim-Level"])
}

func TestExtractClaimsFromJWT_MissingClaims(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"team": "engineering",
	})

	rules := []ClaimMappingRule{
		{Claim: "team", Header: "X-Omnia-Claim-Team"},
		{Claim: "missing", Header: "X-Omnia-Claim-Missing"},
	}

	result, err := ExtractClaimsFromJWT(token, rules)
	require.NoError(t, err)

	assert.Equal(t, "engineering", result["X-Omnia-Claim-Team"])
	_, exists := result["X-Omnia-Claim-Missing"]
	assert.False(t, exists)
}

func TestExtractClaimsFromJWT_NumericAndBoolClaims(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"level":  float64(5),
		"active": true,
	})

	rules := []ClaimMappingRule{
		{Claim: "level", Header: "X-Omnia-Claim-Level"},
		{Claim: "active", Header: "X-Omnia-Claim-Active"},
	}

	result, err := ExtractClaimsFromJWT(token, rules)
	require.NoError(t, err)

	assert.Equal(t, "5", result["X-Omnia-Claim-Level"])
	assert.Equal(t, "true", result["X-Omnia-Claim-Active"])
}

func TestExtractClaimsFromJWT_EmptyToken(t *testing.T) {
	rules := []ClaimMappingRule{{Claim: "team", Header: "X-Omnia-Claim-Team"}}
	result, err := ExtractClaimsFromJWT("", rules)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractClaimsFromJWT_EmptyRules(t *testing.T) {
	token := makeJWT(t, map[string]any{"team": "eng"})
	result, err := ExtractClaimsFromJWT(token, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractClaimsFromJWT_InvalidJWT(t *testing.T) {
	rules := []ClaimMappingRule{{Claim: "team", Header: "X-Omnia-Claim-Team"}}
	_, err := ExtractClaimsFromJWT("not.a.jwt.token", rules)
	assert.Error(t, err)
}

func TestExtractClaimsFromJWT_InvalidBase64(t *testing.T) {
	rules := []ClaimMappingRule{{Claim: "team", Header: "X-Omnia-Claim-Team"}}
	_, err := ExtractClaimsFromJWT("header.!!!invalid!!!.sig", rules)
	assert.Error(t, err)
}

func TestExtractClaimsFromJWT_InvalidJSON(t *testing.T) {
	rules := []ClaimMappingRule{{Claim: "team", Header: "X-Omnia-Claim-Team"}}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`not json`))
	_, err := ExtractClaimsFromJWT(header+"."+payload+".sig", rules)
	assert.Error(t, err)
}

func TestExtractClaimsFromJWT_BearerPrefix(t *testing.T) {
	token := makeJWT(t, map[string]any{"team": "eng"})
	rules := []ClaimMappingRule{{Claim: "team", Header: "X-Omnia-Claim-Team"}}

	// Test with "Bearer " prefix
	result, err := ExtractClaimsFromJWT("Bearer "+token, rules)
	require.NoError(t, err)
	assert.Equal(t, "eng", result["X-Omnia-Claim-Team"])

	// Test with lowercase "bearer " prefix
	result, err = ExtractClaimsFromJWT("bearer "+token, rules)
	require.NoError(t, err)
	assert.Equal(t, "eng", result["X-Omnia-Claim-Team"])
}

func TestExtractClaimsFromJWT_ArrayClaimSkipped(t *testing.T) {
	token := makeJWT(t, map[string]any{
		"roles": []any{"admin", "viewer"},
	})
	rules := []ClaimMappingRule{{Claim: "roles", Header: "X-Omnia-Claim-Roles"}}

	result, err := ExtractClaimsFromJWT(token, rules)
	require.NoError(t, err)
	// Array claims should not be mapped (non-scalar)
	assert.Nil(t, result)
}

func TestResolveClaimValue(t *testing.T) {
	claims := map[string]any{
		"simple": "value",
		"nested": map[string]any{
			"key": "nested-value",
		},
		"number": float64(42),
		"bool":   true,
		"array":  []any{"a"},
	}

	assert.Equal(t, "value", resolveClaimValue(claims, "simple"))
	assert.Equal(t, "nested-value", resolveClaimValue(claims, "nested.key"))
	assert.Equal(t, "42", resolveClaimValue(claims, "number"))
	assert.Equal(t, "true", resolveClaimValue(claims, "bool"))
	assert.Equal(t, "", resolveClaimValue(claims, "array"))
	assert.Equal(t, "", resolveClaimValue(claims, "nonexistent"))
	assert.Equal(t, "", resolveClaimValue(claims, "nested.nonexistent"))
	assert.Equal(t, "", resolveClaimValue(claims, "simple.invalid"))
}
