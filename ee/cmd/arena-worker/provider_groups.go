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
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// resolvedFleetProvider tracks a fleet provider that needs post-build connection.
// The provider is created by BuildEngineComponents via the fleet factory; we just
// need to connect it to the agent WebSocket after the engine is built.
type resolvedFleetProvider struct {
	wsURL string
	id    string
	group string
}

// resolveContext bundles the plumbing (k8s client, logger, namespace, arena
// config) threaded through every provider-resolution helper. Extracted to
// drop every resolveXxx function below Sonar's 7-param threshold (go:S107)
// and to keep call sites readable — they pass rc instead of 4–6 args.
type resolveContext struct {
	ctx         context.Context
	log         logr.Logger
	c           client.Client
	namespace   string
	agentWSURLs map[string]string
	arenaCfg    *config.Config
}

// resolveProvidersFromCRD resolves providers from CRD refs when ARENA_PROVIDER_GROUPS is set.
// It reads each Provider/AgentRuntime CRD, builds PromptKit provider configs, and populates
// LoadedProviders. Fleet providers are connected and returned for post-engine registration.
// The returned pricing map contains parsed pricing for providers that have spec.pricing set.
func resolveProvidersFromCRD(
	ctx context.Context,
	log logr.Logger,
	c client.Client,
	cfg *Config,
	arenaCfg *config.Config,
) ([]*resolvedFleetProvider, map[string]*providerPricing, error) {
	// Read the ArenaJob CRD to get spec.Providers
	jobName := cfg.JobName
	jobNamespace := cfg.JobNamespace

	arenaJob, err := getArenaJob(ctx, c, jobName, jobNamespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ArenaJob %s/%s: %w", jobNamespace, jobName, err)
	}

	if len(arenaJob.Providers) == 0 {
		return nil, nil, fmt.Errorf("ArenaJob %s/%s has no providers", jobNamespace, jobName)
	}

	// Clear providers loaded from arena config file references.
	// When spec.providers is set, CRD refs replace the arena config's provider YAML files.
	arenaCfg.LoadedProviders = make(map[string]*config.Provider)
	arenaCfg.ProviderGroups = make(map[string]string)

	// Parse agent WebSocket URLs from env var (injected by controller)
	agentWSURLs := parseAgentWSURLs()

	var fleetProviders []*resolvedFleetProvider
	pricingMap := make(map[string]*providerPricing)

	rc := &resolveContext{
		ctx:         ctx,
		log:         log,
		c:           c,
		namespace:   jobNamespace,
		agentWSURLs: agentWSURLs,
		arenaCfg:    arenaCfg,
	}
	for groupName, pg := range arenaJob.Providers {
		fps, groupPricing, err := resolveProviderGroup(rc, groupName, &pg)
		if err != nil {
			return nil, nil, err
		}
		fleetProviders = append(fleetProviders, fps...)
		for id, p := range groupPricing {
			pricingMap[id] = p
		}
	}

	log.Info("providers resolved from CRDs",
		"providerCount", len(arenaCfg.LoadedProviders),
		"fleetCount", len(fleetProviders),
		"hasPricing", len(pricingMap) > 0,
	)

	return fleetProviders, pricingMap, nil
}

// resolveProviderGroup resolves a single provider group (array or map mode).
func resolveProviderGroup(
	rc *resolveContext,
	groupName string,
	pg *arenaProviderGroup,
) ([]*resolvedFleetProvider, map[string]*providerPricing, error) {
	if pg.isMapMode() {
		return resolveMapModeGroup(rc, groupName, pg.mapping)
	}
	return resolveArrayModeGroup(rc, groupName, pg.entries)
}

// resolveMapModeGroup resolves providers in map mode (configID -> entry).
func resolveMapModeGroup(
	rc *resolveContext,
	groupName string,
	mapping map[string]arenaProviderEntry,
) ([]*resolvedFleetProvider, map[string]*providerPricing, error) {
	var fps []*resolvedFleetProvider
	pricing := make(map[string]*providerPricing)

	for configID, entry := range mapping {
		fp, p, err := resolveEntry(rc, groupName, configID, &entry)
		if err != nil {
			return nil, nil, err
		}
		if fp != nil {
			fps = append(fps, fp)
		}
		if p != nil {
			pricing[configID] = p
		}
	}
	return fps, pricing, nil
}

// resolveArrayModeGroup resolves providers in array mode (sequential entries).
func resolveArrayModeGroup(
	rc *resolveContext,
	groupName string,
	entries []arenaProviderEntry,
) ([]*resolvedFleetProvider, map[string]*providerPricing, error) {
	var fps []*resolvedFleetProvider
	pricing := make(map[string]*providerPricing)

	for _, entry := range entries {
		fp, p, err := resolveEntry(rc, groupName, "", &entry)
		if err != nil {
			return nil, nil, err
		}
		if fp != nil {
			fps = append(fps, fp)
		}
		if p != nil && entry.ProviderRef != nil {
			pricing[sanitizeID(entry.ProviderRef.Name)] = p
		}
	}
	return fps, pricing, nil
}

// resolveEntry resolves a single provider/agent entry. When configID is non-empty,
// it is used as the provider ID (map mode); otherwise sanitizeID derives it (array mode).
func resolveEntry(
	rc *resolveContext,
	groupName, configID string,
	entry *arenaProviderEntry,
) (*resolvedFleetProvider, *providerPricing, error) {
	if entry.ProviderRef != nil {
		if configID != "" {
			p, err := resolveProviderRefEntryWithID(rc, *entry.ProviderRef, configID, groupName)
			return nil, p, err
		}
		p, err := resolveProviderRefEntry(rc, *entry.ProviderRef, groupName)
		return nil, p, err
	}
	if entry.AgentRef != nil {
		if configID != "" {
			fp, err := resolveAgentRefEntryWithID(rc, entry.AgentRef.Name, configID, groupName)
			return fp, nil, err
		}
		fp, err := resolveAgentRefEntry(rc, entry.AgentRef.Name, groupName)
		return fp, nil, err
	}
	return nil, nil, nil
}

// resolveProviderRefEntry resolves a single Provider CRD and adds it to LoadedProviders.
// Returns parsed pricing if the provider has spec.pricing configured.
func resolveProviderRefEntry(
	rc *resolveContext,
	ref v1alpha1.ProviderRef,
	groupName string,
) (*providerPricing, error) {
	provider, err := k8s.GetProvider(rc.ctx, rc.c, ref, rc.namespace)
	if err != nil {
		return nil, fmt.Errorf("group %q: failed to get provider %s: %w", groupName, ref.Name, err)
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

	rc.arenaCfg.LoadedProviders[providerID] = pkProvider
	rc.arenaCfg.ProviderGroups[providerID] = groupName

	rc.log.V(1).Info("provider resolved from CRD",
		"providerID", providerID,
		"type", pkProvider.Type,
		"model", pkProvider.Model,
		"group", groupName,
		"hasCreds", credEnvVar != "" && os.Getenv(credEnvVar) != "",
	)

	return parsePricing(provider.Spec.Pricing), nil
}

// resolveAgentRefEntry resolves an AgentRuntime CRD and creates a fleet provider.
func resolveAgentRefEntry(
	rc *resolveContext,
	agentName string,
	groupName string,
) (*resolvedFleetProvider, error) {
	wsURL, ok := rc.agentWSURLs[agentName]
	if !ok {
		return nil, fmt.Errorf(
			"group %q: no WebSocket URL for agent %s (missing from ARENA_AGENT_WS_URLS)",
			groupName, agentName)
	}

	providerID := sanitizeID("agent-" + agentName)

	// Add to LoadedProviders with ws_url in AdditionalConfig.
	// The fleet provider factory (registered via init() in ee/pkg/arena/fleet/factory.go)
	// will create the Provider instance during BuildEngineComponents.
	rc.arenaCfg.LoadedProviders[providerID] = &config.Provider{
		ID:   providerID,
		Type: "fleet",
		AdditionalConfig: map[string]interface{}{
			"ws_url": wsURL,
		},
	}
	rc.arenaCfg.ProviderGroups[providerID] = groupName

	rc.log.Info("agent resolved from CRD",
		"providerID", providerID,
		"agentName", agentName,
		"wsURL", wsURL,
		"group", groupName,
	)

	return &resolvedFleetProvider{
		wsURL: wsURL,
		id:    providerID,
		group: groupName,
	}, nil
}

// resolveProviderRefEntryWithID resolves a Provider CRD using an explicit config provider ID
// instead of deriving it from sanitizeID(provider.Name). Used in map mode.
// Returns parsed pricing if the provider has spec.pricing configured.
func resolveProviderRefEntryWithID(
	rc *resolveContext,
	ref v1alpha1.ProviderRef,
	configID string,
	groupName string,
) (*providerPricing, error) {
	provider, err := k8s.GetProvider(rc.ctx, rc.c, ref, rc.namespace)
	if err != nil {
		return nil, fmt.Errorf("group %q: failed to get provider %s: %w", groupName, ref.Name, err)
	}

	pkProvider := &config.Provider{
		ID:      configID,
		Type:    string(provider.Spec.Type),
		Model:   provider.Spec.Model,
		BaseURL: provider.Spec.BaseURL,
	}

	credEnvVar := resolveProviderCredentialEnv(provider)
	if credEnvVar != "" {
		pkProvider.Credential = &config.CredentialConfig{
			CredentialEnv: credEnvVar,
		}
	}

	if provider.Spec.Defaults != nil {
		pkProvider.Defaults = convertProviderDefaults(provider.Spec.Defaults)
	}

	rc.arenaCfg.LoadedProviders[configID] = pkProvider
	rc.arenaCfg.ProviderGroups[configID] = groupName

	rc.log.V(1).Info("provider resolved from CRD (map mode)",
		"configID", configID,
		"crdName", provider.Name,
		"type", pkProvider.Type,
		"model", pkProvider.Model,
		"group", groupName,
	)

	return parsePricing(provider.Spec.Pricing), nil
}

// resolveAgentRefEntryWithID resolves an AgentRuntime CRD using an explicit config provider ID.
// Used in map mode where the key IS the config provider ID.
func resolveAgentRefEntryWithID(
	rc *resolveContext,
	agentName string,
	configID string,
	groupName string,
) (*resolvedFleetProvider, error) {
	wsURL, ok := rc.agentWSURLs[agentName]
	if !ok {
		return nil, fmt.Errorf(
			"group %q: no WebSocket URL for agent %s (missing from ARENA_AGENT_WS_URLS)",
			groupName, agentName)
	}

	rc.arenaCfg.LoadedProviders[configID] = &config.Provider{
		ID:   configID,
		Type: "fleet",
		AdditionalConfig: map[string]interface{}{
			"ws_url": wsURL,
		},
	}
	rc.arenaCfg.ProviderGroups[configID] = groupName

	rc.log.Info("agent resolved from CRD (map mode)",
		"configID", configID,
		"agentName", agentName,
		"wsURL", wsURL,
		"group", groupName,
	)

	return &resolvedFleetProvider{
		wsURL: wsURL,
		id:    configID,
		group: groupName,
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

// connectFleetProviders connects fleet providers that were created by BuildEngineComponents
// via the fleet factory. The factory creates the provider but doesn't connect it.
// This must be called AFTER BuildEngineComponents but BEFORE engine execution.
func connectFleetProviders(
	ctx context.Context,
	log logr.Logger,
	registry *pkproviders.Registry,
	fleetProviders []*resolvedFleetProvider,
) error {
	for _, fp := range fleetProviders {
		provider, _ := registry.Get(fp.id)
		if provider == nil {
			return fmt.Errorf("fleet provider %q not found in registry after engine build", fp.id)
		}
		fleetProv, ok := provider.(*fleet.Provider)
		if !ok {
			return fmt.Errorf("provider %q is not a fleet provider (type %T)", fp.id, provider)
		}
		if err := fleetProv.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect fleet provider %q: %w", fp.id, err)
		}
		log.Info("fleet provider connected",
			"providerID", fp.id,
			"wsURL", fp.wsURL,
			"sessionID", fleetProv.SessionID(),
		)
	}
	return nil
}

// closeFleetProviders closes all fleet provider connections via the registry.
func closeFleetProviders(registry *pkproviders.Registry, fleetProviders []*resolvedFleetProvider) {
	for _, fp := range fleetProviders {
		provider, _ := registry.Get(fp.id)
		if provider == nil {
			continue
		}
		if fleetProv, ok := provider.(*fleet.Provider); ok {
			_ = fleetProv.Close()
		}
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

// remapProviderIDs remaps CRD-resolved provider IDs to match the IDs expected by
// the arena config's self-play roles and judges. When CRD resolution creates a provider
// with key "ollama-tools" in group "selfplay", but the config references provider "selfplay",
// PromptKit's engine fails with "unknown provider". This function bridges that gap.
func remapProviderIDs(log logr.Logger, arenaCfg *config.Config, configPath string) error {
	expectedIDs, err := extractProviderIDRefs(configPath)
	if err != nil {
		return fmt.Errorf("extract provider ID refs from config: %w", err)
	}
	if len(expectedIDs) == 0 {
		return nil
	}

	// Build reverse map: group → []providerID from ProviderGroups
	groupToProviders := make(map[string][]string)
	for provID, group := range arenaCfg.ProviderGroups {
		groupToProviders[group] = append(groupToProviders[group], provID)
	}

	for _, expectedID := range expectedIDs {
		// Skip if already present in LoadedProviders
		if _, exists := arenaCfg.LoadedProviders[expectedID]; exists {
			continue
		}

		// Look for CRD providers whose group matches the expected ID
		candidates := groupToProviders[expectedID]
		if len(candidates) == 0 {
			return fmt.Errorf(
				"config references provider %q but no provider in group %q",
				expectedID, expectedID,
			)
		}
		// Pick the first provider in the group to remap.
		// Groups can have multiple providers (e.g., for A/B testing); we remap
		// one to the expected ID so PromptKit can find it, others keep their CRD names.
		oldID := candidates[0]
		provider := arenaCfg.LoadedProviders[oldID]
		provider.ID = expectedID

		// Move in LoadedProviders
		delete(arenaCfg.LoadedProviders, oldID)
		arenaCfg.LoadedProviders[expectedID] = provider

		// Update ProviderGroups
		delete(arenaCfg.ProviderGroups, oldID)
		arenaCfg.ProviderGroups[expectedID] = expectedID

		log.V(1).Info("provider ID remapped",
			"oldID", oldID,
			"newID", expectedID,
			"group", expectedID,
		)
	}

	return nil
}

// arenaConfigProviderRefs is a minimal representation of the arena config YAML
// for extracting provider ID references from self-play roles, judges, and judge specs.
type arenaConfigProviderRefs struct {
	Spec struct {
		SelfPlay *struct {
			Enabled bool `yaml:"enabled"`
			Roles   []struct {
				Provider string `yaml:"provider"`
			} `yaml:"roles"`
		} `yaml:"self_play"`
		Judges []struct {
			Provider string `yaml:"provider"`
		} `yaml:"judges"`
		JudgeSpecs map[string]struct {
			Provider string `yaml:"provider"`
		} `yaml:"judge_specs"`
	} `yaml:"spec"`
}

// extractProviderIDRefs parses the arena config YAML and returns all provider IDs
// referenced by self-play roles, judges, and judge specs.
func extractProviderIDRefs(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read arena config: %w", err)
	}

	var cfg arenaConfigProviderRefs
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse arena config: %w", err)
	}

	seen := make(map[string]bool)
	var ids []string

	addID := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if cfg.Spec.SelfPlay != nil && cfg.Spec.SelfPlay.Enabled {
		for _, role := range cfg.Spec.SelfPlay.Roles {
			addID(role.Provider)
		}
	}

	for _, judge := range cfg.Spec.Judges {
		addID(judge.Provider)
	}

	for _, spec := range cfg.Spec.JudgeSpecs {
		addID(spec.Provider)
	}

	return ids, nil
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
	Providers      map[string]arenaProviderGroup   `json:"providers,omitempty"`
	ToolRegistries []v1alpha1.LocalObjectReference `json:"toolRegistries,omitempty"`
}

// arenaProviderEntry mirrors ee/api/v1alpha1.ArenaProviderEntry for worker-side parsing.
type arenaProviderEntry struct {
	ProviderRef *v1alpha1.ProviderRef          `json:"providerRef,omitempty"`
	AgentRef    *v1alpha1.LocalObjectReference `json:"agentRef,omitempty"`
}

// arenaProviderGroup is a polymorphic provider group value (worker-side mirror).
// Array mode: []arenaProviderEntry (test provider pools).
// Map mode: map[string]arenaProviderEntry (1:1 config-provider-ID → CRD mappings).
type arenaProviderGroup struct {
	entries []arenaProviderEntry
	mapping map[string]arenaProviderEntry
}

// UnmarshalJSON detects array vs object and populates the correct field.
func (g *arenaProviderGroup) UnmarshalJSON(data []byte) error {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '[':
			return json.Unmarshal(data, &g.entries)
		case '{':
			return json.Unmarshal(data, &g.mapping)
		default:
			return json.Unmarshal(data, &g.entries)
		}
	}
	return nil
}

// MarshalJSON serialises as object (map mode) or array (entries mode).
func (g arenaProviderGroup) MarshalJSON() ([]byte, error) {
	if g.mapping != nil {
		return json.Marshal(g.mapping)
	}
	return json.Marshal(g.entries)
}

// isMapMode returns true when the group uses 1:1 config-provider-ID mapping.
func (g *arenaProviderGroup) isMapMode() bool {
	return g.mapping != nil
}

// allEntries returns every arenaProviderEntry regardless of mode.
func (g *arenaProviderGroup) allEntries() []arenaProviderEntry {
	if g.mapping != nil {
		entries := make([]arenaProviderEntry, 0, len(g.mapping))
		for _, e := range g.mapping {
			entries = append(entries, e)
		}
		return entries
	}
	return g.entries
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
