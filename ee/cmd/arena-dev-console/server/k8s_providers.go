/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.

Package server provides the WebSocket server for the Arena Dev Console.
This file implements Kubernetes provider loading for dynamic provider resolution.
*/

package server

import (
	"context"
	"fmt"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/go-logr/logr"
)

// K8sProviderLoader loads Provider CRDs from Kubernetes and converts them to PromptKit config.
// The loader only accesses providers in its own namespace (determined by POD_NAMESPACE env var)
// for security isolation.
type K8sProviderLoader struct {
	client    client.Client
	log       logr.Logger
	namespace string // The namespace this dev console is deployed in
}

// NewK8sProviderLoader creates a new Kubernetes provider loader.
// The loader uses POD_NAMESPACE to determine which namespace to access.
func NewK8sProviderLoader(log logr.Logger) (*K8sProviderLoader, error) {
	// Get namespace from environment (set by Kubernetes downward API)
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable not set")
	}

	// Create a runtime scheme with our types
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core v1alpha1 to scheme: %w", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core v1 to scheme: %w", err)
	}

	// Create in-cluster client
	// This uses the ServiceAccount credentials mounted in the pod
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	log.Info("K8s provider loader initialized", "namespace", namespace)

	return &K8sProviderLoader{
		client:    c,
		log:       log.WithName("k8s-provider-loader"),
		namespace: namespace,
	}, nil
}

// Namespace returns the namespace this loader is configured for.
func (l *K8sProviderLoader) Namespace() string {
	return l.namespace
}

// LoadProviders loads all Provider CRDs from this dev console's namespace
// and converts them to PromptKit config format.
func (l *K8sProviderLoader) LoadProviders(ctx context.Context) (map[string]*config.Provider, error) {
	return l.LoadProvidersForNamespace(ctx, l.namespace)
}

// LoadProvidersForNamespace loads all Provider CRDs from the specified namespace
// and converts them to PromptKit config format.
// NOTE: The loader can only access its own namespace due to RBAC restrictions.
func (l *K8sProviderLoader) LoadProvidersForNamespace(
	ctx context.Context,
	namespace string,
) (map[string]*config.Provider, error) {
	if namespace == "" {
		namespace = l.namespace
	}

	// Security check: only allow access to own namespace
	if namespace != l.namespace {
		return nil, fmt.Errorf("cannot access providers in namespace %s (only %s is allowed)", namespace, l.namespace)
	}

	// List all providers in the namespace
	providerList := &corev1alpha1.ProviderList{}
	if err := l.client.List(ctx, providerList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list providers in namespace %s: %w", namespace, err)
	}

	l.log.Info("found providers", "namespace", namespace, "count", len(providerList.Items))

	providers := make(map[string]*config.Provider)
	for i := range providerList.Items {
		p := &providerList.Items[i]

		// Skip providers that aren't ready
		if p.Status.Phase != corev1alpha1.ProviderPhaseReady {
			l.log.V(1).Info("skipping provider not ready", "name", p.Name, "phase", p.Status.Phase)
			continue
		}

		// Convert to PromptKit config
		pkProvider, err := l.convertProvider(ctx, p)
		if err != nil {
			l.log.Error(err, "failed to convert provider", "name", p.Name)
			continue
		}

		providers[p.Name] = pkProvider
		l.log.V(1).Info("loaded provider", "name", p.Name, "type", p.Spec.Type)
	}

	return providers, nil
}

// convertProvider converts a Provider CRD to PromptKit config.Provider.
func (l *K8sProviderLoader) convertProvider(ctx context.Context, p *corev1alpha1.Provider) (*config.Provider, error) {
	provider := &config.Provider{
		ID:      p.Name,
		Type:    string(p.Spec.Type),
		Model:   p.Spec.Model,
		BaseURL: p.Spec.BaseURL,
	}

	// Set defaults if specified
	if p.Spec.Defaults != nil {
		if p.Spec.Defaults.Temperature != nil {
			if temp, err := strconv.ParseFloat(*p.Spec.Defaults.Temperature, 32); err == nil {
				provider.Defaults.Temperature = float32(temp)
			}
		}
		if p.Spec.Defaults.MaxTokens != nil {
			provider.Defaults.MaxTokens = int(*p.Spec.Defaults.MaxTokens)
		}
	}

	// Resolve credential from secret
	if p.Spec.SecretRef != nil {
		apiKey, err := l.resolveSecret(ctx, p.Namespace, p.Spec.SecretRef)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve credential: %w", err)
		}
		// Create a unique env var name and set it in the process environment
		// This is a workaround since PromptKit expects credentials in env vars
		envVarName := fmt.Sprintf("PROVIDER_%s_API_KEY", p.Name)
		if err := os.Setenv(envVarName, apiKey); err != nil {
			return nil, fmt.Errorf("failed to set env var for credential: %w", err)
		}
		provider.Credential = &config.CredentialConfig{
			CredentialEnv: envVarName,
		}
	}

	return provider, nil
}

// resolveSecret reads the API key from a Kubernetes secret.
func (l *K8sProviderLoader) resolveSecret(
	ctx context.Context,
	namespace string,
	secretRef *corev1alpha1.SecretKeyRef,
) (string, error) {
	secret := &corev1.Secret{}
	err := l.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      secretRef.Name,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	// Use the specified key, or fall back to "api-key"
	key := "api-key"
	if secretRef.Key != nil && *secretRef.Key != "" {
		key = *secretRef.Key
	}

	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s", key, secretRef.Name)
	}

	return string(value), nil
}

// BuildConfigFromProviders creates a PromptKit config.Config with the given providers.
func BuildConfigFromProviders(providers map[string]*config.Provider) *config.Config {
	return &config.Config{
		LoadedProviders: providers,
	}
}
