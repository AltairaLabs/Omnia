package deploy

import (
	"errors"
	"fmt"
)

// APIVersionV1 is the only DeployIntent contract version this operator serves.
// An unknown apiVersion is rejected with 400 so the adapter can fall back.
const APIVersionV1 = "deploy.omnia.altairalabs.ai/v1"

// DeployIntent is a versioned, CRD-agnostic description of a whole deploy. The
// server translates it into real v1alpha1 objects; the adapter never constructs
// CRDs. Agents are already fanned-out and name-sanitized by the adapter.
type DeployIntent struct {
	APIVersion string            `json:"apiVersion"`
	Pack       PackIntent        `json:"pack"`
	Tools      *ToolsIntent      `json:"tools,omitempty"`  // Plan B
	Policy     *PolicyIntent     `json:"policy,omitempty"` // Plan B
	Agents     []AgentIntent     `json:"agents"`
	Labels     map[string]string `json:"labels,omitempty"`
	DryRun     bool              `json:"dryRun,omitempty"`
}

// PackIntent maps to a PromptPack + its content ConfigMap. Content is the raw
// pack.json; the operator treats it as opaque bytes.
type PackIntent struct {
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	Content      string              `json:"content"`
	Skills       []SkillRefIntent    `json:"skills,omitempty"`
	SkillsConfig *SkillsConfigIntent `json:"skillsConfig,omitempty"`
}

// AgentIntent maps to one AgentRuntime.
type AgentIntent struct {
	Name         string              `json:"name"`
	PromptName   string              `json:"promptName,omitempty"`
	Providers    []ProviderBind      `json:"providers"`
	Runtime      *RuntimeIntent      `json:"runtime,omitempty"`
	Facades      []FacadeIntent      `json:"facades,omitempty"`
	UseTools     bool                `json:"useTools,omitempty"`
	ExternalAuth *ExternalAuthIntent `json:"externalAuth,omitempty"` // Plan B
	Memory       *MemoryIntent       `json:"memory,omitempty"`       // Plan B
	Evals        *EvalsIntent        `json:"evals,omitempty"`        // Plan B
	Rollout      *RolloutIntent      `json:"rollout,omitempty"`
}

// ProviderBind is one provider binding: a logical slot name, the Provider CRD
// name, and the role (default "llm").
type ProviderBind struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
	Role string `json:"role,omitempty"`
}

// RuntimeIntent maps to spec.runtime (Plan A: replicas + resources only).
type RuntimeIntent struct {
	Replicas *int32 `json:"replicas,omitempty"`
	CPU      string `json:"cpu,omitempty"`
	Memory   string `json:"memory,omitempty"`
}

// FacadeIntent maps to one spec.facades[] entry. Plan A supports the websocket
// runtime facade shape; a nil/empty Agents.Facades yields a single default.
type FacadeIntent struct {
	Type            string `json:"type"`
	ManagementPlane *bool  `json:"managementPlane,omitempty"`
}

// RolloutIntent maps to spec.rollout. Trigger set => canary mode.
type RolloutIntent struct {
	Trigger *RolloutTriggerIntent `json:"trigger,omitempty"`
	Steps   []RolloutStepIntent   `json:"steps,omitempty"`
}

// RolloutTriggerIntent maps to spec.rollout.trigger.
type RolloutTriggerIntent struct {
	PromptPackChannel string `json:"promptPackChannel"`
}

// RolloutStepIntent maps to a spec.rollout.steps[] entry (Plan A: setWeight +
// pause duration).
type RolloutStepIntent struct {
	SetWeight     *int32 `json:"setWeight,omitempty"`
	PauseDuration string `json:"pauseDuration,omitempty"`
}

// SkillRefIntent maps to a PromptPack spec.skills[] entry.
type SkillRefIntent struct {
	Source  string   `json:"source"`
	Include []string `json:"include,omitempty"`
	MountAs string   `json:"mountAs,omitempty"`
}

// SkillsConfigIntent maps to PromptPack spec.skillsConfig.
type SkillsConfigIntent struct {
	MaxActive *int32 `json:"maxActive,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

// Plan B placeholders — declared so the wire contract is stable, mapped later.
type ToolsIntent struct {
	Ref string `json:"ref,omitempty"`
}
type PolicyIntent struct {
	ToolBlocklist []string `json:"toolBlocklist,omitempty"`
}
type ExternalAuthIntent struct{}
type MemoryIntent struct{}
type EvalsIntent struct{}

// Resource action outcomes reported per applied object.
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionUnchanged = "unchanged"
	ActionFailed    = "failed"
)

// ResourceResult is the per-object apply outcome.
type ResourceResult struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
	Error  string `json:"error,omitempty"`
}

// DeployResult is the endpoint response: best-effort per-resource status.
type DeployResult struct {
	Succeeded bool             `json:"succeeded"`
	Results   []ResourceResult `json:"results"`
}

// Validate performs structural validation only ("reject only what the CRDs
// would reject"). Field-level CRD validation stays with the apiserver.
func (d DeployIntent) Validate() error {
	if d.APIVersion != APIVersionV1 {
		return fmt.Errorf("unsupported apiVersion %q (want %q)", d.APIVersion, APIVersionV1)
	}
	if d.Pack.Name == "" {
		return errors.New("pack.name is required")
	}
	if d.Pack.Version == "" {
		return errors.New("pack.version is required")
	}
	if len(d.Agents) == 0 {
		return errors.New("at least one agent is required")
	}
	for i, a := range d.Agents {
		if a.Name == "" {
			return fmt.Errorf("agents[%d].name is required", i)
		}
		if len(a.Providers) == 0 {
			return fmt.Errorf("agents[%d].providers must not be empty", i)
		}
	}
	return nil
}
