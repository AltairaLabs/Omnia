package deploy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

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
	ar := agentToAgentRuntime("ns", pack, agent, nil)

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

func TestAgentToAgentRuntime_TriggerRollout(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0"}
	agent := AgentIntent{
		Name:      "support",
		Providers: []ProviderBind{{Name: "default", Ref: "claude"}},
		Rollout:   &RolloutIntent{Trigger: &RolloutTriggerIntent{PromptPackChannel: "stable"}},
	}
	ar := agentToAgentRuntime("ns", pack, agent, nil)
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
	ar := agentToAgentRuntime("ns", pack, agent, nil)

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
	ar := agentToAgentRuntime("ns", pack, agent, nil)

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
	ar := agentToAgentRuntime("ns", pack, agent, nil)
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
	ar := agentToAgentRuntime("ns", pack, agent, nil)
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
	ar := agentToAgentRuntime("ns", pack, agent, nil)
	if ar.Spec.ExternalAuth != nil {
		t.Errorf("externalAuth = %+v, want nil", ar.Spec.ExternalAuth)
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
	ar := agentToAgentRuntime("ns", pack, agent, nil)
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
