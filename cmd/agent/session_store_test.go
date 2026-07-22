package main

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/agent"
	omniak8s "github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

type fakeResolver struct {
	urls *servicediscovery.ServiceURLs
	err  error

	gotWorkspace string
}

func (f *fakeResolver) ResolveServiceURLs(
	_ context.Context, workspaceName, _ string,
) (*servicediscovery.ServiceURLs, error) {
	f.gotWorkspace = workspaceName
	return f.urls, f.err
}

func TestSessionStoreFromResolver_HTTPClientOnSuccess(t *testing.T) {
	t.Setenv(omniak8s.EnvWorkspaceName, "demo")

	resolver := &fakeResolver{urls: &servicediscovery.ServiceURLs{SessionURL: "http://session-api:8080"}}
	store, mode, err := sessionStoreFromResolver(context.Background(), resolver, logr.Discard())

	require.NoError(t, err)
	assert.Equal(t, agent.SessionStoreModeHTTPClient, mode)
	require.NotNil(t, store)
	// The workspace NAME is passed through, not the namespace it owns (#1875).
	assert.Equal(t, "demo", resolver.gotWorkspace)
}

// Without the operator-injected workspace name the facade cannot scope its
// Workspace read, and must fail the same loud, non-fatal way as a discovery
// error rather than guessing a name from its namespace (#1875).
func TestSessionStoreFromResolver_NoArchiveWhenWorkspaceNameMissing(t *testing.T) {
	t.Setenv(omniak8s.EnvWorkspaceName, "")

	store, mode, err := sessionStoreFromResolver(
		context.Background(),
		&fakeResolver{urls: &servicediscovery.ServiceURLs{SessionURL: "http://session-api:8080"}},
		logr.Discard(),
	)

	require.NoError(t, err)
	assert.Equal(t, agent.SessionStoreModeNone, mode)
	require.Nil(t, store)
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
