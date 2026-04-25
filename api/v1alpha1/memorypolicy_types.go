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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MemoryRetentionMode describes how a tier's memories are considered for
// auto-pruning. Phase 1 only validates the shape — the RetentionWorker
// rewrite in Phase 3 is what actually applies these modes.
// +kubebuilder:validation:Enum=Manual;TTL;Decay;LRU;Composite
type MemoryRetentionMode string

const (
	// MemoryRetentionModeManual leaves memories alone. Only explicit
	// memory__forget, user-initiated "forget everything", or DSAR
	// deletes the row. The expected default for the institutional tier.
	MemoryRetentionModeManual MemoryRetentionMode = "Manual"
	// MemoryRetentionModeTTL prunes rows when expires_at < now(). Default
	// TTL is applied at write time if the caller didn't set one; maxAge
	// caps any explicit value.
	MemoryRetentionModeTTL MemoryRetentionMode = "TTL"
	// MemoryRetentionModeDecay computes the per-row score using
	// ScoreFormula and soft-deletes when the score drops below minScore.
	// Uses the same formula as retrieval ranking.
	MemoryRetentionModeDecay MemoryRetentionMode = "Decay"
	// MemoryRetentionModeLRU prunes rows whose accessed_at < now - staleAfter.
	// Depends on the read-path accessed_at updates shipped in retention
	// Phase 2.
	MemoryRetentionModeLRU MemoryRetentionMode = "LRU"
	// MemoryRetentionModeComposite applies TTL, Decay, and LRU independently;
	// the first one to fire wins. Expected default for the agent and user
	// tiers.
	MemoryRetentionModeComposite MemoryRetentionMode = "Composite"
)

// ConsentRevocationAction describes what the retention worker does when
// a user revokes a consent category.
// +kubebuilder:validation:Enum=SoftDelete;HardDelete;Stop
type ConsentRevocationAction string

const (
	// ConsentRevocationSoftDelete marks matching rows forgotten=true and
	// hard-deletes after the policy's grace window. The safe default.
	ConsentRevocationSoftDelete ConsentRevocationAction = "SoftDelete"
	// ConsentRevocationHardDelete removes rows immediately in a transaction.
	ConsentRevocationHardDelete ConsentRevocationAction = "HardDelete"
	// ConsentRevocationStop leaves existing rows alone; only blocks future
	// writes in that category. Not GDPR-compliant in most jurisdictions
	// but useful for development.
	ConsentRevocationStop ConsentRevocationAction = "Stop"
)

// MemoryPolicyPhase tracks the lifecycle state the controller
// reports back on the CRD's status.
// +kubebuilder:validation:Enum=Active;Error
type MemoryPolicyPhase string

const (
	// MemoryPolicyPhaseActive means the spec validated and is
	// available to the memory-api retention worker.
	MemoryPolicyPhaseActive MemoryPolicyPhase = "Active"
	// MemoryPolicyPhaseError means the spec failed validation —
	// see status.conditions for the reason.
	MemoryPolicyPhaseError MemoryPolicyPhase = "Error"
)

// MemoryTTLConfig configures the TTL branch of retention.
type MemoryTTLConfig struct {
	// default is applied at write time when the caller doesn't set an
	// explicit expires_at. Must be a Go duration string (e.g. "180d",
	// "720h"). Empty means "no default TTL" — rows without an explicit
	// expires_at never expire via the TTL branch.
	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	Default string `json:"default,omitempty"`

	// maxAge caps any explicit expires_at so a misbehaving client can't
	// pin a row in the store for longer than the operator accepts. Empty
	// means no cap.
	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	MaxAge string `json:"maxAge,omitempty"`
}

// MemoryDecayScoreFormula mirrors the multi-tier retrieval ranking
// weights — same score that demotes a memory from retrieval eventually
// drops it below the prune threshold. One mental model, two effects.
//
// Weights are strings (e.g. "0.5") so the CRD admits decimals without
// relying on Kubernetes resource.Quantity. The controller parses them
// and enforces the 0..1 range; bad values surface as a status
// condition rather than a rejected apply.
type MemoryDecayScoreFormula struct {
	// +kubebuilder:default="0.5"
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	ConfidenceWeight string `json:"confidenceWeight,omitempty"`

	// +kubebuilder:default="0.3"
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	AccessFrequencyWeight string `json:"accessFrequencyWeight,omitempty"`

	// +kubebuilder:default="0.2"
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	RecencyWeight string `json:"recencyWeight,omitempty"`
}

// MemoryDecayConfig configures the Decay branch.
type MemoryDecayConfig struct {
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// minScore is the score below which a row is prunable. Uses the
	// 0.5/0.3/0.2 confidence/frequency/recency formula by default. The
	// controller parses the string and enforces the 0..1 range; bad
	// values surface as a status condition rather than a rejected apply.
	// +kubebuilder:default="0.2"
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	MinScore string `json:"minScore,omitempty"`

	// scoreFormula overrides the default weights. Weights should sum to
	// roughly 1.0 — the controller doesn't enforce this strictly because
	// operators sometimes want to emphasise one signal.
	// +optional
	ScoreFormula *MemoryDecayScoreFormula `json:"scoreFormula,omitempty"`

	// halfLifeDays controls how aggressively recency pulls the score down.
	// Shorter half-life = personal memories decay faster; longer = agent
	// policies stay relevant longer.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3650
	// +kubebuilder:default=90
	// +optional
	HalfLifeDays *int32 `json:"halfLifeDays,omitempty"`
}

// MemoryLRUConfig configures the LRU branch.
type MemoryLRUConfig struct {
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// staleAfter is the accessed_at age beyond which a row is prunable.
	// Requires the read-path accessed_at updates shipped in retention
	// Phase 2 — without those, every row looks stale forever.
	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +kubebuilder:default="120d"
	// +optional
	StaleAfter string `json:"staleAfter,omitempty"`
}

// MemoryTierLeafConfig is the per-category retention override. Same
// shape as MemoryTierConfig but cannot itself carry a perCategory map —
// CRD schema generation rejects arbitrarily-recursive types.
type MemoryTierLeafConfig struct {
	// +kubebuilder:default=Manual
	// +optional
	Mode MemoryRetentionMode `json:"mode,omitempty"`

	// softDeleteGraceDays is the window between soft-delete (forgotten=true)
	// and hard-delete. Applies to every mode.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3650
	// +kubebuilder:default=30
	// +optional
	SoftDeleteGraceDays *int32 `json:"softDeleteGraceDays,omitempty"`

	// +optional
	TTL *MemoryTTLConfig `json:"ttl,omitempty"`

	// +optional
	Decay *MemoryDecayConfig `json:"decay,omitempty"`

	// +optional
	LRU *MemoryLRUConfig `json:"lru,omitempty"`
}

// MemoryTierConfig is the per-tier retention config. Each tier
// (institutional / agent / user) can have a different mode and branch
// config. Missing branches mean "that branch is off for this tier".
type MemoryTierConfig struct {
	// +kubebuilder:default=Manual
	// +optional
	Mode MemoryRetentionMode `json:"mode,omitempty"`

	// softDeleteGraceDays is the window between soft-delete (forgotten=true)
	// and hard-delete. Applies to every mode.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3650
	// +kubebuilder:default=30
	// +optional
	SoftDeleteGraceDays *int32 `json:"softDeleteGraceDays,omitempty"`

	// +optional
	TTL *MemoryTTLConfig `json:"ttl,omitempty"`

	// +optional
	Decay *MemoryDecayConfig `json:"decay,omitempty"`

	// +optional
	LRU *MemoryLRUConfig `json:"lru,omitempty"`

	// perCategory lets operators override the tier policy for specific
	// consent categories (e.g. "memory:health"). Keys are the consent
	// category strings from the consent model; values are a nested
	// tier config applied only to rows carrying that category.
	// +optional
	PerCategory map[string]MemoryTierLeafConfig `json:"perCategory,omitempty"`
}

// MemoryConsentRevocationConfig describes how the retention worker
// reacts to a user toggling off a consent category. Phase 4 of the
// retention proposal wires the event subscription; Phase 1 only
// validates the shape.
type MemoryConsentRevocationConfig struct {
	// +kubebuilder:default=SoftDelete
	// +optional
	Action ConsentRevocationAction `json:"action,omitempty"`

	// graceDays is the soft-delete → hard-delete window for
	// SoftDelete action. Ignored for HardDelete and Stop.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=365
	// +kubebuilder:default=7
	// +optional
	GraceDays *int32 `json:"graceDays,omitempty"`
}

// MemorySupersessionConfig controls cleanup of rows superseded by
// temporal-summarization summaries. Gated behind enabled=false until
// the summarizer agent described in
// docs/local-backlog/2026-04-23-memory-summarization-via-agent.md is
// actually generating summaries in your deployment.
type MemorySupersessionConfig struct {
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// graceDays gives operators a window to roll back a bad summary
	// before the originals are hard-deleted.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=365
	// +kubebuilder:default=14
	// +optional
	GraceDays *int32 `json:"graceDays,omitempty"`
}

// MemoryRetentionTierSet groups the three memory tiers so operators
// can express tier-specific defaults without repeating the top-level
// spec.
type MemoryRetentionTierSet struct {
	// +optional
	Institutional *MemoryTierConfig `json:"institutional,omitempty"`

	// +optional
	Agent *MemoryTierConfig `json:"agent,omitempty"`

	// +optional
	User *MemoryTierConfig `json:"user,omitempty"`
}

// MemoryRetentionDefaults holds the cluster-wide retention policy.
type MemoryRetentionDefaults struct {
	// Tiers must be populated — at least one tier needs a mode for the
	// policy to do anything useful.
	// +kubebuilder:validation:Required
	Tiers MemoryRetentionTierSet `json:"tiers"`

	// +optional
	ConsentRevocation *MemoryConsentRevocationConfig `json:"consentRevocation,omitempty"`

	// +optional
	Supersession *MemorySupersessionConfig `json:"supersession,omitempty"`

	// schedule is a cron expression for when the retention worker runs.
	// Defaults to 03:00 daily so pruning happens off-peak.
	// +kubebuilder:default="0 3 * * *"
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// batchSize caps how many rows the worker processes per transaction.
	// Higher = fewer commits but longer-running locks; lower = gentler
	// on the DB but more overhead.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100000
	// +kubebuilder:default=1000
	// +optional
	BatchSize *int32 `json:"batchSize,omitempty"`
}

// MemoryWorkspaceRetentionOverride lets operators tighten or loosen the
// policy for a specific workspace. Same fields as the cluster default;
// missing fields inherit from the default.
type MemoryWorkspaceRetentionOverride struct {
	// +optional
	Tiers *MemoryRetentionTierSet `json:"tiers,omitempty"`

	// +optional
	ConsentRevocation *MemoryConsentRevocationConfig `json:"consentRevocation,omitempty"`

	// +optional
	Supersession *MemorySupersessionConfig `json:"supersession,omitempty"`

	// schedule is a cron expression; validated by the controller rather
	// than a CRD regex pattern because cron regexes are brittle under
	// OpenAPI schema validation.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100000
	BatchSize *int32 `json:"batchSize,omitempty"`
}

// MemoryPolicySpec is the top-level spec.
type MemoryPolicySpec struct {
	// default is the cluster-wide policy applied to every workspace
	// that doesn't have an explicit override.
	// +kubebuilder:validation:Required
	Default MemoryRetentionDefaults `json:"default"`

	// perWorkspace keys are Workspace resource names. Values override
	// the matching fields in default; unset fields inherit.
	// +optional
	PerWorkspace map[string]MemoryWorkspaceRetentionOverride `json:"perWorkspace,omitempty"`
}

// MemoryPolicyStatus is the observed state.
type MemoryPolicyStatus struct {
	// phase is the controller's high-level lifecycle state.
	// +optional
	Phase MemoryPolicyPhase `json:"phase,omitempty"`

	// observedGeneration is the spec generation the controller last
	// reconciled. Lets the dashboard detect "spec changed but status
	// hasn't caught up".
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// workspaceCount is the number of per-workspace overrides that
	// resolved successfully (i.e. the named Workspace actually exists).
	// Useful for operator dashboards.
	// +optional
	WorkspaceCount int32 `json:"workspaceCount,omitempty"`

	// conditions reports individual validation / wiring checks.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Inst Mode",type=string,JSONPath=`.spec.default.tiers.institutional.mode`
// +kubebuilder:printcolumn:name="Agent Mode",type=string,JSONPath=`.spec.default.tiers.agent.mode`
// +kubebuilder:printcolumn:name="User Mode",type=string,JSONPath=`.spec.default.tiers.user.mode`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.default.schedule`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MemoryPolicy is the schema for the memorypolicies
// API. It defines retention rules for the memory store across the
// institutional, agent, and user tiers. See
// docs/local-backlog/memory-retention-and-pruning-proposal.md for the
// full design rationale.
//
// Phase 1 (shipped): CRD + controller validation. No pruning behaviour
// yet — the retention worker still runs the legacy TTL-only logic.
// Phase 3 will wire the composite worker; Phase 4 wires consent
// revocation; Phase 5 wires supersession cleanup.
type MemoryPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec MemoryPolicySpec `json:"spec"`

	// +optional
	Status MemoryPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MemoryPolicyList contains a list of MemoryPolicy.
type MemoryPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MemoryPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MemoryPolicy{}, &MemoryPolicyList{})
}
