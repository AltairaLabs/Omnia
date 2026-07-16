package deploy

import (
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
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

// ExternalAuthIntent maps to spec.externalAuth (AgentExternalAuth). Current CRD
// vocabulary — the server maps to whatever shape the CRD has.
type ExternalAuthIntent struct {
	ClientKeys *ClientKeysIntent `json:"clientKeys,omitempty"`
	OIDC       *OIDCIntent       `json:"oidc,omitempty"`
	EdgeTrust  *EdgeTrustIntent  `json:"edgeTrust,omitempty"`
}

// ClientKeysIntent maps to AgentExternalAuth.clientKeys.
type ClientKeysIntent struct {
	DefaultRole        string `json:"defaultRole,omitempty"`
	TrustEndUserHeader bool   `json:"trustEndUserHeader,omitempty"`
}

// OIDCIntent maps to AgentExternalAuth.oidc.
type OIDCIntent struct {
	Issuer       string             `json:"issuer"`
	Audience     string             `json:"audience"`
	ClaimMapping *OIDCMappingIntent `json:"claimMapping,omitempty"`
}

// OIDCMappingIntent maps to AgentExternalAuth.oidc.claimMapping.
type OIDCMappingIntent struct {
	Subject string `json:"subject,omitempty"`
	EndUser string `json:"endUser,omitempty"`
}

// EdgeTrustIntent maps to AgentExternalAuth.edgeTrust.
type EdgeTrustIntent struct {
	HeaderMapping     *EdgeTrustHeaderIntent `json:"headerMapping,omitempty"`
	ClaimsFromHeaders map[string]string      `json:"claimsFromHeaders,omitempty"`
}

// EdgeTrustHeaderIntent maps to AgentExternalAuth.edgeTrust.headerMapping.
type EdgeTrustHeaderIntent struct {
	Subject string `json:"subject,omitempty"`
	EndUser string `json:"endUser,omitempty"`
	Email   string `json:"email,omitempty"`
}

// MemoryIntent maps to spec.memory (MemoryConfig).
type MemoryIntent struct {
	Enabled   bool                   `json:"enabled,omitempty"`
	Retrieval *MemoryRetrievalIntent `json:"retrieval,omitempty"`
	Tools     *MemoryToolsIntent     `json:"tools,omitempty"`
}

// MemoryRetrievalIntent maps to spec.memory.retrieval.
type MemoryRetrievalIntent struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	Strategy string `json:"strategy,omitempty"` // keyword|semantic|composite
	Limit    *int32 `json:"limit,omitempty"`
	DenyCEL  string `json:"denyCEL,omitempty"`
}

// MemoryToolsIntent maps to spec.memory.tools.
type MemoryToolsIntent struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// EvalsIntent maps to spec.evals (EvalConfig).
type EvalsIntent struct {
	Enabled bool     `json:"enabled,omitempty"`
	Inline  []string `json:"inlineGroups,omitempty"`
	Worker  []string `json:"workerGroups,omitempty"`
}

// ToolsIntent: reference an existing registry OR create one (create-only).
type ToolsIntent struct {
	Ref      string          `json:"ref,omitempty"`      // existing registry name
	Handlers []HandlerIntent `json:"handlers,omitempty"` // create-only registry contents
}

// HandlerIntent mirrors HandlerDefinition's stable surface. The per-executor
// config blocks are carried as raw JSON so the intent tracks CRD growth without
// re-typing every executor (mapped straight onto the CRD's json fields).
type HandlerIntent struct {
	Name          string           `json:"name"`
	Type          string           `json:"type"` // http|openapi|grpc|mcp|client
	Tool          *json.RawMessage `json:"tool,omitempty"`
	HTTPConfig    *json.RawMessage `json:"httpConfig,omitempty"`
	OpenAPIConfig *json.RawMessage `json:"openAPIConfig,omitempty"`
	GRPCConfig    *json.RawMessage `json:"grpcConfig,omitempty"`
	MCPConfig     *json.RawMessage `json:"mcpConfig,omitempty"`
	ClientConfig  *json.RawMessage `json:"clientConfig,omitempty"`
	Auth          *json.RawMessage `json:"auth,omitempty"`
	Timeout       string           `json:"timeout,omitempty"`
}

// PolicyIntent maps to an AgentPolicy denylist (NOT a toolBlocklist field —
// see the shape-correction note). toolBlocklist is the flat list of tool names
// to deny; the server builds toolAccess{denylist, rules:[{registry, tools}]}.
type PolicyIntent struct {
	ToolBlocklist []string `json:"toolBlocklist,omitempty"`
}

// validHandlerTypes is the allowed set of HandlerIntent.Type values, built
// from the ToolRegistry CRD's handler executor kind constants so there is a
// single source of truth for the recognized types.
var validHandlerTypes = map[string]bool{
	string(omniav1alpha1.HandlerTypeHTTP):    true,
	string(omniav1alpha1.HandlerTypeOpenAPI): true,
	string(omniav1alpha1.HandlerTypeGRPC):    true,
	string(omniav1alpha1.HandlerTypeMCP):     true,
	string(omniav1alpha1.HandlerTypeClient):  true,
}

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
	if d.Pack.Content == "" {
		return errors.New("pack.content is required")
	}
	if len(d.Agents) == 0 {
		return errors.New("at least one agent is required")
	}
	if err := d.Tools.validate(); err != nil {
		return err
	}
	if err := d.Policy.validate(d.Tools); err != nil {
		return err
	}
	for i, a := range d.Agents {
		if err := a.validate(i); err != nil {
			return err
		}
	}
	return nil
}

// validate checks ToolsIntent, when set: ref and handlers are mutually
// exclusive (one references an existing registry, the other creates one),
// and each handler has a non-empty name/type drawn from the allowed set.
func (t *ToolsIntent) validate() error {
	if t == nil {
		return nil
	}
	if t.Ref != "" && len(t.Handlers) > 0 {
		return errors.New("tools: ref and handlers are mutually exclusive")
	}
	for i, h := range t.Handlers {
		if h.Name == "" {
			return fmt.Errorf("tools.handlers[%d].name is required", i)
		}
		if h.Type == "" {
			return fmt.Errorf("tools.handlers[%d].type is required", i)
		}
		if !validHandlerTypes[h.Type] {
			return fmt.Errorf("tools.handlers[%d].type: invalid type %q", i, h.Type)
		}
	}
	return nil
}

// validate checks PolicyIntent, when set: a non-empty toolBlocklist requires
// tools (ref or handlers) so the server has a registry to build the
// AgentPolicy denylist rule against.
func (p *PolicyIntent) validate(tools *ToolsIntent) error {
	if p == nil || len(p.ToolBlocklist) == 0 {
		return nil
	}
	if tools == nil || (tools.Ref == "" && len(tools.Handlers) == 0) {
		return errors.New("policy.toolBlocklist requires tools.ref or tools.handlers to reference a registry")
	}
	return nil
}

// validate checks one AgentIntent at index i in the parent Agents slice.
func (a AgentIntent) validate(i int) error {
	if a.Name == "" {
		return fmt.Errorf("agents[%d].name is required", i)
	}
	if len(a.Providers) == 0 {
		return fmt.Errorf("agents[%d].providers must not be empty", i)
	}
	for j, p := range a.Providers {
		if p.Name == "" {
			return fmt.Errorf("agents[%d].providers[%d].name is required", i, j)
		}
		if p.Ref == "" {
			return fmt.Errorf("agents[%d].providers[%d].ref is required", i, j)
		}
	}
	if err := a.Rollout.validate(i); err != nil {
		return err
	}
	if err := a.ExternalAuth.validate(i); err != nil {
		return err
	}
	if err := a.Memory.validate(i); err != nil {
		return err
	}
	return a.Runtime.validate(i)
}

// validate checks ExternalAuthIntent, when set: an OIDC block requires a
// non-empty issuer and audience (mirrors the CRD's required OIDC fields).
func (e *ExternalAuthIntent) validate(i int) error {
	if e == nil || e.OIDC == nil {
		return nil
	}
	if e.OIDC.Issuer == "" {
		return fmt.Errorf("agents[%d].externalAuth.oidc.issuer is required", i)
	}
	if e.OIDC.Audience == "" {
		return fmt.Errorf("agents[%d].externalAuth.oidc.audience is required", i)
	}
	return nil
}

// Memory retrieval strategy values recognized by MemoryIntent.validate. The
// CRD's MemoryRetrievalConfig.Strategy is a plain string (no typed enum), so
// these are local constants rather than reused CRD constants.
const (
	memoryStrategyKeyword   = "keyword"
	memoryStrategySemantic  = "semantic"
	memoryStrategyComposite = "composite"
)

// validate checks MemoryIntent, when set: a non-empty retrieval strategy must
// be one of the recognized strategies.
func (m *MemoryIntent) validate(i int) error {
	if m == nil || m.Retrieval == nil || m.Retrieval.Strategy == "" {
		return nil
	}
	switch m.Retrieval.Strategy {
	case memoryStrategyKeyword, memoryStrategySemantic, memoryStrategyComposite:
		return nil
	default:
		return fmt.Errorf("agents[%d].memory.retrieval.strategy: invalid strategy %q", i, m.Retrieval.Strategy)
	}
}

// validate checks the RolloutIntent, when set: the CRD requires
// spec.rollout.steps to be non-empty (MinItems=1) — an intent that specifies
// a rollout block but no steps would translate to a CRD-invalid AgentRuntime.
// When a trigger is set, its PromptPackChannel is required (mirrors the CRD's
// RolloutTrigger.PromptPackChannel required field).
func (r *RolloutIntent) validate(i int) error {
	if r == nil {
		return nil
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("agents[%d].rollout.steps must not be empty", i)
	}
	if r.Trigger != nil && r.Trigger.PromptPackChannel == "" {
		return fmt.Errorf("agents[%d].rollout.trigger.promptPackChannel is required", i)
	}
	return nil
}

// validate checks the CPU/Memory quantities, when set, parse cleanly.
// resourceRequirements (translate.go) uses resource.MustParse on these same
// fields, so a malformed quantity must be rejected here (400) rather than
// panicking during translation (500).
func (r *RuntimeIntent) validate(i int) error {
	if r == nil {
		return nil
	}
	if r.CPU != "" {
		if _, err := resource.ParseQuantity(r.CPU); err != nil {
			return fmt.Errorf("agents[%d].runtime.cpu: invalid quantity %q: %w", i, r.CPU, err)
		}
	}
	if r.Memory != "" {
		if _, err := resource.ParseQuantity(r.Memory); err != nil {
			return fmt.Errorf("agents[%d].runtime.memory: invalid quantity %q: %w", i, r.Memory, err)
		}
	}
	return nil
}
