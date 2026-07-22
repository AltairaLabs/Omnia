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
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// Apply the canary override (candidate pods only): substitute the
	// candidate's provider refs onto the in-memory AgentRuntime so the live
	// provider resolution below runs against the candidate's providers, not the
	// shared stable spec (#1468). A no-op on stable / non-rollout pods.
	if err := applyCanaryOverrideFromMount(ar); err != nil {
		return nil, fmt.Errorf("apply canary override: %w", err)
	}

	// Prefer the operator-injected name, then the AgentRuntime's own label.
	// Both avoid reading the Namespace object; ResolveWorkspaceName's
	// namespace-label fallback stays for pods that predate the injection (#1875).
	workspaceName, err := k8s.WorkspaceNameFromEnvOrLabels(ar.Labels)
	if err != nil {
		workspaceName, err = k8s.ResolveWorkspaceName(ctx, c, ar.Labels, namespace)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace name: %w", err)
		}
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
		ContextTTL:     defaultContextTTL,
		MediaBasePath:  defaultMediaBasePath,
	}

	// PromptPack info from CRD
	cfg.PromptPackName = ar.Spec.PromptPackRef.Name
	cfg.PromptPackNamespace = namespace
	if ar.Spec.PromptPackRef.Version != nil {
		cfg.PromptPackVersion = *ar.Spec.PromptPackRef.Version
	}
	// track:-selected AgentRuntimes have no pinned version — fall back to the
	// operator-resolved version stamped into the pod env (#1847), so the
	// eval-path version stamp is always concrete.
	if cfg.PromptPackVersion == "" {
		if v := os.Getenv(envPromptPackVersion); v != "" {
			cfg.PromptPackVersion = v
		}
	}

	// Function response-format inputs (consumed by resolveResponseFormat, #1483).
	cfg.Mode = string(ar.Spec.Mode)
	cfg.OutputFormat = ar.Spec.OutputFormat
	if ar.Spec.OutputSchema != nil {
		cfg.OutputSchemaJSON = ar.Spec.OutputSchema.Raw
	}

	// Context store config from CRD
	if err := loadRuntimeContextFromCRD(cfg, ar); err != nil {
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
	// The workspace name comes from the operator, never inferred from the
	// namespace — they are different identifiers and conflating them is a
	// recurring bug class here (#1875). An unresolvable name is treated like any
	// other discovery failure: loud, but non-fatal, so a pod that predates the
	// operator injecting it still starts and falls back to env vars.
	urls, urlErr := resolver.ResolveServiceURLs(ctx, workspaceName, serviceGroup)
	if urlErr != nil {
		log := logf.FromContext(ctx)
		log.Error(urlErr, "service URL resolution failed, falling back to env vars",
			"serviceGroup", serviceGroup, "workspace", workspaceName)
	} else {
		cfg.SessionAPIURL = urls.SessionURL
		cfg.MemoryAPIURL = urls.MemoryURL
	}

	// Memory config from CRD
	if ar.Spec.Memory != nil && ar.Spec.Memory.Enabled {
		cfg.MemoryEnabled = true
		// Ambient RAG and the memory tools both default ON when memory is
		// enabled and the sub-toggle is unset, preserving existing behavior.
		cfg.MemoryRetrievalEnabled = true
		cfg.MemoryToolsEnabled = true
		// The operator injects OMNIA_WORKSPACE_UID when memory is enabled
		// (deployment_builder_env.go). Prefer it; otherwise read metadata.uid off
		// the agent's own Workspace. This used to be a second cluster-wide
		// WorkspaceList issued at startup purely to learn a UID (#1875).
		cfg.WorkspaceUID = os.Getenv(envWorkspaceUID)
		if cfg.WorkspaceUID == "" && workspaceName != "" {
			uid, uidErr := workspaceUID(ctx, resolver, workspaceName)
			if uidErr != nil {
				return nil, fmt.Errorf("resolve workspace UID for memory: %w", uidErr)
			}
			cfg.WorkspaceUID = uid
		}
		if r := ar.Spec.Memory.Retrieval; r != nil {
			if r.Enabled != nil {
				cfg.MemoryRetrievalEnabled = *r.Enabled
			}
			cfg.MemoryStrategy = r.Strategy
			if r.Limit != nil {
				cfg.MemoryLimit = int(*r.Limit)
			}
			if r.AccessFilter != nil {
				cfg.MemoryDenyCEL = r.AccessFilter.DenyCEL
			}
		}
		if t := ar.Spec.Memory.Tools; t != nil && t.Enabled != nil {
			cfg.MemoryToolsEnabled = *t.Enabled
		}
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

// workspaceUID returns the Kubernetes UID of the named Workspace. The
// memory_entities table uses workspace_id as UUID, which corresponds to the
// Workspace CR's UID.
//
// A missing Workspace yields an empty UID rather than an error, matching the
// behaviour of the cluster-wide list this replaced: that returned "" when no
// workspace matched and only failed on a genuine API error (#1875).
func workspaceUID(
	ctx context.Context, resolver *servicediscovery.Resolver, workspaceName string,
) (string, error) {
	ws, err := resolver.GetWorkspace(ctx, workspaceName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return string(ws.UID), nil
}

// loadRuntimeContextFromCRD populates context store config from the AgentRuntime CRD.
func loadRuntimeContextFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	if ar.Spec.Context == nil {
		cfg.ContextType = defaultContextType
		return nil
	}

	cfg.ContextType = string(ar.Spec.Context.Type)

	if ar.Spec.Context.TTL != nil {
		ttl, err := time.ParseDuration(*ar.Spec.Context.TTL)
		if err != nil {
			return fmt.Errorf("parse context TTL %q: %w", *ar.Spec.Context.TTL, err)
		}
		cfg.ContextTTL = ttl
	}

	// Context store URL still comes from env (secret-backed)
	cfg.ContextURL = os.Getenv(envContextURL)
	return nil
}

// ResolvedProvider is a non-default provider referenced by the AgentRuntime,
// carried through to conversation wiring where it maps to a WithXProvider option.
type ResolvedProvider struct {
	Role     v1alpha1.ProviderRole
	Provider *v1alpha1.Provider
	// APIKey is the resolved non-platform API key for this provider, carried on
	// the value rather than process env. Empty for platform/keyless providers.
	APIKey string
}

// loadProviderFromCRD resolves the provider from the AgentRuntime CRD and sets
// the API key environment variable for the PromptKit SDK.
func loadProviderFromCRD(ctx context.Context, c client.Client, cfg *Config, ar *v1alpha1.AgentRuntime, namespace string) error {
	if len(ar.Spec.Providers) > 0 {
		return loadFromNamedProviders(ctx, c, cfg, ar.Spec.Providers, namespace)
	}

	return nil
}

// loadFromNamedProviders resolves the "default" (or first sorted) named provider
// into the scalar Config fields (the llm flat path), then resolves every other
// entry into cfg.ExtraProviders keyed by its effective role.
func loadFromNamedProviders(ctx context.Context, c client.Client, cfg *Config, providers []v1alpha1.NamedProviderRef, namespace string) error {
	defaultIdx := defaultProviderIndex(providers)

	if err := loadFromProviderRef(ctx, c, cfg, providers[defaultIdx].ProviderRef, namespace); err != nil {
		return err
	}

	return loadExtraProviders(ctx, c, cfg, providers, defaultIdx, namespace)
}

// defaultProviderIndex returns the index of the entry used for the default llm
// flat-load: the "default" entry if present, otherwise the first by sorted name.
func defaultProviderIndex(providers []v1alpha1.NamedProviderRef) int {
	for i, np := range providers {
		if np.Name == "default" {
			return i
		}
	}

	// No "default": pick the index of the lexicographically-first name.
	first := 0
	for i, np := range providers {
		if np.Name < providers[first].Name {
			first = i
		}
	}
	return first
}

// loadExtraProviders resolves every entry except the one at defaultIdx into
// cfg.ExtraProviders, keyed by each provider's effective role.
func loadExtraProviders(ctx context.Context, c client.Client, cfg *Config, providers []v1alpha1.NamedProviderRef, defaultIdx int, namespace string) error {
	for i, np := range providers {
		if i == defaultIdx {
			continue
		}
		provider, err := k8s.GetProvider(ctx, c, np.ProviderRef, namespace)
		if err != nil {
			return fmt.Errorf("resolve provider %q: %w", np.Name, err)
		}
		var apiKey string
		if provider.Spec.Platform == nil {
			apiKey, err = resolveProviderAPIKey(ctx, c, provider)
			if err != nil {
				return fmt.Errorf("resolve API key for provider %q: %w", np.Name, err)
			}
		} else if err := injectPlatformCredentials(ctx, c, provider); err != nil {
			// Platform credentials still travel via process env in this wave (2b-1b).
			return fmt.Errorf("inject platform credentials for provider %q: %w", np.Name, err)
		}
		cfg.ExtraProviders = append(cfg.ExtraProviders, ResolvedProvider{
			Role:     provider.EffectiveRole(),
			Provider: provider,
			APIKey:   apiKey,
		})
	}
	return nil
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

	// Resolve credentials from secret. API keys travel on the value
	// (cfg.ProviderAPIKey); platform creds still use process env in this wave.
	if provider.Spec.Platform == nil {
		key, keyErr := resolveProviderAPIKey(ctx, c, provider)
		if keyErr != nil {
			return keyErr
		}
		cfg.ProviderAPIKey = key
		return nil
	}
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

// resolveProviderAPIKey reads the provider's Secret and returns the API key
// value. It does NOT write process env — the caller carries the value on
// Config/ResolvedProvider so same-type providers cannot overwrite each other
// (design §5.3.1). Returns "" (no error) when the provider has no secret ref
// (ollama/mock) or its type has no API-key env var name.
func resolveProviderAPIKey(ctx context.Context, c client.Client, provider *v1alpha1.Provider) (string, error) {
	ref := k8s.EffectiveSecretRef(provider)
	if ref == nil {
		return "", nil // No secret configured (e.g., ollama, mock)
	}

	// Provider types with no API-key env var name don't use a key.
	if pkgprovider.APIKeyEnvVarName(string(provider.Spec.Type)) == "" {
		return "", nil
	}

	secret, err := k8s.GetSecret(ctx, c, ref.Name, provider.Namespace)
	if err != nil {
		return "", fmt.Errorf("read provider secret: %w", err)
	}

	secretKey := k8s.DetermineSecretKey(ref, provider.Spec.Type)
	apiKeyValue, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %q", provider.Namespace, ref.Name, secretKey)
	}

	return string(apiKeyValue), nil
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
