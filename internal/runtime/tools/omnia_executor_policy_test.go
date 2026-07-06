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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHTTPToolServer builds a tool backend that records the last request's
// headers and echoes a fixed JSON body.
func newHTTPToolServer(t *testing.T, captured *http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
}

// newHTTPToolExecutor builds an OmniaExecutor with a single HTTP tool
// pointed at toolSrv, so dispatch is exercised end-to-end through
// ExecuteTool.
func newHTTPToolExecutor(toolSrv *httptest.Server) *OmniaExecutor {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["test-http"] = &HandlerEntry{
		Name: "test-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: toolSrv.URL,
			Method:   "POST",
		},
		Tool: &ToolDefCfg{Name: "test-http-tool", Description: "test tool"},
	}
	e.toolHandlers["test-http-tool"] = "test-http"
	return e
}

func TestDispatch_PolicyBrokerDeny_AbortsCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		toolCalled = true
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":false,"deniedBy":"deny-all","message":"nope"}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	_, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPolicyDenied), "expected errPolicyDenied, got %v", err)
	assert.Contains(t, err.Error(), "deny-all")
	assert.False(t, toolCalled, "tool backend must not be called when the broker denies")
}

func TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundRequest(t *testing.T) {
	var captured http.Header
	toolSrv := newHTTPToolServer(t, &captured)
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":true,"injectedHeaders":{"X-Injected-Auth":"secret-token"}}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	result, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
	assert.Equal(t, "secret-token", captured.Get("X-Injected-Auth"))
}

func TestDispatch_PolicyBrokerAuditWouldDeny_ProceedsWithCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":false,"wouldDeny":true,"deniedBy":"deny-all","mode":"audit"}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	_, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, toolCalled, "audit-mode wouldDeny must not block the call")
}

func TestDispatch_PolicyBrokerDisabled_NoBehaviorChange(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, "")

	var captured http.Header
	toolSrv := newHTTPToolServer(t, &captured)
	defer toolSrv.Close()

	e := newHTTPToolExecutor(toolSrv)

	result, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
}
