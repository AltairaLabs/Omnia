package deploy

import (
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
// trigger-mode agent keeps its live pin instead.
func agentToAgentRuntime(namespace string, pack PackIntent, agent AgentIntent, deployLabels map[string]string) *omniav1alpha1.AgentRuntime {
	version := pack.Version
	spec := omniav1alpha1.AgentRuntimeSpec{
		PromptPackRef: omniav1alpha1.PromptPackRef{Name: pack.Name, Version: &version},
		Facades:       facadeConfigs(agent.Facades),
		Providers:     providerRefs(agent.Providers),
		Runtime:       runtimeConfig(agent),
		Rollout:       rolloutConfig(agent.Rollout),
		ExternalAuth:  externalAuthConfig(agent.ExternalAuth),
	}
	if agent.UseTools {
		spec.ToolRegistryRef = &omniav1alpha1.ToolRegistryRef{Name: toolRegistryName(pack.Name)}
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
// the adapter's "<pack>-tools" convention). Registry contents land in Plan B.
func toolRegistryName(packName string) string { return packName + "-tools" }

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
