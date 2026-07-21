package main

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

type fakeResolver struct {
	urls *servicediscovery.ServiceURLs
	err  error
}

func (f *fakeResolver) ResolveServiceURLs(context.Context, string) (*servicediscovery.ServiceURLs, error) {
	return f.urls, f.err
}

func TestSessionStoreFromResolver_HTTPClientOnSuccess(t *testing.T) {
	store, mode, err := sessionStoreFromResolver(
		context.Background(),
		&fakeResolver{urls: &servicediscovery.ServiceURLs{SessionURL: "http://session-api:8080"}},
		logr.Discard(),
	)
	require.NoError(t, err)
	assert.Equal(t, agent.SessionStoreModeHTTPClient, mode)
	require.NotNil(t, store)
}

// TestSessionStoreFromResolver_MemoryFallbackOnDiscoveryFailure is the #1223
// guard: a service-discovery failure must surface the "memory" mode (which
// drives the omnia_agent_session_store metric and the loud error log) rather
// than silently falling back. The fallback itself stays non-fatal.
func TestSessionStoreFromResolver_NoArchiveOnDiscoveryFailure(t *testing.T) {
	store, mode, err := sessionStoreFromResolver(
		context.Background(),
		&fakeResolver{err: errors.New("forbidden: cannot list workspaces")},
		logr.Discard(),
	)
	require.NoError(t, err) // non-fatal: the agent still serves, just without recording
	assert.Equal(t, agent.SessionStoreModeNone, mode)
	// No store at all, rather than one that satisfies the interface and
	// discards every write: the facade treats nil as "no archive configured"
	// and serves from the context store (#1876).
	require.Nil(t, store)
}
