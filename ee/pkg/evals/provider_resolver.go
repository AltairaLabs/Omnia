/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AltairaLabs/PromptKit/runtime/credentials"
	"github.com/AltairaLabs/PromptKit/runtime/providers"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	pkgprovider "github.com/altairalabs/omnia/pkg/provider"
)

const providerCacheTTL = 5 * time.Minute

// ProviderResolver resolves named providers from an AgentRuntime CRD into
// PromptKit ProviderSpec values. Results are cached with a short TTL.
type ProviderResolver struct {
	client client.Client
	mu     sync.Mutex
	cache  map[string]*providerCacheEntry
}

type providerCacheEntry struct {
	specs   map[string]providers.ProviderSpec
	expires time.Time
}

// NewProviderResolver creates a new ProviderResolver.
func NewProviderResolver(c client.Client) *ProviderResolver {
	return &ProviderResolver{
		client: c,
		cache:  make(map[string]*providerCacheEntry),
	}
}

// ResolveProviderSpecs resolves all named providers from an AgentRuntime CRD
// into a map of PromptKit ProviderSpec keyed by provider name.
func (r *ProviderResolver) ResolveProviderSpecs(
	ctx context.Context, agentName, namespace string,
) (map[string]providers.ProviderSpec, error) {
	cacheKey := namespace + "/" + agentName

	if specs := r.getCached(cacheKey); specs != nil {
		return specs, nil
	}

	specs, err := r.resolve(ctx, agentName, namespace)
	if err != nil {
		return nil, err
	}

	r.putCache(cacheKey, specs)
	return specs, nil
}

func (r *ProviderResolver) getCached(key string) map[string]providers.ProviderSpec {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.cache[key]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.specs
}

func (r *ProviderResolver) putCache(key string, specs map[string]providers.ProviderSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[key] = &providerCacheEntry{
		specs:   specs,
		expires: time.Now().Add(providerCacheTTL),
	}
}

func (r *ProviderResolver) resolve(
	ctx context.Context, agentName, namespace string,
) (map[string]providers.ProviderSpec, error) {
	ar, err := k8s.GetAgentRuntime(ctx, r.client, agentName, namespace)
	if err != nil {
		return nil, fmt.Errorf("get AgentRuntime: %w", err)
	}

	if len(ar.Spec.Providers) == 0 {
		return nil, nil
	}

	specs := make(map[string]providers.ProviderSpec, len(ar.Spec.Providers))
	for _, np := range ar.Spec.Providers {
		spec, err := r.resolveOne(ctx, np, namespace)
		if err != nil {
			return nil, fmt.Errorf("resolve provider %q: %w", np.Name, err)
		}
		specs[np.Name] = spec
	}

	return specs, nil
}

func (r *ProviderResolver) resolveOne(
	ctx context.Context, np v1alpha1.NamedProviderRef, namespace string,
) (providers.ProviderSpec, error) {
	provider, err := k8s.GetProvider(ctx, r.client, np.ProviderRef, namespace)
	if err != nil {
		return providers.ProviderSpec{}, fmt.Errorf("get Provider CRD: %w", err)
	}

	spec := providers.ProviderSpec{
		ID:      np.Name,
		Type:    string(provider.Spec.Type),
		Model:   provider.Spec.Model,
		BaseURL: provider.Spec.BaseURL,
	}

	if provider.Spec.Defaults != nil {
		spec.Defaults = convertDefaults(provider.Spec.Defaults)
	}

	cred, err := r.resolveCredential(ctx, provider)
	if err != nil {
		return providers.ProviderSpec{}, fmt.Errorf("resolve credential: %w", err)
	}
	spec.Credential = cred

	return spec, nil
}

func (r *ProviderResolver) resolveCredential(
	ctx context.Context, provider *v1alpha1.Provider,
) (providers.Credential, error) {
	ref := k8s.EffectiveSecretRef(provider)
	if ref == nil {
		// No credential configured (e.g., mock, ollama)
		return nil, nil //nolint:nilnil // nil credential means env-var fallback
	}

	secret, err := k8s.GetSecret(ctx, r.client, ref.Name, provider.Namespace)
	if err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}

	secretKey := k8s.DetermineSecretKey(ref, provider.Spec.Type)
	apiKeyBytes, ok := secret.Data[secretKey]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s missing key %q", provider.Namespace, ref.Name, secretKey)
	}

	apiKey := string(apiKeyBytes)
	if !pkgprovider.Type(provider.Spec.Type).RequiresCredentials() {
		return nil, nil //nolint:nilnil // provider doesn't use credentials
	}

	return buildCredential(apiKey, string(provider.Spec.Type)), nil
}

// buildCredential creates a PromptKit credential with provider-appropriate
// header configuration. This mirrors PromptKit's credentials.createAPIKeyCredential.
func buildCredential(apiKey, providerType string) providers.Credential {
	headerCfg, ok := credentials.ProviderHeaderConfig[providerType]
	if !ok {
		return credentials.NewAPIKeyCredential(apiKey)
	}

	opts := []credentials.APIKeyOption{credentials.WithHeaderName(headerCfg.HeaderName)}
	if headerCfg.Prefix != "" {
		opts = append(opts, credentials.WithPrefix(headerCfg.Prefix))
	} else {
		opts = append(opts, credentials.WithPrefix(""))
	}

	return credentials.NewAPIKeyCredential(apiKey, opts...)
}

// convertDefaults maps Omnia ProviderDefaults to PromptKit ProviderDefaults.
func convertDefaults(d *v1alpha1.ProviderDefaults) providers.ProviderDefaults {
	pd := providers.ProviderDefaults{}

	if d.Temperature != nil {
		if v, err := strconv.ParseFloat(*d.Temperature, 32); err == nil {
			pd.Temperature = float32(v)
		}
	}
	if d.TopP != nil {
		if v, err := strconv.ParseFloat(*d.TopP, 32); err == nil {
			pd.TopP = float32(v)
		}
	}
	if d.MaxTokens != nil {
		pd.MaxTokens = int(*d.MaxTokens)
	}

	return pd
}
