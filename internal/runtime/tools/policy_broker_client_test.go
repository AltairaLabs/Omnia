/*
Copyright 2026.

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/policy"
)

// jsonHandler builds an httptest handler that writes the given body as a
// 200 JSON response.
func jsonHandler(t *testing.T, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}

func TestPolicyBrokerClient_Disabled(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, "")

	c := NewPolicyBrokerClient(logr.Discard())
	assert.False(t, c.Enabled())

	decision, err := c.Decide(context.Background(), "tool", "registry", json.RawMessage(`{"x":1}`))
	require.NoError(t, err)
	assert.True(t, decision.Allow)
	assert.Empty(t, decision.InjectedHeaders)
}

func TestPolicyBrokerClient_Allow(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(t, `{"allow":true}`))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())
	require.True(t, c.Enabled())

	decision, err := c.Decide(context.Background(), "tool", "registry", json.RawMessage(`{"x":1}`))
	require.NoError(t, err)
	assert.True(t, decision.Allow)
}

func TestPolicyBrokerClient_Deny(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(t, `{"allow":false,"deniedBy":"rule1","message":"nope"}`))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.False(t, decision.Allow)
	assert.Equal(t, "rule1", decision.DeniedBy)
	assert.Equal(t, "nope", decision.Message)
}

func TestPolicyBrokerClient_AllowWithInjectedHeaders(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(t, `{"allow":true,"injectedHeaders":{"X-Extra-Auth":"abc123"}}`))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.True(t, decision.Allow)
	assert.Equal(t, "abc123", decision.InjectedHeaders["X-Extra-Auth"])
}

func TestPolicyBrokerClient_AuditWouldDeny(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(t, `{"allow":false,"wouldDeny":true,"deniedBy":"rule1","mode":"audit"}`))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.False(t, decision.Allow)
	assert.True(t, decision.WouldDeny)
}

// deadURL returns the URL of a server that has already been closed, so any
// request to it fails with connection-refused — used to simulate the broker
// being unreachable.
func deadURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(jsonHandler(t, `{}`))
	srv.Close()
	return srv.URL
}

func TestPolicyBrokerClient_FailClosedByDefault(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, deadURL(t))
	t.Setenv(envPolicyBrokerFailMode, "")

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.False(t, decision.Allow)
	assert.Equal(t, policyDeniedByTransport, decision.DeniedBy)
}

func TestPolicyBrokerClient_FailOpen(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, deadURL(t))
	t.Setenv(envPolicyBrokerFailMode, policyBrokerFailModeOpen)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.True(t, decision.Allow)
}

func TestWithInjectedHeaders_EmptyIsNoOp(t *testing.T) {
	ctx := context.Background()
	got := WithInjectedHeaders(ctx, nil)
	assert.Equal(t, ctx, got)
	assert.Nil(t, InjectedHeadersFromContext(got))

	got = WithInjectedHeaders(ctx, map[string]string{"X-Foo": "bar"})
	assert.Equal(t, "bar", InjectedHeadersFromContext(got)["X-Foo"])
}

func TestDecodeArgsMap_MalformedJSONReturnsNil(t *testing.T) {
	assert.Nil(t, decodeArgsMap(json.RawMessage(`not-json`)))
	assert.Nil(t, decodeArgsMap(nil))
}

func TestPolicyBrokerClient_MalformedResponseFailsClosed(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(t, `not-json`))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.False(t, decision.Allow)
	assert.Equal(t, policyDeniedByTransport, decision.DeniedBy)
}

func TestPolicyBrokerClient_NonOKStatusFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	decision, err := c.Decide(context.Background(), "tool", "registry", nil)
	require.NoError(t, err)
	assert.False(t, decision.Allow)
}

// TestPolicyBrokerClient_RequestShape verifies the DecisionRequest sent to
// the broker carries the tool/registry identification headers (so the
// broker can select the right ToolPolicy) plus the parsed body and identity
// — the shape ee/pkg/policy.BrokerHandler expects.
func TestPolicyBrokerClient_RequestShape(t *testing.T) {
	var captured policy.DecisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"allow":true}`))
	}))
	defer srv.Close()
	t.Setenv(envPolicyBrokerURL, srv.URL)

	c := NewPolicyBrokerClient(logr.Discard())

	ctx := policy.WithIdentity(context.Background(), &policy.AuthenticatedIdentity{
		Origin:    policy.OriginOIDC,
		Subject:   "user-1",
		EndUser:   "user-1",
		Workspace: "ws-1",
		Agent:     "agent-1",
		Role:      policy.RoleEditor,
		Claims:    map[string]string{"team": "eng"},
	})

	_, err := c.Decide(ctx, "my-tool", "my-registry", json.RawMessage(`{"customer_id":"cust-1"}`))
	require.NoError(t, err)

	assert.Equal(t, "my-tool", captured.Headers["X-Omnia-Tool-Name"])
	assert.Equal(t, "my-registry", captured.Headers["X-Omnia-Tool-Registry"])
	assert.Equal(t, "cust-1", captured.Body["customer_id"])
	require.NotNil(t, captured.Identity)
	assert.Equal(t, "ws-1", captured.Identity.Workspace)
	assert.Equal(t, "eng", captured.Identity.Claims["team"])
}
