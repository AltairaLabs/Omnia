/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

func TestNewSessionUsageEmitter_EmptyURLReturnsNil(t *testing.T) {
	assert.Nil(t, newSessionUsageEmitter("", logr.Discard()))
	assert.Nil(t, newSessionUsageEmitter("   ", logr.Discard()))
}

func TestSessionUsageEmitter_PostsProviderUsage(t *testing.T) {
	type captured struct {
		path string
		body []providerUsagePayload
	}
	got := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload []providerUsagePayload
		_ = json.Unmarshal(raw, &payload)
		got <- captured{path: r.URL.Path, body: payload}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	emitter := newSessionUsageEmitter(srv.URL, logr.Discard())
	require.NotNil(t, emitter)

	emitter.EmitProviderUsage(t.Context(), memory.ProviderUsageRecord{
		Namespace:     "omnia-demo",
		WorkspaceName: "demo",
		Provider:      "azure",
		ProviderName:  "azure-embed",
		Model:         "text-embedding-3-small",
		Source:        memory.EmbeddingUsageSource,
		InputTokens:   512,
		CallCount:     1,
	})

	select {
	case c := <-got:
		assert.Equal(t, "/api/v1/provider-usage", c.path)
		require.Len(t, c.body, 1)
		assert.Equal(t, "omnia-demo", c.body[0].Namespace)
		assert.Equal(t, "azure", c.body[0].Provider)
		assert.Equal(t, "azure-embed", c.body[0].ProviderName)
		assert.Equal(t, memory.EmbeddingUsageSource, c.body[0].Source)
		assert.Equal(t, int64(512), c.body[0].InputTokens)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the emit POST")
	}
}

func TestSessionUsageEmitter_SendsServiceAccountToken(t *testing.T) {
	const token = "sa-emitter-token-xyz"
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte(token+"\n"), 0o600))
	t.Setenv(tokenPathEnv, tokenPath)

	gotAuth := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotAuth <- r.Header.Get("Authorization"):
		default:
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	emitter := newSessionUsageEmitter(srv.URL, logr.Discard())
	require.NotNil(t, emitter)
	emitter.EmitProviderUsage(t.Context(), memory.ProviderUsageRecord{Namespace: "ns", Source: memory.EmbeddingUsageSource})

	select {
	case auth := <-gotAuth:
		assert.Equal(t, "Bearer "+token, auth)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the emit POST")
	}
}

func TestSessionUsageEmitter_NoTokenFile_NoAuthHeader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(tokenPathEnv, filepath.Join(dir, "does-not-exist"))

	gotAuth := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotAuth <- r.Header.Get("Authorization"):
		default:
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	emitter := newSessionUsageEmitter(srv.URL, logr.Discard())
	require.NotNil(t, emitter)
	emitter.EmitProviderUsage(t.Context(), memory.ProviderUsageRecord{Namespace: "ns", Source: memory.EmbeddingUsageSource})

	select {
	case auth := <-gotAuth:
		assert.Empty(t, auth)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the emit POST")
	}
}

func TestSessionUsageEmitter_BuildsCorrectURL(t *testing.T) {
	e, ok := newSessionUsageEmitter("http://session-demo-default:8080/", logr.Discard()).(*sessionUsageEmitter)
	require.True(t, ok)
	assert.Equal(t, "http://session-demo-default:8080/api/v1/provider-usage", e.url)
}
