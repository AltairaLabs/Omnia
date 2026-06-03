package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	docs    []Doc
	content *DocContent
	err     error
}

func (f *fakeSource) List(_ context.Context) ([]Doc, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

func (f *fakeSource) Fetch(_ context.Context, _ string) (*DocContent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.content, nil
}

func newTestServer(src DocSource) *httptest.Server {
	return httptest.NewServer(NewServer(src, slog.Default()).Routes())
}

func TestServer_Health(t *testing.T) {
	ts := newTestServer(&fakeSource{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_List(t *testing.T) {
	ts := newTestServer(&fakeSource{docs: []Doc{{Title: "a", URL: "u", Site: "s", Summary: "a"}}})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/list")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), `"docs"`)
	assert.Contains(t, string(body), `"title":"a"`)
}

func TestServer_Fetch_OK(t *testing.T) {
	ts := newTestServer(&fakeSource{content: &DocContent{Title: "a", URL: "u", Text: "body"}})
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/fetch", "application/json", strings.NewReader(`{"url":"u"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), `"text":"body"`)
}

func TestServer_Fetch_BadRequest(t *testing.T) {
	ts := newTestServer(&fakeSource{})
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/fetch", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestServer_Fetch_GraphErrorPassthrough(t *testing.T) {
	ts := newTestServer(&fakeSource{err: &GraphError{StatusCode: http.StatusForbidden, Body: "denied"}})
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/fetch", "application/json", strings.NewReader(`{"url":"u"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode) // governance beat
}
