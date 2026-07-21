package deploy

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

// handlerTypeClient is the HandlerIntent.Type value for client-side tools,
// shared across translate_test.go and apply_test.go test fixtures to avoid
// the goconst "client" literal appearing 3+ times across the package.
const handlerTypeClient = "client"

func TestPackToPromptPack(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0", Content: `{"id":"support"}`}
	want := omniav1alpha1.PromptPackObjectName("support", "1.2.0")

	pp := packToPromptPack("ns", pack, map[string]string{"env": "prod"})
	if pp.Name != want {
		t.Errorf("name = %q, want %q", pp.Name, want)
	}
	if pp.Namespace != "ns" {
		t.Errorf("namespace = %q, want ns", pp.Namespace)
	}
	if pp.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, pp.Labels[packselect.Label])
	}
	if pp.Labels["env"] != "prod" {
		t.Errorf("deploy label not propagated: %v", pp.Labels)
	}
	if pp.Spec.PackName != "support" || pp.Spec.Version != "1.2.0" {
		t.Errorf("spec = %+v", pp.Spec)
	}
	if pp.Spec.Source.Type != omniav1alpha1.PromptPackSourceTypeConfigMap {
		t.Errorf("source type = %q", pp.Spec.Source.Type)
	}
	if pp.Spec.Source.ConfigMapRef == nil || pp.Spec.Source.ConfigMapRef.Name != contentConfigMapName(want) {
		t.Errorf("configMapRef = %+v", pp.Spec.Source.ConfigMapRef)
	}
}

func TestPackContentConfigMap(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0", Content: `{"id":"support"}`}
	cm := packContentConfigMap("ns", pack, nil)
	if cm.Name != contentConfigMapName(omniav1alpha1.PromptPackObjectName("support", "1.2.0")) {
		t.Errorf("cm name = %q", cm.Name)
	}
	if cm.Data["pack.json"] != `{"id":"support"}` {
		t.Errorf("pack.json = %q", cm.Data["pack.json"])
	}
}

func TestPackToPromptPackWithSkills(t *testing.T) {
	maxActive := int32(3)
	pack := PackIntent{
		Name:    "support",
		Version: "1.2.0",
		Content: `{"id":"support"}`,
		Skills: []SkillRefIntent{
			{Source: "docs-source", Include: []string{"faq"}, MountAs: "docs"},
		},
		SkillsConfig: &SkillsConfigIntent{MaxActive: &maxActive, Selector: "tag"},
	}

	pp := packToPromptPack("ns", pack, nil)

	if len(pp.Spec.Skills) != 1 {
		t.Fatalf("skills = %+v, want 1 entry", pp.Spec.Skills)
	}
	got := pp.Spec.Skills[0]
	want := omniav1alpha1.SkillRef{Source: "docs-source", Include: []string{"faq"}, MountAs: "docs"}
	if got.Source != want.Source || got.MountAs != want.MountAs || len(got.Include) != 1 || got.Include[0] != "faq" {
		t.Errorf("skills[0] = %+v, want %+v", got, want)
	}

	if pp.Spec.SkillsConfig == nil {
		t.Fatal("skillsConfig = nil, want non-nil")
	}
	if pp.Spec.SkillsConfig.MaxActive == nil || *pp.Spec.SkillsConfig.MaxActive != maxActive {
		t.Errorf("skillsConfig.MaxActive = %v, want %d", pp.Spec.SkillsConfig.MaxActive, maxActive)
	}
	if pp.Spec.SkillsConfig.Selector != omniav1alpha1.SkillSelectorTag {
		t.Errorf("skillsConfig.Selector = %q, want %q", pp.Spec.SkillsConfig.Selector, omniav1alpha1.SkillSelectorTag)
	}

	// nil deployLabels must not panic and must still stamp the packselect label.
	if pp.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, pp.Labels[packselect.Label])
	}
}

// assertAgentRuntimeMeta and hasEnvVar are extracted out of
// TestAgentToAgentRuntime_Pinned to keep the test under the repo's gocyclo-15
// pre-commit gate; the assertions themselves are unchanged from the brief.
func assertAgentRuntimeMeta(t *testing.T, ar *omniav1alpha1.AgentRuntime, wantName, wantNamespace string) {
	t.Helper()
	if ar.Name != wantName || ar.Namespace != wantNamespace {
		t.Fatalf("meta = %s/%s", ar.Namespace, ar.Name)
	}
}

func hasEnvVar(rc *omniav1alpha1.RuntimeConfig, name, value string) bool {
	if rc == nil {
		return false
	}
	for _, e := range rc.ExtraEnv {
		if e.Name == name && e.Value == value {
			return true
		}
	}
	return false
}

func TestAgentToAgentRuntime_Pinned(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:       "support-triage",
		PromptName: "triage",
		Providers:  []ProviderBind{{Name: "default", Ref: "claude"}, {Name: "judge", Ref: "gpt", Role: "llm"}},
		UseTools:   true,
	}
	ar := agentToAgentRuntime("ns", pack, agent, toolRegistryName("support"), nil)

	assertAgentRuntimeMeta(t, ar, "support-triage", "ns")
	if ar.Spec.PromptPackRef.Name != "support" || ar.Spec.PromptPackRef.Version == nil || *ar.Spec.PromptPackRef.Version != "1.2.0" {
		t.Errorf("promptPackRef = %+v", ar.Spec.PromptPackRef)
	}
	if ar.Spec.PromptPackRef.Track != nil {
		t.Errorf("pinned mode must not set track")
	}
	if len(ar.Spec.Facades) != 1 || ar.Spec.Facades[0].Type != omniav1alpha1.FacadeTypeWebSocket {
		t.Errorf("facades = %+v", ar.Spec.Facades)
	}
	if len(ar.Spec.Providers) != 2 || ar.Spec.Providers[0].Name != "default" || ar.Spec.Providers[0].ProviderRef.Name != "claude" {
		t.Errorf("providers = %+v", ar.Spec.Providers)
	}
	if ar.Spec.ToolRegistryRef == nil {
		t.Errorf("useTools true but toolRegistryRef nil")
	}
	// OMNIA_PROMPT_NAME env pins the prompt for a fanned-out runtime. The literal
	// (not the promptNameEnv constant) is asserted here because OMNIA_PROMPT_NAME
	// is a cross-package external contract (also literal in internal/runtime/config.go
	// and internal/controller/deployment_builder_env.go) — pinning the literal means
	// a future rename of the local constant can't silently pass while breaking it.
	if !hasEnvVar(ar.Spec.Runtime, "OMNIA_PROMPT_NAME", "triage") {
		t.Errorf("OMNIA_PROMPT_NAME env not set: %+v", ar.Spec.Runtime)
	}
}

// TestAgentToAgentRuntime_RegistryNameFlowsThroughAsToolRegistryRef verifies
// the #1862-adjacent fix: agentToAgentRuntime no longer recomputes
// "<pack>-tools" internally — it trusts the caller-resolved registryName
// verbatim, so a tools.ref value (which names an EXISTING registry, not the
// pack's own "<pack>-tools" convention name) reaches spec.toolRegistryRef
// unchanged instead of being silently overridden.
func TestAgentToAgentRuntime_RegistryNameFlowsThroughAsToolRegistryRef(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support-triage",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		UseTools:  true,
	}
	ar := agentToAgentRuntime("ns", pack, agent, "other-registry", nil)
	if ar.Spec.ToolRegistryRef == nil || ar.Spec.ToolRegistryRef.Name != "other-registry" {
		t.Errorf("toolRegistryRef = %+v, want name=other-registry", ar.Spec.ToolRegistryRef)
	}
}

// TestAgentToAgentRuntime_EmptyRegistryNameLeavesRefNil verifies that when
// the deploy has no resolvable registry (registryName == ""), UseTools:true
// does NOT produce a dangling ToolRegistryRef pointing at a registry that was
// never created — better to grant no tools than reference a 404.
func TestAgentToAgentRuntime_EmptyRegistryNameLeavesRefNil(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support-triage",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		UseTools:  true,
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.ToolRegistryRef != nil {
		t.Errorf("toolRegistryRef = %+v, want nil (registryName empty)", ar.Spec.ToolRegistryRef)
	}
}

func TestDeployRegistryName(t *testing.T) {
	pack := PackIntent{Name: "support"}

	if got := deployRegistryName(pack, nil); got != "" {
		t.Errorf("nil tools: got %q, want \"\"", got)
	}
	if got := deployRegistryName(pack, &ToolsIntent{}); got != "" {
		t.Errorf("empty tools: got %q, want \"\"", got)
	}
	if got := deployRegistryName(pack, &ToolsIntent{Ref: "other-registry"}); got != "other-registry" {
		t.Errorf("ref-only: got %q, want other-registry", got)
	}
	handlersOnly := &ToolsIntent{Handlers: []HandlerIntent{{Name: "h", Type: handlerTypeClient}}}
	if got := deployRegistryName(pack, handlersOnly); got != toolRegistryName("support") {
		t.Errorf("handlers-only: got %q, want %q", got, toolRegistryName("support"))
	}
	// Ref wins even when handlers are also present (shouldn't normally co-occur,
	// but the resolution priority must still be well-defined).
	both := &ToolsIntent{Ref: "other-registry", Handlers: handlersOnly.Handlers}
	if got := deployRegistryName(pack, both); got != "other-registry" {
		t.Errorf("ref+handlers: got %q, want other-registry (ref wins)", got)
	}
}

func TestAgentToAgentRuntime_TriggerRollout(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Rollout:   &RolloutIntent{Trigger: &RolloutTriggerIntent{PromptPackChannel: "stable"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Trigger == nil || ar.Spec.Rollout.Trigger.PromptPackChannel != "stable" {
		t.Fatalf("rollout trigger = %+v", ar.Spec.Rollout)
	}
	// Even in trigger mode the *desired* object pins the version; the apply step
	// is what preserves the live pin on an EXISTING agent (see apply_test).
	if ar.Spec.PromptPackRef.Version == nil || *ar.Spec.PromptPackRef.Version != "1.2.0" {
		t.Errorf("promptPackRef = %+v", ar.Spec.PromptPackRef)
	}
}

func TestAgentToAgentRuntime_ExplicitFacades(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	mgmt := false
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Facades:   []FacadeIntent{{Type: "rest", ManagementPlane: &mgmt}, {Type: "mcp"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)

	if len(ar.Spec.Facades) != 2 {
		t.Fatalf("facades = %+v, want 2 entries", ar.Spec.Facades)
	}
	if ar.Spec.Facades[0].Type != omniav1alpha1.FacadeTypeREST || ar.Spec.Facades[0].ManagementPlane == nil || *ar.Spec.Facades[0].ManagementPlane != false {
		t.Errorf("facades[0] = %+v", ar.Spec.Facades[0])
	}
	if ar.Spec.Facades[1].Type != omniav1alpha1.FacadeTypeMCP {
		t.Errorf("facades[1] = %+v", ar.Spec.Facades[1])
	}
	for i, f := range ar.Spec.Facades {
		if f.Handler == nil || *f.Handler != omniav1alpha1.HandlerModeRuntime {
			t.Errorf("facades[%d].Handler = %+v, want runtime", i, f.Handler)
		}
	}
}

func TestAgentToAgentRuntime_RuntimeReplicasAndResources(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	replicas := int32(3)
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Runtime:   &RuntimeIntent{Replicas: &replicas, CPU: "500m", Memory: "256Mi"},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)

	if ar.Spec.Runtime == nil {
		t.Fatal("runtime = nil")
	}
	if ar.Spec.Runtime.Replicas == nil || *ar.Spec.Runtime.Replicas != 3 {
		t.Errorf("replicas = %v, want 3", ar.Spec.Runtime.Replicas)
	}
	if ar.Spec.Runtime.Resources == nil {
		t.Fatal("resources = nil")
	}
	cpu := ar.Spec.Runtime.Resources.Requests[corev1.ResourceCPU]
	if cpu.String() != "500m" {
		t.Errorf("cpu = %s, want 500m", cpu.String())
	}
	mem := ar.Spec.Runtime.Resources.Requests[corev1.ResourceMemory]
	if mem.String() != "256Mi" {
		t.Errorf("memory = %s, want 256Mi", mem.String())
	}
}

func TestAgentToAgentRuntime_RuntimeNilWhenUnset(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Runtime:   &RuntimeIntent{},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.Runtime != nil {
		t.Errorf("runtime = %+v, want nil", ar.Spec.Runtime)
	}
}

func TestExternalAuthConfig_Nil(t *testing.T) {
	if got := externalAuthConfig(nil); got != nil {
		t.Errorf("externalAuthConfig(nil) = %+v, want nil", got)
	}
}

// assertClientKeysAuth, assertOIDCAuth, and assertEdgeTrustAuth are extracted
// out of TestExternalAuthConfig_FullMapping to keep the test under the repo's
// gocyclo-15 pre-commit gate; the assertions themselves are unchanged.
func assertClientKeysAuth(t *testing.T, got *omniav1alpha1.ClientKeysAuth) {
	t.Helper()
	if got == nil || got.DefaultRole != "editor" || !got.TrustEndUserHeader {
		t.Errorf("clientKeys = %+v", got)
	}
}

func assertOIDCAuth(t *testing.T, got *omniav1alpha1.OIDCAuth) {
	t.Helper()
	if got == nil || got.Issuer != "https://issuer.example.com" || got.Audience != "aud-1" {
		t.Fatalf("oidc = %+v", got)
	}
	if got.ClaimMapping == nil || got.ClaimMapping.Subject != "sub-claim" || got.ClaimMapping.EndUser != "enduser-claim" {
		t.Errorf("oidc.claimMapping = %+v", got.ClaimMapping)
	}
}

func assertEdgeTrustAuth(t *testing.T, got *omniav1alpha1.EdgeTrustAuth) {
	t.Helper()
	if got == nil {
		t.Fatal("edgeTrust = nil")
	}
	hm := got.HeaderMapping
	if hm == nil || hm.Subject != "x-subject" || hm.EndUser != "x-end-user" || hm.Email != "x-email" {
		t.Errorf("edgeTrust.headerMapping = %+v", hm)
	}
	if got.ClaimsFromHeaders["x-user-groups"] != "groups" {
		t.Errorf("edgeTrust.claimsFromHeaders = %+v", got.ClaimsFromHeaders)
	}
}

func TestExternalAuthConfig_FullMapping(t *testing.T) {
	in := &ExternalAuthIntent{
		ClientKeys: &ClientKeysIntent{DefaultRole: "editor", TrustEndUserHeader: true},
		OIDC: &OIDCIntent{
			Issuer:   "https://issuer.example.com",
			Audience: "aud-1",
			ClaimMapping: &OIDCMappingIntent{
				Subject: "sub-claim",
				EndUser: "enduser-claim",
			},
		},
		EdgeTrust: &EdgeTrustIntent{
			HeaderMapping: &EdgeTrustHeaderIntent{
				Subject: "x-subject",
				EndUser: "x-end-user",
				Email:   "x-email",
			},
			ClaimsFromHeaders: map[string]string{"x-user-groups": "groups"},
		},
	}

	got := externalAuthConfig(in)
	if got == nil {
		t.Fatal("externalAuthConfig(in) = nil, want non-nil")
	}
	assertClientKeysAuth(t, got.ClientKeys)
	assertOIDCAuth(t, got.OIDC)
	assertEdgeTrustAuth(t, got.EdgeTrust)
}

func TestExternalAuthConfig_PartialNilSubStructs(t *testing.T) {
	in := &ExternalAuthIntent{
		OIDC: &OIDCIntent{Issuer: "https://issuer.example.com", Audience: "aud-1"},
	}
	got := externalAuthConfig(in)
	if got == nil {
		t.Fatal("externalAuthConfig(in) = nil, want non-nil")
	}
	if got.ClientKeys != nil {
		t.Errorf("clientKeys = %+v, want nil", got.ClientKeys)
	}
	if got.OIDC == nil || got.OIDC.ClaimMapping != nil {
		t.Errorf("oidc.claimMapping = %+v, want nil", got.OIDC.ClaimMapping)
	}
	if got.EdgeTrust != nil {
		t.Errorf("edgeTrust = %+v, want nil", got.EdgeTrust)
	}
}

// TestExternalAuthConfig_EdgeTrustWithoutHeaderMapping covers the case where
// EdgeTrust is configured (e.g. only claimsFromHeaders) but headerMapping is
// left unset — headerMapping must stay nil rather than becoming an empty struct.
func TestExternalAuthConfig_EdgeTrustWithoutHeaderMapping(t *testing.T) {
	in := &ExternalAuthIntent{
		EdgeTrust: &EdgeTrustIntent{
			ClaimsFromHeaders: map[string]string{"x-user-groups": "groups"},
		},
	}
	got := externalAuthConfig(in)
	if got == nil || got.EdgeTrust == nil {
		t.Fatal("externalAuthConfig(in).EdgeTrust = nil, want non-nil")
	}
	if got.EdgeTrust.HeaderMapping != nil {
		t.Errorf("edgeTrust.headerMapping = %+v, want nil", got.EdgeTrust.HeaderMapping)
	}
	if got.EdgeTrust.ClaimsFromHeaders["x-user-groups"] != "groups" {
		t.Errorf("edgeTrust.claimsFromHeaders = %+v", got.EdgeTrust.ClaimsFromHeaders)
	}
}

func TestAgentToAgentRuntime_ExternalAuthWired(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		ExternalAuth: &ExternalAuthIntent{
			ClientKeys: &ClientKeysIntent{DefaultRole: "viewer"},
		},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.ExternalAuth == nil || ar.Spec.ExternalAuth.ClientKeys == nil || ar.Spec.ExternalAuth.ClientKeys.DefaultRole != "viewer" {
		t.Errorf("externalAuth = %+v", ar.Spec.ExternalAuth)
	}
}

func TestAgentToAgentRuntime_ExternalAuthNilWhenUnset(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.ExternalAuth != nil {
		t.Errorf("externalAuth = %+v, want nil", ar.Spec.ExternalAuth)
	}
}

func TestMemoryConfig_Nil(t *testing.T) {
	if got := memoryConfig(nil); got != nil {
		t.Errorf("memoryConfig(nil) = %+v, want nil", got)
	}
}

func TestMemoryConfig_FullMapping(t *testing.T) {
	retrievalEnabled := false
	toolsEnabled := false
	limit := int32(5)
	in := &MemoryIntent{
		Enabled: true,
		Retrieval: &MemoryRetrievalIntent{
			Enabled:  &retrievalEnabled,
			Strategy: "semantic",
			Limit:    &limit,
			DenyCEL:  `has(memory.tags) && memory.tags.exists(t, t == "secret")`,
		},
		Tools: &MemoryToolsIntent{Enabled: &toolsEnabled},
	}

	got := memoryConfig(in)
	if got == nil {
		t.Fatal("memoryConfig(in) = nil, want non-nil")
	}
	if !got.Enabled {
		t.Errorf("enabled = %v, want true", got.Enabled)
	}
	if got.Retrieval == nil {
		t.Fatal("retrieval = nil, want non-nil")
	}
	if got.Retrieval.Enabled == nil || *got.Retrieval.Enabled != false {
		t.Errorf("retrieval.enabled = %v, want false", got.Retrieval.Enabled)
	}
	if got.Retrieval.Strategy != "semantic" {
		t.Errorf("retrieval.strategy = %q, want semantic", got.Retrieval.Strategy)
	}
	if got.Retrieval.Limit == nil || *got.Retrieval.Limit != 5 {
		t.Errorf("retrieval.limit = %v, want 5", got.Retrieval.Limit)
	}
	if got.Retrieval.AccessFilter == nil || got.Retrieval.AccessFilter.DenyCEL != in.Retrieval.DenyCEL {
		t.Errorf("retrieval.accessFilter = %+v, want denyCEL %q", got.Retrieval.AccessFilter, in.Retrieval.DenyCEL)
	}
	if got.Tools == nil || got.Tools.Enabled == nil || *got.Tools.Enabled != false {
		t.Errorf("tools = %+v, want enabled=false", got.Tools)
	}
}

func TestMemoryConfig_DenyCELEmptyOmitsAccessFilter(t *testing.T) {
	in := &MemoryIntent{
		Enabled:   true,
		Retrieval: &MemoryRetrievalIntent{Strategy: "keyword"},
	}
	got := memoryConfig(in)
	if got == nil || got.Retrieval == nil {
		t.Fatal("memoryConfig(in).Retrieval = nil, want non-nil")
	}
	if got.Retrieval.AccessFilter != nil {
		t.Errorf("accessFilter = %+v, want nil (denyCEL empty)", got.Retrieval.AccessFilter)
	}
}

func TestMemoryConfig_NoRetrievalOrTools(t *testing.T) {
	in := &MemoryIntent{Enabled: true}
	got := memoryConfig(in)
	if got == nil {
		t.Fatal("memoryConfig(in) = nil, want non-nil")
	}
	if !got.Enabled {
		t.Errorf("enabled = %v, want true", got.Enabled)
	}
	if got.Retrieval != nil {
		t.Errorf("retrieval = %+v, want nil", got.Retrieval)
	}
	if got.Tools != nil {
		t.Errorf("tools = %+v, want nil", got.Tools)
	}
}

func TestEvalConfig_Nil(t *testing.T) {
	if got := evalConfig(nil); got != nil {
		t.Errorf("evalConfig(nil) = %+v, want nil", got)
	}
}

func TestEvalConfig_FullMapping(t *testing.T) {
	in := &EvalsIntent{
		Enabled: true,
		Inline:  []string{"safety", "quality"},
		Worker:  []string{"regression"},
	}
	got := evalConfig(in)
	if got == nil {
		t.Fatal("evalConfig(in) = nil, want non-nil")
	}
	if !got.Enabled {
		t.Errorf("enabled = %v, want true", got.Enabled)
	}
	if got.Inline == nil || len(got.Inline.Groups) != 2 || got.Inline.Groups[0] != "safety" || got.Inline.Groups[1] != "quality" {
		t.Errorf("inline = %+v", got.Inline)
	}
	if got.Worker == nil || len(got.Worker.Groups) != 1 || got.Worker.Groups[0] != "regression" {
		t.Errorf("worker = %+v", got.Worker)
	}
	if got.Sampling != nil || got.RateLimit != nil || got.SessionCompletion != nil || got.PodOverrides != nil {
		t.Errorf("unmapped fields must stay nil: sampling=%+v rateLimit=%+v sessionCompletion=%+v podOverrides=%+v",
			got.Sampling, got.RateLimit, got.SessionCompletion, got.PodOverrides)
	}
}

func TestEvalConfig_EmptyGroupsOmitPaths(t *testing.T) {
	in := &EvalsIntent{Enabled: true}
	got := evalConfig(in)
	if got == nil {
		t.Fatal("evalConfig(in) = nil, want non-nil")
	}
	if got.Inline != nil {
		t.Errorf("inline = %+v, want nil (no groups)", got.Inline)
	}
	if got.Worker != nil {
		t.Errorf("worker = %+v, want nil (no groups)", got.Worker)
	}
}

func TestAgentToAgentRuntime_MemoryAndEvalsWired(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Memory: &MemoryIntent{
			Enabled:   true,
			Retrieval: &MemoryRetrievalIntent{Strategy: "composite"},
		},
		Evals: &EvalsIntent{Enabled: true, Inline: []string{"safety"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.Memory == nil || !ar.Spec.Memory.Enabled || ar.Spec.Memory.Retrieval == nil || ar.Spec.Memory.Retrieval.Strategy != "composite" {
		t.Errorf("memory = %+v", ar.Spec.Memory)
	}
	if ar.Spec.Evals == nil || !ar.Spec.Evals.Enabled || ar.Spec.Evals.Inline == nil || len(ar.Spec.Evals.Inline.Groups) != 1 || ar.Spec.Evals.Inline.Groups[0] != "safety" {
		t.Errorf("evals = %+v", ar.Spec.Evals)
	}
}

func TestAgentToAgentRuntime_MemoryAndEvalsNilWhenUnset(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.Memory != nil {
		t.Errorf("memory = %+v, want nil", ar.Spec.Memory)
	}
	if ar.Spec.Evals != nil {
		t.Errorf("evals = %+v, want nil", ar.Spec.Evals)
	}
}

func TestAgentToAgentRuntime_RolloutStepsWithPause(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	weight := int32(50)
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Rollout: &RolloutIntent{
			Steps: []RolloutStepIntent{
				{SetWeight: &weight, PauseDuration: "5m"},
				{SetWeight: &weight},
			},
		},
	}
	ar := agentToAgentRuntime("ns", pack, agent, "", nil)
	if ar.Spec.Rollout == nil || len(ar.Spec.Rollout.Steps) != 2 {
		t.Fatalf("rollout steps = %+v", ar.Spec.Rollout)
	}
	step0 := ar.Spec.Rollout.Steps[0]
	if step0.SetWeight == nil || *step0.SetWeight != 50 {
		t.Errorf("step0.setWeight = %v", step0.SetWeight)
	}
	if step0.Pause == nil || step0.Pause.Duration == nil || *step0.Pause.Duration != "5m" {
		t.Errorf("step0.pause = %+v", step0.Pause)
	}
	step1 := ar.Spec.Rollout.Steps[1]
	if step1.Pause != nil {
		t.Errorf("step1.pause = %+v, want nil (no pauseDuration)", step1.Pause)
	}
}

func TestAgentPolicy_ToolAccessDenylistShape(t *testing.T) {
	pack := PackIntent{Name: "support"}
	policy := &PolicyIntent{ToolBlocklist: []string{"delete-account", "wipe-db"}}
	agents := []string{"support-agent", "escalation-agent"}

	ap := agentPolicy("ns", pack, policy, agents, toolRegistryName("support"), map[string]string{"env": "prod"})
	if ap == nil {
		t.Fatal("agentPolicy = nil, want non-nil")
	}
	assertAgentPolicyMetadata(t, ap)
	assertAgentPolicySpec(t, ap, agents, policy.ToolBlocklist)
}

// assertAgentPolicyMetadata checks the name/namespace/labels of an AgentPolicy
// built by agentPolicy, split out of TestAgentPolicy_ToolAccessDenylistShape to
// keep that test's cyclomatic complexity under the SonarCloud threshold.
func assertAgentPolicyMetadata(t *testing.T, ap *omniav1alpha1.AgentPolicy) {
	t.Helper()
	if ap.Name != "support-policy" {
		t.Errorf("name = %q, want support-policy", ap.Name)
	}
	if ap.Namespace != "ns" {
		t.Errorf("namespace = %q, want ns", ap.Namespace)
	}
	if ap.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, ap.Labels[packselect.Label])
	}
	if ap.Labels["env"] != "prod" {
		t.Errorf("deploy label not propagated: %v", ap.Labels)
	}
}

// assertAgentPolicySpec checks the mode/onFailure/selector/toolAccess of an
// AgentPolicy built by agentPolicy, split out of
// TestAgentPolicy_ToolAccessDenylistShape to keep that test's cyclomatic
// complexity under the SonarCloud threshold.
func assertAgentPolicySpec(t *testing.T, ap *omniav1alpha1.AgentPolicy, wantAgents, wantTools []string) {
	t.Helper()
	if ap.Spec.Mode != omniav1alpha1.AgentPolicyModeEnforce {
		t.Errorf("mode = %q, want enforce", ap.Spec.Mode)
	}
	if ap.Spec.OnFailure != omniav1alpha1.OnFailureDeny {
		t.Errorf("onFailure = %q, want deny", ap.Spec.OnFailure)
	}
	if ap.Spec.Selector == nil || len(ap.Spec.Selector.Agents) != 2 ||
		ap.Spec.Selector.Agents[0] != wantAgents[0] || ap.Spec.Selector.Agents[1] != wantAgents[1] {
		t.Errorf("selector.agents = %+v, want %v", ap.Spec.Selector, wantAgents)
	}
	if ap.Spec.ToolAccess == nil {
		t.Fatal("toolAccess = nil, want non-nil")
	}
	if ap.Spec.ToolAccess.Mode != omniav1alpha1.ToolAccessModeDenylist {
		t.Errorf("toolAccess.mode = %q, want denylist", ap.Spec.ToolAccess.Mode)
	}
	if len(ap.Spec.ToolAccess.Rules) != 1 {
		t.Fatalf("toolAccess.rules = %+v, want 1 rule", ap.Spec.ToolAccess.Rules)
	}
	rule := ap.Spec.ToolAccess.Rules[0]
	if rule.Registry != toolRegistryName("support") {
		t.Errorf("rule.registry = %q, want %q", rule.Registry, toolRegistryName("support"))
	}
	if len(rule.Tools) != 2 || rule.Tools[0] != wantTools[0] || rule.Tools[1] != wantTools[1] {
		t.Errorf("rule.tools = %v, want %v", rule.Tools, wantTools)
	}
}

// TestAgentPolicy_NilGuards verifies agentPolicy returns nil for each
// individual guard condition: nil policy, empty blocklist, and no registry
// name to attach the denylist rule to (the last case mirrors what Validate
// already rejects at the HTTP layer — asserted here as a translate-level
// belt-and-suspenders guard).
func TestAgentPolicy_NilGuards(t *testing.T) {
	pack := PackIntent{Name: "support"}
	agents := []string{"support-agent"}
	registry := toolRegistryName("support")

	if ap := agentPolicy("ns", pack, nil, agents, registry, nil); ap != nil {
		t.Errorf("nil policy: got %+v, want nil", ap)
	}
	if ap := agentPolicy("ns", pack, &PolicyIntent{}, agents, registry, nil); ap != nil {
		t.Errorf("empty blocklist: got %+v, want nil", ap)
	}
	policy := &PolicyIntent{ToolBlocklist: []string{"delete-account"}}
	if ap := agentPolicy("ns", pack, policy, agents, "", nil); ap != nil {
		t.Errorf("no registry: got %+v, want nil", ap)
	}
}

func TestToolRegistry_NilOrRefOnlyCreatesNothing(t *testing.T) {
	pack := PackIntent{Name: "support"}

	if tr, err := toolRegistry("ns", pack, nil, nil); err != nil || tr != nil {
		t.Errorf("nil tools: got (%+v, %v), want (nil, nil)", tr, err)
	}
	refOnly := &ToolsIntent{Ref: "existing-registry"}
	if tr, err := toolRegistry("ns", pack, refOnly, nil); err != nil || tr != nil {
		t.Errorf("ref-only tools: got (%+v, %v), want (nil, nil)", tr, err)
	}
	empty := &ToolsIntent{}
	if tr, err := toolRegistry("ns", pack, empty, nil); err != nil || tr != nil {
		t.Errorf("empty tools: got (%+v, %v), want (nil, nil)", tr, err)
	}
}

// toolRegistryTestHandlers returns the three-handler fixture shared by the
// ToolRegistry handler tests below: a client handler (consent + timeout), an
// http handler (tool + httpConfig), and a grpc handler carrying every other
// raw config block (grpcConfig/openAPIConfig/mcpConfig/auth) purely to
// exercise each unmarshal branch — not a realistic single-handler config.
func toolRegistryTestHandlers() []HandlerIntent {
	clientCfg := json.RawMessage(`{"consentMessage":"allow?","categories":["location"]}`)
	httpCfg := json.RawMessage(`{"endpoint":"https://api.example.com/tool","method":"POST"}`)
	toolDef := json.RawMessage(`{"name":"lookup","description":"desc","inputSchema":{}}`)
	grpcCfg := json.RawMessage(`{"endpoint":"grpc.example.com:443","tls":true}`)
	openAPICfg := json.RawMessage(`{"specURL":"https://api.example.com/openapi.json"}`)
	mcpCfg := json.RawMessage(`{"transport":"sse","endpoint":"https://mcp.example.com/sse"}`)
	authCfg := json.RawMessage(`{"type":"bearer","secretRef":{"name":"tool-secret","key":"token"}}`)

	return []HandlerIntent{
		{Name: "browser-consent", Type: handlerTypeClient, ClientConfig: &clientCfg, Timeout: "15s"},
		{Name: "lookup-http", Type: "http", HTTPConfig: &httpCfg, Tool: &toolDef},
		{
			Name:          "multi",
			Type:          "grpc",
			GRPCConfig:    &grpcCfg,
			OpenAPIConfig: &openAPICfg,
			MCPConfig:     &mcpCfg,
			Auth:          &authCfg,
		},
	}
}

func TestToolRegistry_Metadata(t *testing.T) {
	tools := &ToolsIntent{Handlers: toolRegistryTestHandlers()}
	pack := PackIntent{Name: "support"}

	tr, err := toolRegistry("ns", pack, tools, map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("toolRegistry: %v", err)
	}
	if tr == nil {
		t.Fatal("toolRegistry = nil, want non-nil")
	}
	if tr.Name != toolRegistryName("support") {
		t.Errorf("name = %q, want %q", tr.Name, toolRegistryName("support"))
	}
	if tr.Namespace != "ns" {
		t.Errorf("namespace = %q, want ns", tr.Namespace)
	}
	if tr.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, tr.Labels[packselect.Label])
	}
	if tr.Labels["env"] != "prod" {
		t.Errorf("deploy label not propagated: %v", tr.Labels)
	}
	if len(tr.Spec.Handlers) != 3 {
		t.Fatalf("handlers = %+v, want 3", tr.Spec.Handlers)
	}
}

func TestToolRegistry_ClientHandler(t *testing.T) {
	tools := &ToolsIntent{Handlers: toolRegistryTestHandlers()}
	tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil)
	if err != nil {
		t.Fatalf("toolRegistry: %v", err)
	}

	h0 := tr.Spec.Handlers[0]
	if h0.Name != "browser-consent" || h0.Type != omniav1alpha1.HandlerTypeClient {
		t.Errorf("name/type = %q/%q", h0.Name, h0.Type)
	}
	if h0.ClientConfig == nil || h0.ClientConfig.ConsentMessage != "allow?" ||
		len(h0.ClientConfig.Categories) != 1 || h0.ClientConfig.Categories[0] != "location" {
		t.Errorf("clientConfig = %+v", h0.ClientConfig)
	}
	if h0.Timeout == nil || h0.Timeout.Duration != 15*time.Second {
		t.Errorf("timeout = %v, want 15s", h0.Timeout)
	}
}

func TestToolRegistry_HTTPHandler(t *testing.T) {
	tools := &ToolsIntent{Handlers: toolRegistryTestHandlers()}
	tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil)
	if err != nil {
		t.Fatalf("toolRegistry: %v", err)
	}

	h1 := tr.Spec.Handlers[1]
	if h1.Name != "lookup-http" || h1.Type != omniav1alpha1.HandlerTypeHTTP {
		t.Errorf("name/type = %q/%q", h1.Name, h1.Type)
	}
	if h1.HTTPConfig == nil || h1.HTTPConfig.Endpoint != "https://api.example.com/tool" || h1.HTTPConfig.Method != "POST" {
		t.Errorf("httpConfig = %+v", h1.HTTPConfig)
	}
	if h1.Tool == nil || h1.Tool.Name != "lookup" || h1.Tool.Description != "desc" {
		t.Errorf("tool = %+v", h1.Tool)
	}
	if h1.Timeout != nil {
		t.Errorf("timeout = %v, want nil (no timeout set)", h1.Timeout)
	}
}

func TestToolRegistry_MultiConfigHandler(t *testing.T) {
	tools := &ToolsIntent{Handlers: toolRegistryTestHandlers()}
	tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil)
	if err != nil {
		t.Fatalf("toolRegistry: %v", err)
	}

	h2 := tr.Spec.Handlers[2]
	if h2.GRPCConfig == nil || h2.GRPCConfig.Endpoint != "grpc.example.com:443" || !h2.GRPCConfig.TLS {
		t.Errorf("grpcConfig = %+v", h2.GRPCConfig)
	}
	if h2.OpenAPIConfig == nil || h2.OpenAPIConfig.SpecURL != "https://api.example.com/openapi.json" {
		t.Errorf("openAPIConfig = %+v", h2.OpenAPIConfig)
	}
	if h2.MCPConfig == nil || h2.MCPConfig.Transport != omniav1alpha1.MCPTransportSSE {
		t.Errorf("mcpConfig = %+v", h2.MCPConfig)
	}
	if h2.Auth == nil || h2.Auth.Type != "bearer" || h2.Auth.SecretRef == nil || h2.Auth.SecretRef.Name != "tool-secret" {
		t.Errorf("auth = %+v", h2.Auth)
	}
}

func TestToolRegistry_MalformedConfigReturnsError(t *testing.T) {
	badCfg := json.RawMessage(`{"endpoint": 123}`) // endpoint should be a string, not a number
	tools := &ToolsIntent{
		Handlers: []HandlerIntent{{Name: "bad", Type: "http", HTTPConfig: &badCfg}},
	}
	tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil)
	if err == nil {
		t.Fatal("expected error for malformed httpConfig, got nil")
	}
	if tr != nil {
		t.Errorf("expected nil ToolRegistry on error, got %+v", tr)
	}
}

// TestToolRegistry_MalformedConfigReturnsError_AllFields exercises the
// malformed-unmarshal error branch for every raw-JSON config field on
// HandlerIntent, not just httpConfig — each block is a near-identical
// unmarshal-or-error step in unmarshalHandlerConfigs, and each has its own
// error return to cover.
func TestToolRegistry_MalformedConfigReturnsError_AllFields(t *testing.T) {
	bad := json.RawMessage(`"not-an-object"`)
	tests := map[string]HandlerIntent{
		"tool":          {Name: "h", Type: "http", Tool: &bad},
		"openAPIConfig": {Name: "h", Type: "openapi", OpenAPIConfig: &bad},
		"grpcConfig":    {Name: "h", Type: "grpc", GRPCConfig: &bad},
		"mcpConfig":     {Name: "h", Type: "mcp", MCPConfig: &bad},
		"clientConfig":  {Name: "h", Type: handlerTypeClient, ClientConfig: &bad},
		"auth":          {Name: "h", Type: "http", Auth: &bad},
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			tools := &ToolsIntent{Handlers: []HandlerIntent{h}}
			tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil)
			if err == nil {
				t.Fatalf("expected error for malformed %s, got nil", name)
			}
			if tr != nil {
				t.Errorf("expected nil ToolRegistry on error, got %+v", tr)
			}
		})
	}
}

func TestToolRegistry_InvalidTimeoutReturnsError(t *testing.T) {
	tools := &ToolsIntent{Handlers: []HandlerIntent{{Name: "h", Type: handlerTypeClient, Timeout: "not-a-duration"}}}
	if tr, err := toolRegistry("ns", PackIntent{Name: "support"}, tools, nil); err == nil {
		t.Fatal("expected error for invalid timeout")
	} else if tr != nil {
		t.Errorf("expected nil ToolRegistry on error, got %+v", tr)
	}
}
