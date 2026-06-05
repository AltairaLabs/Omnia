package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedFakeSource lets tests control List and per-URL Fetch independently.
type seedFakeSource struct {
	docs     []Doc
	listErr  error
	contents map[string]*DocContent // keyed by URL
	fetchErr map[string]error       // keyed by URL
}

func (f *seedFakeSource) List(_ context.Context) ([]Doc, error) {
	return f.docs, f.listErr
}

func (f *seedFakeSource) Fetch(_ context.Context, url string) (*DocContent, error) {
	if err, ok := f.fetchErr[url]; ok {
		return nil, err
	}
	if c, ok := f.contents[url]; ok {
		return c, nil
	}
	return &DocContent{Title: "doc", URL: url, Text: "default text"}, nil
}

// newIngestServer builds an httptest server that records ingest request bodies.
// statusFn lets tests return different status codes per call count.
func newIngestServer(t *testing.T, statusFn func(n int) int) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var got []map[string]any
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/institutional/ingest", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		require.NoError(t, json.Unmarshal(body, &m))
		got = append(got, m)
		code := statusFn(callCount)
		callCount++
		w.WriteHeader(code)
	}))
	return srv, &got
}

func TestSeeder_Run_HappyPath(t *testing.T) {
	srv, got := newIngestServer(t, func(_ int) int { return http.StatusAccepted })
	defer srv.Close()

	src := &seedFakeSource{
		docs: []Doc{
			{Title: "Policy", URL: "https://sp/policy.docx", Site: "site-1", Summary: "s"},
			{Title: "Guide", URL: "https://sp/guide.docx", Site: "site-2", Summary: "g"},
		},
		contents: map[string]*DocContent{
			"https://sp/policy.docx": {Title: "Policy", URL: "https://sp/policy.docx", Text: "full policy text"},
			"https://sp/guide.docx":  {Title: "Guide", URL: "https://sp/guide.docx", Text: "full guide text"},
		},
	}
	s := &Seeder{src: src, memoryURL: srv.URL, workspaceID: "demo", http: srv.Client(), log: slog.Default()}

	n, err := s.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	require.Len(t, *got, 2)

	first := (*got)[0]
	assert.Equal(t, "demo", first["workspace_id"])
	assert.Equal(t, "Policy", first["title"])
	assert.Equal(t, "https://sp/policy.docx", first["url"])
	assert.Equal(t, "site-1", first["site"])
	assert.Equal(t, "full policy text", first["text"])

	second := (*got)[1]
	assert.Equal(t, "Guide", second["title"])
	assert.Equal(t, "full guide text", second["text"])
}

func TestSeeder_Run_FetchErrorSkips(t *testing.T) {
	srv, got := newIngestServer(t, func(_ int) int { return http.StatusAccepted })
	defer srv.Close()

	src := &seedFakeSource{
		docs: []Doc{
			{Title: "Bad", URL: "https://sp/bad.docx", Site: "site-1", Summary: ""},
			{Title: "Good", URL: "https://sp/good.docx", Site: "site-1", Summary: ""},
		},
		fetchErr: map[string]error{
			"https://sp/bad.docx": errors.New("graph: 403 forbidden"),
		},
		contents: map[string]*DocContent{
			"https://sp/good.docx": {Title: "Good", URL: "https://sp/good.docx", Text: "good text"},
		},
	}
	s := &Seeder{src: src, memoryURL: srv.URL, workspaceID: "ws1", http: srv.Client(), log: slog.Default()}

	n, err := s.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.Len(t, *got, 1)
	assert.Equal(t, "good text", (*got)[0]["text"])
}

func TestSeeder_Run_IngestErrorSkips(t *testing.T) {
	// Ingest server always returns 500 — all docs skipped, no error returned.
	srv, got := newIngestServer(t, func(_ int) int { return http.StatusInternalServerError })
	defer srv.Close()

	src := &seedFakeSource{
		docs: []Doc{
			{Title: "Doc", URL: "https://sp/doc.docx", Site: "s", Summary: ""},
		},
		contents: map[string]*DocContent{
			"https://sp/doc.docx": {Title: "Doc", URL: "https://sp/doc.docx", Text: "some text"},
		},
	}
	s := &Seeder{src: src, memoryURL: srv.URL, workspaceID: "ws2", http: srv.Client(), log: slog.Default()}

	n, err := s.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	// Body was still received (just returned 500).
	require.Len(t, *got, 1)
}

func TestSeeder_Run_ListErrorPropagates(t *testing.T) {
	srv, _ := newIngestServer(t, func(_ int) int { return http.StatusAccepted })
	defer srv.Close()

	listErr := errors.New("graph: service unavailable")
	src := &seedFakeSource{listErr: listErr}
	s := &Seeder{src: src, memoryURL: srv.URL, workspaceID: "ws3", http: srv.Client(), log: slog.Default()}

	n, err := s.Run(context.Background())

	assert.Equal(t, 0, n)
	require.Error(t, err)
	assert.ErrorContains(t, err, "list documents")
}
