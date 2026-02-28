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
	"fmt"
	"strings"
)

// ClaimMappingRule represents a single claim-to-header mapping rule.
type ClaimMappingRule struct {
	// Claim is the JWT claim name (supports dot-notation for nested claims).
	Claim string
	// Header is the HTTP header name for the extracted claim value.
	Header string
}

// ExtractClaimsFromJWT extracts configured claims from a JWT token and returns
// them as a header name -> value map. The JWT is decoded without signature
// verification because Istio has already validated it.
func ExtractClaimsFromJWT(token string, rules []ClaimMappingRule) (map[string]string, error) {
	if token == "" || len(rules) == 0 {
		return nil, nil
	}

	// Strip "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")

	claims, err := decodeJWTPayload(token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	return mapClaimsToHeaders(claims, rules), nil
}

// decodeJWTPayload decodes the payload (second part) of a JWT without signature
// verification. This is safe because Istio has already validated the JWT.
func decodeJWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT payload JSON: %w", err)
	}
	return claims, nil
}

// mapClaimsToHeaders extracts claim values using the rules and returns a header map.
func mapClaimsToHeaders(claims map[string]any, rules []ClaimMappingRule) map[string]string {
	result := make(map[string]string)
	for _, rule := range rules {
		value := resolveClaimValue(claims, rule.Claim)
		if value != "" {
			result[rule.Header] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// resolveClaimValue resolves a claim value from the claims map.
// Supports dot-notation for nested claims (e.g., "org.team").
func resolveClaimValue(claims map[string]any, claimPath string) string {
	parts := strings.Split(claimPath, ".")
	var current any = claims

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}
	return scalarToStr(current)
}

// scalarToStr converts a claim value to string. Only scalar types are supported.
func scalarToStr(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case json.Number:
		return val.String()
	default:
		return ""
	}
}
