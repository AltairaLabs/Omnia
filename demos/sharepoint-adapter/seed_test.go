package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeeder_Run(t *testing.T) {
	var got []map[string]any
	mem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/institutional/memories", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		require.NoError(t, json.Unmarshal(body, &m))
		got = append(got, m)
		w.WriteHeader(http.StatusCreated)
	}))
	defer mem.Close()

	src := &fakeSource{docs: []Doc{
		{Title: "Policy", URL: "https://c/p.txt", Site: "site-1", Summary: "Policy"},
	}}
	s := &Seeder{src: src, memoryURL: mem.URL, workspaceID: "demo", http: mem.Client(), log: slog.Default()}

	n, err := s.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.Len(t, got, 1)
	assert.Equal(t, "demo", got[0]["workspace_id"])
	assert.Equal(t, "knowledge_reference", got[0]["type"])
	assert.Contains(t, got[0]["content"], "Policy")
	meta := got[0]["metadata"].(map[string]any)
	assert.Equal(t, "https://c/p.txt", meta["url"])
	assert.Equal(t, "site-1", meta["site"])
}

func TestSeeder_Run_MemoryError(t *testing.T) {
	mem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mem.Close()

	s := &Seeder{
		src:         &fakeSource{docs: []Doc{{Title: "x", URL: "u", Site: "s", Summary: "x"}}},
		memoryURL:   mem.URL,
		workspaceID: "demo",
		http:        mem.Client(),
		log:         slog.Default(),
	}
	_, err := s.Run(context.Background())
	assert.Error(t, err)
}
