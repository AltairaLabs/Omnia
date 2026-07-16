package deploy

import (
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

const packJSONKey = "pack.json"

// contentConfigMapName is the deterministic name of the ConfigMap holding a
// PromptPack's compiled pack.json, derived from the pack's object name.
func contentConfigMapName(packObjectName string) string {
	return packObjectName + "-content"
}

// mergeLabels returns base with the deploy-wide labels overlaid (deploy labels
// never override the reserved keys base sets).
func mergeLabels(base, deploy map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range deploy {
		out[k] = v
	}
	for k, v := range base {
		out[k] = v
	}
	return out
}

// packToPromptPack builds the immutable PromptPack object for {pack.Name,
// pack.Version}. The name is deterministic so a duplicate coordinate is rejected
// natively by the apiserver (AlreadyExists).
func packToPromptPack(namespace string, pack PackIntent, deployLabels map[string]string) *omniav1alpha1.PromptPack {
	name := omniav1alpha1.PromptPackObjectName(pack.Name, pack.Version)
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: pack.Name,
			Version:  pack.Version,
			Source: omniav1alpha1.PromptPackContentSource{
				Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: contentConfigMapName(name)},
			},
			Skills:       skillRefs(pack.Skills),
			SkillsConfig: skillsConfig(pack.SkillsConfig),
		},
	}
}

// packContentConfigMap builds the ConfigMap holding the raw pack.json.
func packContentConfigMap(namespace string, pack PackIntent, deployLabels map[string]string) *corev1.ConfigMap {
	name := omniav1alpha1.PromptPackObjectName(pack.Name, pack.Version)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      contentConfigMapName(name),
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Data: map[string]string{packJSONKey: pack.Content},
	}
}

func skillRefs(in []SkillRefIntent) []omniav1alpha1.SkillRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]omniav1alpha1.SkillRef, 0, len(in))
	for _, s := range in {
		out = append(out, omniav1alpha1.SkillRef{Source: s.Source, Include: s.Include, MountAs: s.MountAs})
	}
	return out
}

func skillsConfig(in *SkillsConfigIntent) *omniav1alpha1.SkillsConfig {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.SkillsConfig{MaxActive: in.MaxActive, Selector: omniav1alpha1.SkillSelector(in.Selector)}
}

const promptNameEnv = "OMNIA_PROMPT_NAME"

// agentToAgentRuntime builds the desired AgentRuntime for one AgentIntent,
// pinned to pack.Version. The apply step (apply.go) decides whether an existing
// trigger-mode agent keeps its live pin instead. registryName is the resolved
// ToolRegistry name (see deployRegistryName) — the caller computes it once per
// deploy so every agent and the AgentPolicy denylist rule agree on the same
// registry, whether it came from tools.ref or the pack's handler convention
// name. A registryName of "" means the deploy has no tools, so ToolRegistryRef
// is left nil even when agent.UseTools is set — a dangling ref to a
// non-existent registry is worse than silently granting no tools.
func agentToAgentRuntime(namespace string, pack PackIntent, agent AgentIntent, registryName string, deployLabels map[string]string) *omniav1alpha1.AgentRuntime {
	version := pack.Version
	spec := omniav1alpha1.AgentRuntimeSpec{
		PromptPackRef: omniav1alpha1.PromptPackRef{Name: pack.Name, Version: &version},
		Facades:       facadeConfigs(agent.Facades),
		Providers:     providerRefs(agent.Providers),
		Runtime:       runtimeConfig(agent),
		Rollout:       rolloutConfig(agent.Rollout),
		ExternalAuth:  externalAuthConfig(agent.ExternalAuth),
		Memory:        memoryConfig(agent.Memory),
		Evals:         evalConfig(agent.Evals),
	}
	if agent.UseTools && registryName != "" {
		spec.ToolRegistryRef = &omniav1alpha1.ToolRegistryRef{Name: registryName}
	}
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Spec: spec,
	}
}

// toolRegistryName is the deterministic ToolRegistry name for a pack (matches
// the adapter's "<pack>-tools" convention). The registry is materialized
// create-only from tools.handlers by toolRegistry / applyToolRegistry.
func toolRegistryName(packName string) string { return packName + "-tools" }

// deployRegistryName is the single source of truth for which ToolRegistry a
// deploy's agents and AgentPolicy denylist rule point at. tools.ref names an
// EXISTING, operator/user-owned registry and always wins when set — it is
// never overridden by the pack's handler-convention name. Only when tools has
// handlers and no ref does the deploy create (and point at) the
// "<pack>-tools" registry. Returns "" when the deploy has no tools at all, so
// callers can distinguish "no registry" from "use the pack convention name".
func deployRegistryName(pack PackIntent, tools *ToolsIntent) string {
	if tools == nil {
		return ""
	}
	if tools.Ref != "" {
		return tools.Ref
	}
	if len(tools.Handlers) > 0 {
		return toolRegistryName(pack.Name)
	}
	return ""
}

// toolRegistry builds the create-only ToolRegistry for tools.Handlers. Returns
// nil (no error) when tools is nil or has no handlers — a ref-only ToolsIntent
// points AgentRuntimes at an existing, operator/user-owned registry and
// creates nothing. A malformed handler config block (raw JSON that doesn't
// unmarshal into the corresponding CRD field) is a translation error the
// caller must surface, not a panic.
func toolRegistry(namespace string, pack PackIntent, tools *ToolsIntent, deployLabels map[string]string) (*omniav1alpha1.ToolRegistry, error) {
	if tools == nil || len(tools.Handlers) == 0 {
		return nil, nil
	}
	handlers := make([]omniav1alpha1.HandlerDefinition, 0, len(tools.Handlers))
	for i, h := range tools.Handlers {
		hd, err := handlerDefinition(h)
		if err != nil {
			return nil, fmt.Errorf("tools.handlers[%d]: %w", i, err)
		}
		handlers = append(handlers, hd)
	}
	return &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toolRegistryName(pack.Name),
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Spec: omniav1alpha1.ToolRegistrySpec{Handlers: handlers},
	}, nil
}

// handlerDefinition translates one HandlerIntent onto the real
// HandlerDefinition CRD shape: Name/Type direct, Timeout parsed from the
// intent's duration string, and each non-nil raw-JSON config block unmarshaled
// into its corresponding CRD pointer field.
func handlerDefinition(h HandlerIntent) (omniav1alpha1.HandlerDefinition, error) {
	hd := omniav1alpha1.HandlerDefinition{
		Name: h.Name,
		Type: omniav1alpha1.HandlerType(h.Type),
	}
	if h.Timeout != "" {
		d, err := time.ParseDuration(h.Timeout)
		if err != nil {
			return omniav1alpha1.HandlerDefinition{}, fmt.Errorf("timeout: invalid duration %q: %w", h.Timeout, err)
		}
		hd.Timeout = &metav1.Duration{Duration: d}
	}
	if err := unmarshalHandlerConfigs(h, &hd); err != nil {
		return omniav1alpha1.HandlerDefinition{}, err
	}
	return hd, nil
}

// unmarshalHandlerConfigs decodes each non-nil raw-JSON config block on h into
// its corresponding CRD field on hd. Carrying config as raw JSON on
// HandlerIntent lets the intent track CRD growth without re-typing every
// executor's fields. Delegates the identical unmarshal-or-error-wrap step for
// each field to decodeHandlerConfig to keep this function's cognitive
// complexity low despite the field count.
func unmarshalHandlerConfigs(h HandlerIntent, hd *omniav1alpha1.HandlerDefinition) error {
	if err := decodeHandlerConfig(h.Tool, "tool", &hd.Tool); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.HTTPConfig, "httpConfig", &hd.HTTPConfig); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.OpenAPIConfig, "openAPIConfig", &hd.OpenAPIConfig); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.GRPCConfig, "grpcConfig", &hd.GRPCConfig); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.MCPConfig, "mcpConfig", &hd.MCPConfig); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.ClientConfig, "clientConfig", &hd.ClientConfig); err != nil {
		return err
	}
	if err := decodeHandlerConfig(h.Auth, "auth", &hd.Auth); err != nil {
		return err
	}
	return nil
}

// decodeHandlerConfig unmarshals raw into a new T and points *out at it, when
// raw is non-nil. label names the field in a wrapped error on failure.
func decodeHandlerConfig[T any](raw *json.RawMessage, label string, out **T) error {
	if raw == nil {
		return nil
	}
	var v T
	if err := json.Unmarshal(*raw, &v); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	*out = &v
	return nil
}

func facadeConfigs(in []FacadeIntent) []omniav1alpha1.FacadeConfig {
	runtimeHandler := omniav1alpha1.HandlerModeRuntime
	if len(in) == 0 {
		mgmt := true
		return []omniav1alpha1.FacadeConfig{{
			Type:            omniav1alpha1.FacadeTypeWebSocket,
			Handler:         &runtimeHandler,
			ManagementPlane: &mgmt,
		}}
	}
	out := make([]omniav1alpha1.FacadeConfig, 0, len(in))
	for _, f := range in {
		out = append(out, omniav1alpha1.FacadeConfig{
			Type:            omniav1alpha1.FacadeType(f.Type),
			Handler:         &runtimeHandler,
			ManagementPlane: f.ManagementPlane,
		})
	}
	return out
}

func providerRefs(in []ProviderBind) []omniav1alpha1.NamedProviderRef {
	out := make([]omniav1alpha1.NamedProviderRef, 0, len(in))
	for _, p := range in {
		role := omniav1alpha1.ProviderRole(p.Role)
		if p.Role == "" {
			role = omniav1alpha1.ProviderRoleLLM
		}
		out = append(out, omniav1alpha1.NamedProviderRef{
			Name:        p.Name,
			ProviderRef: omniav1alpha1.ProviderRef{Name: p.Ref},
			Role:        role,
		})
	}
	return out
}

func runtimeConfig(agent AgentIntent) *omniav1alpha1.RuntimeConfig {
	var rc omniav1alpha1.RuntimeConfig
	set := false
	if agent.Runtime != nil {
		if agent.Runtime.Replicas != nil {
			rc.Replicas = agent.Runtime.Replicas
			set = true
		}
		if req := resourceRequirements(agent.Runtime); req != nil {
			rc.Resources = req
			set = true
		}
	}
	if agent.PromptName != "" {
		rc.ExtraEnv = append(rc.ExtraEnv, corev1.EnvVar{Name: promptNameEnv, Value: agent.PromptName})
		set = true
	}
	if !set {
		return nil
	}
	return &rc
}

func resourceRequirements(r *RuntimeIntent) *corev1.ResourceRequirements {
	if r.CPU == "" && r.Memory == "" {
		return nil
	}
	reqs := corev1.ResourceList{}
	if r.CPU != "" {
		reqs[corev1.ResourceCPU] = resource.MustParse(r.CPU)
	}
	if r.Memory != "" {
		reqs[corev1.ResourceMemory] = resource.MustParse(r.Memory)
	}
	return &corev1.ResourceRequirements{Requests: reqs}
}

func rolloutConfig(in *RolloutIntent) *omniav1alpha1.RolloutConfig {
	if in == nil {
		return nil
	}
	rc := &omniav1alpha1.RolloutConfig{}
	if in.Trigger != nil {
		rc.Trigger = &omniav1alpha1.RolloutTrigger{PromptPackChannel: in.Trigger.PromptPackChannel}
	}
	for _, s := range in.Steps {
		step := omniav1alpha1.RolloutStep{SetWeight: s.SetWeight}
		if s.PauseDuration != "" {
			step.Pause = &omniav1alpha1.RolloutPause{Duration: rolloutPauseDuration(s.PauseDuration)}
		}
		rc.Steps = append(rc.Steps, step)
	}
	return rc
}

// rolloutPauseDuration builds the RolloutPause.Duration pointer (*string).
func rolloutPauseDuration(s string) *string { return &s }

// externalAuthConfig maps ExternalAuthIntent field-for-field onto the real
// AgentExternalAuth CRD shape. nil-safe: nil in => nil out. Nested sub-structs
// are only built when their intent pointer is non-nil, so an unset OIDC
// claimMapping (etc.) stays nil rather than becoming an empty struct.
func externalAuthConfig(in *ExternalAuthIntent) *omniav1alpha1.AgentExternalAuth {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.AgentExternalAuth{
		ClientKeys: clientKeysAuth(in.ClientKeys),
		OIDC:       oidcAuth(in.OIDC),
		EdgeTrust:  edgeTrustAuth(in.EdgeTrust),
	}
}

func clientKeysAuth(in *ClientKeysIntent) *omniav1alpha1.ClientKeysAuth {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.ClientKeysAuth{
		DefaultRole:        in.DefaultRole,
		TrustEndUserHeader: in.TrustEndUserHeader,
	}
}

func oidcAuth(in *OIDCIntent) *omniav1alpha1.OIDCAuth {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.OIDCAuth{
		Issuer:       in.Issuer,
		Audience:     in.Audience,
		ClaimMapping: oidcClaimMapping(in.ClaimMapping),
	}
}

func oidcClaimMapping(in *OIDCMappingIntent) *omniav1alpha1.OIDCClaimMapping {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.OIDCClaimMapping{
		Subject: in.Subject,
		EndUser: in.EndUser,
	}
}

func edgeTrustAuth(in *EdgeTrustIntent) *omniav1alpha1.EdgeTrustAuth {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.EdgeTrustAuth{
		HeaderMapping:     edgeTrustHeaderMapping(in.HeaderMapping),
		ClaimsFromHeaders: in.ClaimsFromHeaders,
	}
}

func edgeTrustHeaderMapping(in *EdgeTrustHeaderIntent) *omniav1alpha1.EdgeTrustHeaderMapping {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.EdgeTrustHeaderMapping{
		Subject: in.Subject,
		EndUser: in.EndUser,
		Email:   in.Email,
	}
}

// memoryConfig maps MemoryIntent onto the real MemoryConfig CRD shape. nil-safe:
// nil in => nil out. The intent's flat Retrieval.DenyCEL becomes the nested
// Retrieval.AccessFilter.DenyCEL — AccessFilter is only built when DenyCEL is
// non-empty, so an unset deny policy stays nil rather than an empty struct.
func memoryConfig(in *MemoryIntent) *omniav1alpha1.MemoryConfig {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.MemoryConfig{
		Enabled:   in.Enabled,
		Retrieval: memoryRetrievalConfig(in.Retrieval),
		Tools:     memoryToolsConfig(in.Tools),
	}
}

func memoryRetrievalConfig(in *MemoryRetrievalIntent) *omniav1alpha1.MemoryRetrievalConfig {
	if in == nil {
		return nil
	}
	rc := &omniav1alpha1.MemoryRetrievalConfig{
		Enabled:  in.Enabled,
		Strategy: in.Strategy,
		Limit:    in.Limit,
	}
	if in.DenyCEL != "" {
		rc.AccessFilter = &omniav1alpha1.MemoryAccessFilterConfig{DenyCEL: in.DenyCEL}
	}
	return rc
}

func memoryToolsConfig(in *MemoryToolsIntent) *omniav1alpha1.MemoryToolsConfig {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.MemoryToolsConfig{Enabled: in.Enabled}
}

// evalConfig maps EvalsIntent onto the real EvalConfig CRD shape. nil-safe: nil
// in => nil out. Inline/Worker are only built when their group slice is
// non-empty; other EvalConfig fields (Sampling, RateLimit, SessionCompletion,
// PodOverrides) are not part of Plan B and stay unset.
func evalConfig(in *EvalsIntent) *omniav1alpha1.EvalConfig {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.EvalConfig{
		Enabled: in.Enabled,
		Inline:  evalPathConfig(in.Inline),
		Worker:  evalPathConfig(in.Worker),
	}
}

func evalPathConfig(groups []string) *omniav1alpha1.EvalPathConfig {
	if len(groups) == 0 {
		return nil
	}
	return &omniav1alpha1.EvalPathConfig{Groups: groups}
}

// agentPolicyNameSuffix names the AgentPolicy derived from a pack's
// toolBlocklist, matching the "<pack>-tools" ToolRegistry naming convention.
const agentPolicyNameSuffix = "-policy"

// agentPolicyName is the deterministic AgentPolicy name for a pack.
func agentPolicyName(packName string) string { return packName + agentPolicyNameSuffix }

// agentPolicy builds the desired AgentPolicy translating PolicyIntent's flat
// toolBlocklist into a toolAccess denylist rule against registryName. Returns
// nil when there's nothing to apply: no policy, an empty blocklist, or no
// registry to attach the denylist rule to — a denylist rule requires a
// ToolAccessRule.Registry name (CRD MinLength=1). Validate (types.go) already
// rejects a non-empty blocklist with no tools at the HTTP layer, so the
// registryName=="" case here is a translate-level guard against that same
// invariant, not a path expected to be hit at apply time.
func agentPolicy(namespace string, pack PackIntent, policy *PolicyIntent, agentNames []string, registryName string, deployLabels map[string]string) *omniav1alpha1.AgentPolicy {
	if policy == nil || len(policy.ToolBlocklist) == 0 || registryName == "" {
		return nil
	}
	return &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentPolicyName(pack.Name),
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Selector: &omniav1alpha1.AgentPolicySelector{Agents: agentNames},
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: registryName, Tools: policy.ToolBlocklist},
				},
			},
			Mode:      omniav1alpha1.AgentPolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
}
