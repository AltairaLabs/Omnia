/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// resolvedFleetProvider holds a connected fleet provider for registration after engine init.
type resolvedFleetProvider struct {
	provider *fleet.Provider
	id       string
	group    string
}

// resolveProvidersFromCRD resolves providers from CRD refs when ARENA_PROVIDER_GROUPS is set.
// It reads each Provider/AgentRuntime CRD, builds PromptKit provider configs, and populates
// LoadedProviders. Fleet providers are connected and returned for post-engine registration.
func resolveProvidersFromCRD(
	ctx context.Context,
	log logr.Logger,
	c client.Client,
	cfg *Config,
	arenaCfg *config.Config,
) ([]*resolvedFleetProvider, error) {
	// Read the ArenaJob CRD to get spec.Providers
	jobName := cfg.JobName
	jobNamespace := cfg.JobNamespace

	arenaJob, err := getArenaJob(ctx, c, jobName, jobNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to read ArenaJob %s/%s: %w", jobNamespace, jobName, err)
	}

	if len(arenaJob.Providers) == 0 {
		return nil, fmt.Errorf("ArenaJob %s/%s has no providers", jobNamespace, jobName)
	}

	if arenaCfg.LoadedProviders == nil {
		arenaCfg.LoadedProviders = make(map[string]*config.Provider)
	}
	if arenaCfg.ProviderGroups == nil {
		arenaCfg.ProviderGroups = make(map[string]string)
	}

	// Parse agent WebSocket URLs from env var (injected by controller)
	agentWSURLs := parseAgentWSURLs()

	var fleetProviders []*resolvedFleetProvider

	for groupName, entries := range arenaJob.Providers {
		for _, entry := range entries {
			if entry.ProviderRef != nil {
				if err := resolveProviderRefEntry(ctx, log, c, jobNamespace, *entry.ProviderRef, groupName, arenaCfg); err != nil {
					return nil, err
				}
			} else if entry.AgentRef != nil {
				fp, err := resolveAgentRefEntry(ctx, log, entry.AgentRef.Name, groupName, agentWSURLs, arenaCfg)
				if err != nil {
					return nil, err
				}
				fleetProviders = append(fleetProviders, fp)
			}
		}
	}

	log.Info("providers resolved from CRDs",
		"providerCount", len(arenaCfg.LoadedProviders),
		"fleetCount", len(fleetProviders),
	)

	return fleetProviders, nil
}

// resolveProviderRefEntry resolves a single Provider CRD and adds it to LoadedProviders.
func resolveProviderRefEntry(
	ctx context.Context,
	log logr.Logger,
	c client.Client,
	namespace string,
	ref v1alpha1.ProviderRef,
	groupName string,
	arenaCfg *config.Config,
) error {
	provider, err := k8s.GetProvider(ctx, c, ref, namespace)
	if err != nil {
		return fmt.Errorf("group %q: failed to get provider %s: %w", groupName, ref.Name, err)
	}

	providerID := sanitizeID(provider.Name)

	// Build PromptKit provider config
	pkProvider := &config.Provider{
		ID:      providerID,
		Type:    string(provider.Spec.Type),
		Model:   provider.Spec.Model,
		BaseURL: provider.Spec.BaseURL,
	}

	// Resolve credential
	credEnvVar := resolveProviderCredentialEnv(provider)
	if credEnvVar != "" {
		pkProvider.Credential = &config.CredentialConfig{
			CredentialEnv: credEnvVar,
		}
	}

	// Set defaults
	if provider.Spec.Defaults != nil {
		pkProvider.Defaults = convertProviderDefaults(provider.Spec.Defaults)
	}

	arenaCfg.LoadedProviders[providerID] = pkProvider
	arenaCfg.ProviderGroups[providerID] = groupName

	log.V(1).Info("provider resolved from CRD",
		"providerID", providerID,
		"type", pkProvider.Type,
		"model", pkProvider.Model,
		"group", groupName,
		"hasCreds", credEnvVar != "" && os.Getenv(credEnvVar) != "",
	)

	return nil
}

// resolveAgentRefEntry resolves an AgentRuntime CRD and creates a fleet provider.
func resolveAgentRefEntry(
	ctx context.Context,
	log logr.Logger,
	agentName string,
	groupName string,
	agentWSURLs map[string]string,
	arenaCfg *config.Config,
) (*resolvedFleetProvider, error) {
	wsURL, ok := agentWSURLs[agentName]
	if !ok {
		return nil, fmt.Errorf(
			"group %q: no WebSocket URL for agent %s (missing from ARENA_AGENT_WS_URLS)",
			groupName, agentName)
	}

	providerID := sanitizeID("agent-" + agentName)

	// Create fleet provider and connect
	fp := fleet.NewProvider(providerID, wsURL, nil)
	if err := fp.Connect(ctx); err != nil {
		return nil, fmt.Errorf("group %q: failed to connect to agent %s: %w", groupName, agentName, err)
	}

	// NOTE: Do NOT add fleet providers to LoadedProviders here.
	// BuildEngineComponents rejects the unknown "fleet" type.
	// Fleet providers are added to LoadedProviders AFTER BuildEngineComponents
	// but BEFORE NewEngine (see registerFleetProviders in worker.go).

	log.Info("agent resolved from CRD",
		"providerID", providerID,
		"agentName", agentName,
		"wsURL", wsURL,
		"group", groupName,
		"sessionID", fp.SessionID(),
	)

	return &resolvedFleetProvider{
		provider: fp,
		id:       providerID,
		group:    groupName,
	}, nil
}

// resolveToolsFromCRD resolves ToolRegistry CRDs and populates LoadedTools.
func resolveToolsFromCRD(
	ctx context.Context,
	log logr.Logger,
	c client.Client,
	cfg *Config,
) error {
	jobName := cfg.JobName
	jobNamespace := cfg.JobNamespace

	arenaJob, err := getArenaJob(ctx, c, jobName, jobNamespace)
	if err != nil {
		return fmt.Errorf("failed to read ArenaJob %s/%s: %w", jobNamespace, jobName, err)
	}

	if len(arenaJob.ToolRegistries) == 0 {
		return nil
	}

	for _, ref := range arenaJob.ToolRegistries {
		tr, err := k8s.GetToolRegistry(ctx, c, ref.Name, jobNamespace)
		if err != nil {
			return fmt.Errorf("failed to get ToolRegistry %s: %w", ref.Name, err)
		}

		toolCount := 0
		if tr.Status.DiscoveredTools != nil {
			for _, tool := range tr.Status.DiscoveredTools {
				overrideCfg := ToolOverrideConfig{
					Name:         tool.Name,
					Description:  tool.Description,
					Endpoint:     tool.Endpoint,
					HandlerName:  tool.HandlerName,
					RegistryName: tr.Name,
				}
				if cfg.ToolOverrides == nil {
					cfg.ToolOverrides = make(map[string]ToolOverrideConfig)
				}
				cfg.ToolOverrides[tool.Name] = overrideCfg
				toolCount++
			}
		}

		log.V(1).Info("tool registry resolved from CRD",
			"registry", tr.Name,
			"tools", toolCount,
		)
	}

	log.Info("tools resolved from CRDs", "totalTools", len(cfg.ToolOverrides))
	return nil
}

// registerFleetProviders registers pre-connected fleet providers into the provider registry
// AND adds them to LoadedProviders. Must be called AFTER BuildEngineComponents (which rejects
// the unknown "fleet" type) but BEFORE NewEngine (which snapshots LoadedProviders into the
// planner's provider map for GenerateRunPlan).
func registerFleetProviders(
	registry *pkproviders.Registry,
	arenaCfg *config.Config,
	fleetProviders []*resolvedFleetProvider,
) {
	for _, fp := range fleetProviders {
		registry.Register(fp.provider)
		arenaCfg.LoadedProviders[fp.id] = &config.Provider{
			ID:   fp.id,
			Type: "fleet",
		}
		if fp.group != "" {
			arenaCfg.ProviderGroups[fp.id] = fp.group
		}
	}
}

// closeFleetProviders closes all fleet provider connections.
func closeFleetProviders(fleetProviders []*resolvedFleetProvider) {
	for _, fp := range fleetProviders {
		_ = fp.provider.Close()
	}
}

// parseAgentWSURLs parses the ARENA_AGENT_WS_URLS env var (JSON map of agent name → WS URL).
func parseAgentWSURLs() map[string]string {
	raw := os.Getenv("ARENA_AGENT_WS_URLS")
	if raw == "" {
		return nil
	}

	var urls map[string]string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return nil
	}
	return urls
}

// resolveProviderCredentialEnv determines the credential env var name for a provider.
// The controller has already injected the secret as an env var; we just need to know
// which env var to tell PromptKit to read.
func resolveProviderCredentialEnv(provider *v1alpha1.Provider) string {
	// Check explicit credential config first
	if provider.Spec.Credential != nil {
		if provider.Spec.Credential.EnvVar != "" {
			return provider.Spec.Credential.EnvVar
		}
	}

	// Fall back to provider-type-based env var
	envVars := providers.GetAPIKeyEnvVars(string(provider.Spec.Type))
	if len(envVars) > 0 {
		return envVars[0]
	}

	// Provider doesn't need credentials (e.g., mock, ollama)
	return ""
}

// convertProviderDefaults converts CRD ProviderDefaults to PromptKit ProviderDefaults.
func convertProviderDefaults(d *v1alpha1.ProviderDefaults) config.ProviderDefaults {
	pd := config.ProviderDefaults{}

	if d.Temperature != nil {
		if v, err := fmt.Sscanf(*d.Temperature, "%f", &pd.Temperature); v == 0 || err != nil {
			pd.Temperature = 0
		}
	}
	if d.TopP != nil {
		if v, err := fmt.Sscanf(*d.TopP, "%f", &pd.TopP); v == 0 || err != nil {
			pd.TopP = 0
		}
	}
	if d.MaxTokens != nil {
		pd.MaxTokens = int(*d.MaxTokens)
	}

	return pd
}

// sanitizeID converts a CRD name to a safe provider ID.
// Replaces non-alphanumeric characters (except hyphens) with hyphens.
var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9-]`)

func sanitizeID(name string) string {
	return strings.ToLower(nonAlphaNum.ReplaceAllString(name, "-"))
}

// getArenaJob fetches an ArenaJob CRD. Since the core k8s package doesn't register
// EE types, we use an unstructured get and unmarshal the spec.Providers field.
func getArenaJob(
	ctx context.Context, c client.Client, name, namespace string,
) (*arenaJobSpec, error) {
	// Use unstructured client since ArenaJob is an EE type not in the core scheme
	u := &unstructuredArenaJob{}
	if err := getUnstructuredArenaJob(ctx, c, name, namespace, u); err != nil {
		return nil, err
	}
	return &u.Spec, nil
}

// arenaJobSpec is a minimal representation of ArenaJob.spec for the worker.
// We only need the providers and toolRegistries fields.
type arenaJobSpec struct {
	Providers      map[string][]arenaProviderEntry `json:"providers,omitempty"`
	ToolRegistries []v1alpha1.LocalObjectReference `json:"toolRegistries,omitempty"`
}

// arenaProviderEntry mirrors ee/api/v1alpha1.ArenaProviderEntry for worker-side parsing.
type arenaProviderEntry struct {
	ProviderRef *v1alpha1.ProviderRef          `json:"providerRef,omitempty"`
	AgentRef    *v1alpha1.LocalObjectReference `json:"agentRef,omitempty"`
}

// unstructuredArenaJob is a minimal ArenaJob for unstructured deserialization.
type unstructuredArenaJob struct {
	Spec arenaJobSpec `json:"spec"`
}

// getUnstructuredArenaJob fetches an ArenaJob using the unstructured client.
func getUnstructuredArenaJob(
	ctx context.Context, c client.Client, name, namespace string, out *unstructuredArenaJob,
) error {
	// Read the ArenaJob as unstructured and extract spec
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "omnia.altairalabs.ai",
		Version: "v1alpha1",
		Kind:    "ArenaJob",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, u); err != nil {
		return fmt.Errorf("get ArenaJob %s/%s: %w", namespace, name, err)
	}

	// Marshal to JSON and unmarshal into our typed struct
	data, err := json.Marshal(u.Object)
	if err != nil {
		return fmt.Errorf("marshal ArenaJob: %w", err)
	}

	var wrapper struct {
		Spec arenaJobSpec `json:"spec"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("unmarshal ArenaJob spec: %w", err)
	}

	out.Spec = wrapper.Spec
	return nil
}
