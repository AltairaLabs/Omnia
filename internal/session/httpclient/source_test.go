package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/internal/session"
)

const testSourceSessionID = "11111111-1111-4111-8111-111111111111"

// WithSource sets X-Omnia-Source on writes so the privacy middleware can gate
// runtime content separately from facade content.
func TestStore_WithSource_SetsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(session.SourceHeader)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithSource(session.SourceRuntime), WithBufferCapacity(0))
	err := store.RecordProviderCall(context.Background(), testSourceSessionID,
		session.ProviderCall{Provider: "claude", Model: "m"})
	assert.NoError(t, err)
	assert.Equal(t, session.SourceRuntime, got)
}

// Without WithSource the header is omitted (e.g. the doctor tool).
func TestStore_NoSource_OmitsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(session.SourceHeader)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	err := store.RecordProviderCall(context.Background(), testSourceSessionID,
		session.ProviderCall{Provider: "claude", Model: "m"})
	assert.NoError(t, err)
	assert.Empty(t, got)
}
