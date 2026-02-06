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
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/go-logr/logr"
)

const (
	// devConsoleOutputDir is the output directory for the dev console.
	devConsoleOutputDir = "/tmp/arena-dev-console-output"
	// devConsoleConfigDir is the config directory for the dev console.
	devConsoleConfigDir = "/tmp/arena-dev-console"
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

	providerMap := make(map[string]*config.Provider)
	for i := range providerList.Items {
		p := &providerList.Items[i]

		// Skip providers that aren't ready
		if p.Status.Phase != corev1alpha1.ProviderPhaseReady {
			l.log.V(1).Info("skipping provider not ready", "name", p.Name, "phase", p.Status.Phase)
			continue
		}

		// Convert to PromptKit config
		pkProvider := l.convertProvider(p)
		providerMap[p.Name] = pkProvider
		l.log.V(1).Info("loaded provider", "name", p.Name, "type", p.Spec.Type)
	}

	return providerMap, nil
}

// convertProvider converts a Provider CRD to PromptKit config.Provider.
// Credentials are expected to be mounted as environment variables by the
// ArenaDevSession controller (using the same logic as ArenaJob controller).
func (l *K8sProviderLoader) convertProvider(p *corev1alpha1.Provider) *config.Provider {
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

	// Get the expected env var name for this provider type's credentials
	// The ArenaDevSession controller mounts secrets as env vars using the same
	// logic as the ArenaJob controller (via providers.BuildEnvVarsFromProviders)
	envVarNames := providers.GetAPIKeyEnvVars(string(p.Spec.Type))
	if len(envVarNames) > 0 {
		// Use the primary env var for this provider type
		envVarName := envVarNames[0]
		// Check if the env var is set (controller should have mounted it)
		if os.Getenv(envVarName) != "" {
			provider.Credential = &config.CredentialConfig{
				CredentialEnv: envVarName,
			}
			l.log.V(1).Info("using credential from env var", "provider", p.Name, "envVar", envVarName)
		} else {
			l.log.V(1).Info("credential env var not set", "provider", p.Name, "envVar", envVarName)
		}
	}

	return provider
}

// BuildConfigFromProviders creates a PromptKit config.Config with the given providers.
// Sets the output directory to a writable location since the container's working
// directory may be read-only.
func BuildConfigFromProviders(providerMap map[string]*config.Provider) *config.Config {
	return &config.Config{
		LoadedProviders: providerMap,
		// Set ConfigDir to a writable location for any file operations
		ConfigDir: devConsoleConfigDir,
		Defaults: config.Defaults{
			// Set both Output.Dir and the deprecated OutDir for compatibility
			Output: config.OutputConfig{
				Dir: devConsoleOutputDir,
			},
			OutDir:    devConsoleOutputDir,
			ConfigDir: devConsoleConfigDir,
		},
	}
}
