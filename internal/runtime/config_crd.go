/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	pkgprovider "github.com/altairalabs/omnia/pkg/provider"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

// LoadFromCRD loads runtime configuration by reading the AgentRuntime CRD directly.
// It resolves the provider from the CRD, reads the API key secret, and sets the
// corresponding environment variable for the PromptKit SDK.
func LoadFromCRD(ctx context.Context, c client.Client, name, namespace string) (*Config, error) {
	ar, err := k8s.GetAgentRuntime(ctx, c, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("load AgentRuntime CRD: %w", err)
	}

	workspaceName, err := k8s.ResolveWorkspaceName(ctx, c, ar.Labels, namespace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace name: %w", err)
	}

	cfg := &Config{
		AgentName:      name,
		AgentUID:       string(ar.UID),
		Namespace:      namespace,
		WorkspaceName:  workspaceName,
		PromptPackPath: getEnvOrDefault(envPromptPackPath, defaultPromptPackPath),
		PromptName:     getEnvOrDefault(envPromptName, defaultPromptName),
		GRPCPort:       defaultGRPCPort,
		HealthPort:     defaultHealthPort,
		SessionTTL:     defaultSessionTTL,
		MediaBasePath:  defaultMediaBasePath,
	}

	// PromptPack info from CRD
	cfg.PromptPackName = ar.Spec.PromptPackRef.Name
	cfg.PromptPackNamespace = namespace
	if ar.Spec.PromptPackRef.Version != nil {
		cfg.PromptPackVersion = *ar.Spec.PromptPackRef.Version
	}

	// Session config from CRD
	if err := loadRuntimeSessionFromCRD(cfg, ar); err != nil {
		return nil, err
	}

	// Media config from CRD
	if ar.Spec.Media != nil && ar.Spec.Media.BasePath != "" {
		cfg.MediaBasePath = ar.Spec.Media.BasePath
	}

	// Eval config from CRD
	cfg.EvalEnabled = ar.Spec.Evals != nil && ar.Spec.Evals.Enabled
	if ar.Spec.Evals != nil && ar.Spec.Evals.Inline != nil {
		cfg.InlineEvalGroups = ar.Spec.Evals.Inline.Groups
	}

	// Provider resolution: providers map → providerRef → inline provider
	if err := loadProviderFromCRD(ctx, c, cfg, ar, namespace); err != nil {
		return nil, err
	}

	// Mock provider annotation (dev/test mode)
	if mock, ok := ar.Annotations["omnia.altairalabs.ai/mock-provider"]; ok && mock == "true" {
		cfg.MockProvider = true
	}
	if mockCfg, ok := ar.Annotations["omnia.altairalabs.ai/mock-config-path"]; ok && mockCfg != "" {
		cfg.MockConfigPath = mockCfg
	}

	// Auto-enable mock provider when provider type is "mock"
	if cfg.ProviderType == string(pkgprovider.TypeMock) {
		cfg.MockProvider = true
	}

	// Tools config: if the CRD has a toolRegistryRef, the operator mounts the
	// tools ConfigMap at a well-known path. Derive it from the CRD rather than
	// relying on an env var.
	if ar.Spec.ToolRegistryRef != nil {
		cfg.ToolsConfigPath = defaultToolsMountPath + "/" + defaultToolsConfigFile
		cfg.ToolRegistryName = ar.Spec.ToolRegistryRef.Name
		if ar.Spec.ToolRegistryRef.Namespace != nil {
			cfg.ToolRegistryNamespace = *ar.Spec.ToolRegistryRef.Namespace
		} else {
			cfg.ToolRegistryNamespace = namespace
		}
	}

	// Service URLs from Workspace CRD status (in-cluster) or env vars (local dev).
	resolver := servicediscovery.NewResolver(c)
	serviceGroup := ar.Spec.ServiceGroup
	if serviceGroup == "" {
		serviceGroup = "default"
	}
	urls, urlErr := resolver.ResolveServiceURLs(ctx, serviceGroup)
	if urlErr != nil {
		log := logf.FromContext(ctx)
		log.Error(urlErr, "service URL resolution failed, falling back to env vars",
			"serviceGroup", serviceGroup)
	} else {
		cfg.SessionAPIURL = urls.SessionURL
		cfg.MemoryAPIURL = urls.MemoryURL
	}

	// Memory config from CRD
	if ar.Spec.Memory != nil && ar.Spec.Memory.Enabled {
		cfg.MemoryEnabled = true
		uid, uidErr := resolveWorkspaceUID(ctx, c, namespace)
		if uidErr != nil {
			return nil, fmt.Errorf("resolve workspace UID for memory: %w", uidErr)
		}
		cfg.WorkspaceUID = uid
	}

	// Tracing config from env (injected by operator from Helm values)
	cfg.TracingEnabled = os.Getenv(envTracingEnabled) == "true"
	cfg.TracingEndpoint = os.Getenv(envTracingEndpoint)
	cfg.TracingInsecure = os.Getenv(envTracingInsecure) == "true"
	cfg.TracingSampleRate = 1.0

	// Parse env-only overrides (ports, tracing sample rate, etc.)
	if err := cfg.parseEnvironmentOverrides(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// resolveWorkspaceUID finds the Workspace CRD whose spec.namespace.name matches
// the given namespace and returns its Kubernetes UID. The memory_entities table
// uses workspace_id as UUID, which corresponds to the Workspace CR's UID.
func resolveWorkspaceUID(ctx context.Context, c client.Client, namespace string) (string, error) {
	var list v1alpha1.WorkspaceList
	if err := c.List(ctx, &list); err != nil {
		return "", fmt.Errorf("list workspaces: %w", err)
	}
	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			return string(ws.UID), nil
		}
	}
	return "", nil
}

// loadRuntimeSessionFromCRD populates session config from the AgentRuntime CRD.
func loadRuntimeSessionFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	if ar.Spec.Session == nil {
		cfg.SessionType = defaultSessionType
		return nil
	}

	cfg.SessionType = string(ar.Spec.Session.Type)

	if ar.Spec.Session.TTL != nil {
		ttl, err := time.ParseDuration(*ar.Spec.Session.TTL)
		if err != nil {
			return fmt.Errorf("parse session TTL %q: %w", *ar.Spec.Session.TTL, err)
		}
		cfg.SessionTTL = ttl
	}

	// Session store URL still comes from env (secret-backed)
	cfg.SessionURL = os.Getenv(envSessionURL)
	return nil
}

// loadProviderFromCRD resolves the provider from the AgentRuntime CRD and sets
// the API key environment variable for the PromptKit SDK.
func loadProviderFromCRD(ctx context.Context, c client.Client, cfg *Config, ar *v1alpha1.AgentRuntime, namespace string) error {
	if len(ar.Spec.Providers) > 0 {
		return loadFromNamedProviders(ctx, c, cfg, ar.Spec.Providers, namespace)
	}

	return nil
}

// loadFromNamedProviders resolves the "default" (or first sorted) named provider.
func loadFromNamedProviders(ctx context.Context, c client.Client, cfg *Config, providers []v1alpha1.NamedProviderRef, namespace string) error {
	// Find "default" entry, or use first sorted
	var ref v1alpha1.ProviderRef
	found := false
	for _, np := range providers {
		if np.Name == "default" {
			ref = np.ProviderRef
			found = true
			break
		}
	}
	if !found {
		// Sort by name and use first
		sorted := make([]v1alpha1.NamedProviderRef, len(providers))
		copy(sorted, providers)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
		ref = sorted[0].ProviderRef
	}

	return loadFromProviderRef(ctx, c, cfg, ref, namespace)
}

// loadFromProviderRef loads config from a Provider CRD reference and injects the API key.
func loadFromProviderRef(ctx context.Context, c client.Client, cfg *Config, ref v1alpha1.ProviderRef, namespace string) error {
	provider, err := k8s.GetProvider(ctx, c, ref, namespace)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	cfg.ProviderType = string(provider.Spec.Type)
	cfg.Model = provider.Spec.Model
	cfg.BaseURL = provider.Spec.BaseURL
	cfg.Headers = provider.Spec.Headers
	cfg.ProviderRefName = provider.Name
	cfg.ProviderRefNamespace = provider.Namespace

	loadPlatformConfig(cfg, provider.Spec.Platform)
	loadAuthConfig(cfg, provider.Spec.Auth)

	if provider.Spec.Defaults != nil {
		if err := loadProviderDefaults(cfg, provider.Spec.Defaults); err != nil {
			return err
		}
	}

	// Load pricing from Provider CRD
	if err := loadProviderPricing(cfg, provider.Spec.Pricing); err != nil {
		return err
	}

	// Inject API key from secret (for non-platform providers)
	if provider.Spec.Platform == nil {
		return injectAPIKey(ctx, c, provider)
	}

	// Inject platform credentials from secret (for static auth types)
	return injectPlatformCredentials(ctx, c, provider)
}

// loadPlatformConfig copies spec.platform into the runtime Config.
func loadPlatformConfig(cfg *Config, platform *v1alpha1.PlatformConfig) {
	if platform == nil {
		return
	}
	cfg.PlatformType = string(platform.Type)
	cfg.PlatformRegion = platform.Region
	cfg.PlatformProject = platform.Project
	cfg.PlatformEndpoint = platform.Endpoint
}

// loadAuthConfig copies spec.auth into the runtime Config.
func loadAuthConfig(cfg *Config, auth *v1alpha1.AuthConfig) {
	if auth == nil {
		return
	}
	cfg.AuthType = string(auth.Type)
	cfg.AuthRoleArn = auth.RoleArn
	cfg.AuthServiceAccountEmail = auth.ServiceAccountEmail
	if auth.CredentialsSecretRef != nil {
		cfg.AuthCredentialsSecretName = auth.CredentialsSecretRef.Name
		if auth.CredentialsSecretRef.Key != nil {
			cfg.AuthCredentialsSecretKey = *auth.CredentialsSecretRef.Key
		}
	}
}

// loadProviderDefaults populates config fields from Provider CRD defaults.
func loadProviderDefaults(cfg *Config, defaults *v1alpha1.ProviderDefaults) error {
	if defaults.ContextWindow != nil {
		cfg.ContextWindow = int(*defaults.ContextWindow)
	}
	if defaults.TruncationStrategy != "" {
		cfg.TruncationStrategy = string(defaults.TruncationStrategy)
	}
	if defaults.RequestTimeout != "" {
		d, err := time.ParseDuration(defaults.RequestTimeout)
		if err != nil {
			return fmt.Errorf("parse requestTimeout %q: %w", defaults.RequestTimeout, err)
		}
		if d < 0 {
			return fmt.Errorf("requestTimeout %q must be non-negative", defaults.RequestTimeout)
		}
		cfg.ProviderRequestTimeout = d
	}
	if defaults.StreamIdleTimeout != "" {
		d, err := time.ParseDuration(defaults.StreamIdleTimeout)
		if err != nil {
			return fmt.Errorf("parse streamIdleTimeout %q: %w", defaults.StreamIdleTimeout, err)
		}
		if d < 0 {
			return fmt.Errorf("streamIdleTimeout %q must be non-negative", defaults.StreamIdleTimeout)
		}
		cfg.ProviderStreamIdleTimeout = d
	}
	return nil
}

// loadProviderPricing extracts pricing from the Provider CRD and converts to float64.
func loadProviderPricing(cfg *Config, pricing *v1alpha1.ProviderPricing) error {
	if pricing == nil {
		return nil
	}
	if pricing.InputCostPer1K != nil {
		v, err := strconv.ParseFloat(*pricing.InputCostPer1K, 64)
		if err != nil {
			return fmt.Errorf("parse inputCostPer1K %q: %w", *pricing.InputCostPer1K, err)
		}
		cfg.InputCostPer1K = v
	}
	if pricing.OutputCostPer1K != nil {
		v, err := strconv.ParseFloat(*pricing.OutputCostPer1K, 64)
		if err != nil {
			return fmt.Errorf("parse outputCostPer1K %q: %w", *pricing.OutputCostPer1K, err)
		}
		cfg.OutputCostPer1K = v
	}
	return nil
}

// injectAPIKey reads the provider's secret and sets the appropriate env var
// for the PromptKit SDK (e.g., ANTHROPIC_API_KEY, OPENAI_API_KEY).
func injectAPIKey(ctx context.Context, c client.Client, provider *v1alpha1.Provider) error {
	ref := k8s.EffectiveSecretRef(provider)
	if ref == nil {
		return nil // No secret configured (e.g., ollama, mock)
	}

	secret, err := k8s.GetSecret(ctx, c, ref.Name, provider.Namespace)
	if err != nil {
		return fmt.Errorf("read provider secret: %w", err)
	}

	// Determine which key in the Secret to read
	secretKey := k8s.DetermineSecretKey(ref, provider.Spec.Type)
	apiKeyValue, ok := secret.Data[secretKey]
	if !ok {
		return fmt.Errorf("secret %s/%s does not contain key %q", provider.Namespace, ref.Name, secretKey)
	}

	// Set the env var the PromptKit SDK expects
	envVarName := pkgprovider.APIKeyEnvVarName(string(provider.Spec.Type))
	if envVarName == "" {
		return nil // Provider type doesn't use API key env vars
	}

	if err := os.Setenv(envVarName, string(apiKeyValue)); err != nil {
		return fmt.Errorf("set env var %s: %w", envVarName, err)
	}

	return nil
}

// injectPlatformCredentials reads a platform auth secret (when static) and
// sets the corresponding cloud SDK environment variables so PromptKit's
// default credential chain resolves them. workloadIdentity is a no-op — the
// pod's federated identity is picked up by the cloud SDK automatically.
//
// Expected secret shape by auth type:
//
//	accessKey        (bedrock): AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN (optional)
//	serviceAccount   (vertex):  a single key containing the GCP SA JSON
//	servicePrincipal (azure):   AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET
func injectPlatformCredentials(ctx context.Context, c client.Client, provider *v1alpha1.Provider) error {
	auth := provider.Spec.Auth
	if auth == nil || auth.Type == v1alpha1.AuthMethodWorkloadIdentity {
		return nil
	}
	if auth.CredentialsSecretRef == nil {
		return fmt.Errorf("auth type %q requires credentialsSecretRef", auth.Type)
	}

	secret, err := k8s.GetSecret(ctx, c, auth.CredentialsSecretRef.Name, provider.Namespace)
	if err != nil {
		return fmt.Errorf("read platform credentials secret: %w", err)
	}

	platform := provider.Spec.Platform
	switch {
	case platform.Type == v1alpha1.PlatformTypeBedrock && auth.Type == v1alpha1.AuthMethodAccessKey:
		return injectAWSAccessKey(secret.Data, provider.Namespace, auth.CredentialsSecretRef.Name)
	case platform.Type == v1alpha1.PlatformTypeVertex && auth.Type == v1alpha1.AuthMethodServiceAccount:
		return injectGCPServiceAccount(secret.Data, auth.CredentialsSecretRef)
	case platform.Type == v1alpha1.PlatformTypeAzure && auth.Type == v1alpha1.AuthMethodServicePrincipal:
		return injectAzureServicePrincipal(secret.Data, provider.Namespace, auth.CredentialsSecretRef.Name)
	default:
		return fmt.Errorf("unsupported platform/auth combination: %s/%s", platform.Type, auth.Type)
	}
}

// injectAWSAccessKey sets AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY (and
// optionally AWS_SESSION_TOKEN) from the secret.
func injectAWSAccessKey(data map[string][]byte, namespace, name string) error {
	accessKey, ok := data["AWS_ACCESS_KEY_ID"]
	if !ok {
		return fmt.Errorf("secret %s/%s missing AWS_ACCESS_KEY_ID", namespace, name)
	}
	secretKey, ok := data["AWS_SECRET_ACCESS_KEY"]
	if !ok {
		return fmt.Errorf("secret %s/%s missing AWS_SECRET_ACCESS_KEY", namespace, name)
	}
	if err := os.Setenv("AWS_ACCESS_KEY_ID", string(accessKey)); err != nil {
		return fmt.Errorf("set AWS_ACCESS_KEY_ID: %w", err)
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", string(secretKey)); err != nil {
		return fmt.Errorf("set AWS_SECRET_ACCESS_KEY: %w", err)
	}
	if token, ok := data["AWS_SESSION_TOKEN"]; ok {
		if err := os.Setenv("AWS_SESSION_TOKEN", string(token)); err != nil {
			return fmt.Errorf("set AWS_SESSION_TOKEN: %w", err)
		}
	}
	return nil
}

// injectGCPServiceAccount writes the SA JSON to a file and sets
// GOOGLE_APPLICATION_CREDENTIALS. The secret key defaults to
// "credentials.json"; override with spec.auth.credentialsSecretRef.key.
func injectGCPServiceAccount(data map[string][]byte, ref *v1alpha1.SecretKeyRef) error {
	key := "credentials.json"
	if ref.Key != nil && *ref.Key != "" {
		key = *ref.Key
	}
	jsonBytes, ok := data[key]
	if !ok {
		return fmt.Errorf("secret %s missing key %q", ref.Name, key)
	}
	// CreateTemp generates a unique path inside os.TempDir() with 0600 perms,
	// avoiding the predictable-path risk flagged by go:S5443.
	f, err := os.CreateTemp("", "gcp-sa-*.json")
	if err != nil {
		return fmt.Errorf("create GCP SA temp file: %w", err)
	}
	if _, err := f.Write(jsonBytes); err != nil {
		_ = f.Close()
		return fmt.Errorf("write GCP SA key to %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close GCP SA key file %s: %w", f.Name(), err)
	}
	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f.Name()); err != nil {
		return fmt.Errorf("set GOOGLE_APPLICATION_CREDENTIALS: %w", err)
	}
	return nil
}

// injectAzureServicePrincipal sets AZURE_TENANT_ID / AZURE_CLIENT_ID /
// AZURE_CLIENT_SECRET so the Azure EnvironmentCredential picks them up.
func injectAzureServicePrincipal(data map[string][]byte, namespace, name string) error {
	for _, key := range []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"} {
		value, ok := data[key]
		if !ok {
			return fmt.Errorf("secret %s/%s missing %s", namespace, name, key)
		}
		if err := os.Setenv(key, string(value)); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}
