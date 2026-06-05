package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docxBody builds a minimal .docx zip whose word/document.xml contains text.
func docxBody(t *testing.T, text string) []byte {
	t.Helper()
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`
	return makeZip(t, map[string]string{"word/document.xml": docXML})
}

func staticToken(_ context.Context) (string, error) { return "test-token", nil }

func errToken(_ context.Context) (string, error) { return "", errors.New("token boom") }

func TestNewGraphClient_Defaults(t *testing.T) {
	// Empty baseURL + nil http client exercise the default-assignment branches.
	g := NewGraphClient("", "site", staticToken, nil)
	require.NotNil(t, g)
	assert.Equal(t, defaultGraphBaseURL, g.baseURL)
	assert.NotNil(t, g.http)
}

func TestGraphClient_List_TokenError(t *testing.T) {
	g := NewGraphClient("http://example.invalid", "s", errToken, http.DefaultClient)
	_, err := g.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acquire token")
}

func TestGraphClient_Fetch_ContentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"name":"p.txt","webUrl":"https://c/p.txt"}`))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "s", staticToken, srv.Client())
	_, err := g.Fetch(context.Background(), "https://c/p.txt")

	var ge *GraphError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, http.StatusInternalServerError, ge.StatusCode)
}

func TestEncodeShareID(t *testing.T) {
	got := encodeShareID("https://contoso.sharepoint.com/sites/demo/Shared Documents/policy.txt")
	assert.Equal(t, "u!", got[:2])
	assert.NotContains(t, got, "=")
	assert.NotContains(t, got, "/")
	assert.NotContains(t, got, "+")
}

func TestGraphError_Error(t *testing.T) {
	err := &GraphError{StatusCode: 403, Body: "forbidden"}
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "forbidden")
}

func TestGraphClient_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Contains(t, r.URL.Path, "/sites/site-1/drive/root/children")
		_, _ = w.Write([]byte(`{"value":[
			{"name":"policy.txt","webUrl":"https://c/p.txt","file":{"mimeType":"text/plain"}},
			{"name":"Folder"}
		]}`))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "site-1", staticToken, srv.Client())
	docs, err := g.List(context.Background())

	assert.NoError(t, err)
	assert.Len(t, docs, 1) // folder (no "file") skipped
	assert.Equal(t, "policy.txt", docs[0].Title)
	assert.Equal(t, "https://c/p.txt", docs[0].URL)
	assert.Equal(t, "site-1", docs[0].Site)
}

func TestGraphClient_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = w.Write([]byte("the document body"))
			return
		}
		_, _ = w.Write([]byte(`{"name":"policy.txt","webUrl":"https://c/p.txt"}`))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "site-1", staticToken, srv.Client())
	doc, err := g.Fetch(context.Background(), "https://c/p.txt")

	assert.NoError(t, err)
	assert.Equal(t, "policy.txt", doc.Title)
	assert.Equal(t, "the document body", doc.Text)
}

func TestGraphClient_List_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Access denied"))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "site-1", staticToken, srv.Client())
	_, err := g.List(context.Background())

	var ge *GraphError
	assert.ErrorAs(t, err, &ge)
	assert.Equal(t, http.StatusForbidden, ge.StatusCode)
}

// TestGraphClient_Fetch_DocxExtraction verifies that Fetch converts a .docx
// response to extracted plain text rather than returning raw zip bytes.
func TestGraphClient_Fetch_DocxExtraction(t *testing.T) {
	zipBytes := docxBody(t, "Hello World")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = w.Write(zipBytes)
			return
		}
		_, _ = w.Write([]byte(`{"name":"runbook.docx","webUrl":"https://c/runbook.docx"}`))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "site-1", staticToken, srv.Client())
	doc, err := g.Fetch(context.Background(), "https://c/runbook.docx")

	require.NoError(t, err)
	assert.Equal(t, "runbook.docx", doc.Title)
	assert.Contains(t, doc.Text, "Hello World")
	assert.NotContains(t, doc.Text, "PK") // zip magic bytes must not appear in extracted text
}

// TestGraphClient_Fetch_ExtractionError verifies that Fetch surfaces an error
// when the fetched bytes are not a valid OOXML zip (corrupt .docx).
func TestGraphClient_Fetch_ExtractionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = w.Write([]byte("not a zip"))
			return
		}
		_, _ = w.Write([]byte(`{"name":"bad.docx","webUrl":"https://c/bad.docx"}`))
	}))
	defer srv.Close()

	g := NewGraphClient(srv.URL, "site-1", staticToken, srv.Client())
	_, err := g.Fetch(context.Background(), "https://c/bad.docx")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract")
	assert.Contains(t, err.Error(), "bad.docx")
}
