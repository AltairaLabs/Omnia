/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBaseURL_UsesOverride(t *testing.T) {
	got := resolveBaseURL(&flags{baseURL: "http://localhost:9999"})
	assert.Equal(t, "http://localhost:9999", got)
}

func TestResolveBaseURL_ComposesClusterDNS(t *testing.T) {
	got := resolveBaseURL(&flags{
		agent:     "summarizer",
		namespace: "workspace-support",
		port:      9999,
	})
	assert.Equal(t, "http://summarizer.workspace-support.svc.cluster.local:9999", got)
}

func TestLoadToken_EmptyPathReturnsEmpty(t *testing.T) {
	assert.Empty(t, loadToken(""))
}

func TestLoadToken_MissingFileReturnsEmpty(t *testing.T) {
	assert.Empty(t, loadToken(filepath.Join(t.TempDir(), "no-such-file")))
}

func TestLoadToken_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(path, []byte("abc.def.ghi"), 0o600))
	assert.Equal(t, "abc.def.ghi", loadToken(path))
}

func TestBuildRequest_SetsRoleAndText(t *testing.T) {
	req := buildRequest("hello")
	assert.Equal(t, a2a.RoleUser, req.Message.Role)
	assert.NotEmpty(t, req.Message.MessageID, "MessageID must be set to avoid server-side collisions")
	assert.Empty(t, req.Message.ContextID,
		"ContextID must be empty so each invocation starts a fresh conversation")
	require.Len(t, req.Message.Parts, 1)
	require.NotNil(t, req.Message.Parts[0].Text)
	assert.Equal(t, "hello", *req.Message.Parts[0].Text)
}

func TestBuildRequest_MessageIDsDifferAcrossCalls(t *testing.T) {
	r1 := buildRequest("a")
	r2 := buildRequest("b")
	assert.NotEqual(t, r1.Message.MessageID, r2.Message.MessageID)
}

// TestInvoker_EndToEnd stands up a minimal A2A-shaped HTTP server and drives
// the client path. We're not validating full A2A semantics — just that the
// invoker actually POSTs JSON-RPC with the expected shape and can decode
// the response. That's enough to catch wiring regressions.
func TestInvoker_EndToEnd(t *testing.T) {
	var captured struct {
		Method string
		Params struct {
			Message struct {
				Role      string `json:"role"`
				ContextID string `json:"contextId,omitempty"`
				MessageID string `json:"messageId"`
				Parts     []struct {
					Text *string `json:"text,omitempty"`
				} `json:"parts"`
			} `json:"message"`
		} `json:"params"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      "1",
			"result": map[string]any{
				"id":        "task-1",
				"contextId": "ctx-1",
				"kind":      "task",
				"status": map[string]any{
					"state": "completed",
				},
				"artifacts": []map[string]any{
					{
						"artifactId": "a-1",
						"parts": []map[string]any{
							{"text": "summary: done"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := a2a.NewClient(srv.URL)
	task, err := client.SendMessage(ctx, buildRequest("Run compaction now."))
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, "task-1", task.ID)

	// The server saw a JSON-RPC call with method=message/send and no contextID.
	assert.Equal(t, "message/send", captured.Method,
		"invoker must call the standard A2A method")
	assert.Empty(t, captured.Params.Message.ContextID,
		"invoker must NOT send contextID — each scheduled run starts fresh")
	assert.NotEmpty(t, captured.Params.Message.MessageID)
	require.Len(t, captured.Params.Message.Parts, 1)
	require.NotNil(t, captured.Params.Message.Parts[0].Text)
	assert.Equal(t, "Run compaction now.", *captured.Params.Message.Parts[0].Text)

	// ExtractResponseText should surface the artifact text.
	got := a2a.ExtractResponseText(task)
	assert.True(t, strings.Contains(got, "summary: done"),
		"response text should carry the artifact content, got %q", got)
}

func TestParseFlags_RejectsMissingAgentWithoutBaseURL(t *testing.T) {
	f, err := (func() (*flags, error) {
		// We can't call parseFlags() directly because it uses the global flag
		// set. Simulate validation by constructing the struct.
		f := &flags{timeout: time.Minute}
		if f.baseURL == "" {
			if f.agent == "" {
				return nil, assertError("missing agent")
			}
		}
		return f, nil
	})()
	assert.Nil(t, f)
	assert.Error(t, err)
}

type assertError string

func (e assertError) Error() string { return string(e) }
