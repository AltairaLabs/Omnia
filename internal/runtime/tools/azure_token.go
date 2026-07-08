package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// TokenAcquirer acquires a bearer token for an audience under the pod's ambient
// cloud identity (the same identity the LLM provider uses under workloadIdentity).
// Faked in tests.
type TokenAcquirer interface {
	Token(ctx context.Context, audience string) (string, error)
}

type cachedToken struct {
	value  string
	expiry time.Time
}

type azureTokenAcquirer struct {
	cred  azcore.TokenCredential
	mu    sync.Mutex
	cache map[string]cachedToken
}

// newDefaultAzureCredential is a seam over azidentity.NewDefaultAzureCredential
// so the constructor's success and failure paths are unit-testable. It returns
// the azcore.TokenCredential interface (rather than the concrete type) so tests
// can stub it with a fake credential.
var newDefaultAzureCredential = func(opts *azidentity.DefaultAzureCredentialOptions) (azcore.TokenCredential, error) {
	return azidentity.NewDefaultAzureCredential(opts)
}

func newAzureTokenAcquirer() (*azureTokenAcquirer, error) {
	cred, err := newDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure default credential: %w", err)
	}
	return &azureTokenAcquirer{cred: cred, cache: map[string]cachedToken{}}, nil
}

// Token returns a cached token for the audience until 5 minutes before expiry.
func (a *azureTokenAcquirer) Token(ctx context.Context, audience string) (string, error) {
	// The mutex deliberately wraps the credential fetch, not just the cache read:
	// the per-audience cache means contention is low, so the simpler single-lock
	// path is preferred over a double-checked lock.
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.cache[audience]; ok && time.Now().Before(c.expiry.Add(-5*time.Minute)) {
		return c.value, nil
	}
	at, err := a.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{audience + "/.default"}})
	if err != nil {
		return "", fmt.Errorf("acquire azure token for %q: %w", audience, err)
	}
	a.cache[audience] = cachedToken{value: at.Token, expiry: at.ExpiresOn}
	return at.Token, nil
}
