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

// TierPrecedenceConfig selects a ranking strategy by which of its
// mutually-exclusive fields is populated. Exactly one must be set;
// admission rejects zero / multiple. New rankers are added as new
// sibling fields + a widened CEL rule.
// +kubebuilder:validation:XValidation:rule="has(self.multiplicative)",message="spec.tierPrecedence.multiplicative must be set"
type TierPrecedenceConfig struct {
	// multiplicative scales each memory's base score by a per-tier
	// weight. Score ordering is preserved within a tier; the weights
	// bias across tiers.
	// +optional
	Multiplicative *MultiplicativeTierPrecedence `json:"multiplicative,omitempty"`
}

// MultiplicativeTierPrecedence carries the per-tier weights for the
// multiplicative ranker. Weights parse as decimals; controller enforces
// 0 ≤ w ≤ 10. user_for_agent inherits the user weight.
type MultiplicativeTierPrecedence struct {
	// +kubebuilder:validation:Pattern=`^(10(\.0+)?|[0-9](\.[0-9]+)?)$`
	// +kubebuilder:default="1.0"
	// +optional
	Institutional string `json:"institutional,omitempty"`

	// +kubebuilder:validation:Pattern=`^(10(\.0+)?|[0-9](\.[0-9]+)?)$`
	// +kubebuilder:default="1.0"
	// +optional
	Agent string `json:"agent,omitempty"`

	// +kubebuilder:validation:Pattern=`^(10(\.0+)?|[0-9](\.[0-9]+)?)$`
	// +kubebuilder:default="1.0"
	// +optional
	User string `json:"user,omitempty"`
}

// MemoryRecallConfig tunes the read path: half-life per tier (recency
// decay), the inline-vs-preview cutoff for large memories, and the
// per-memory `related[]` cap returned alongside recall results.
type MemoryRecallConfig struct {
	// halfLife per tier in Go-duration form (e.g. "720h" = 30d).
	// Older observations decay exponentially: a memory whose age
	// equals halfLife scores at 0.5× the recency multiplier; at 5×
	// halfLife it's effectively gone. Defaults baked into the
	// recall SQL apply when this block is unset.
	// +optional
	HalfLife *MemoryRecallHalfLife `json:"halfLife,omitempty"`

	// inlineThresholdBytes is the body-size cutoff above which recall
	// returns title + summary + content_preview rather than the full
	// body. The agent calls memory__open(id) when it actually needs
	// the text. 0 / unset uses the API default (2048).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1048576
	// +optional
	InlineThresholdBytes *int32 `json:"inlineThresholdBytes,omitempty"`

	// maxRelatedPerMemory caps the per-memory `related[]` slice the
	// recall response carries. 0 / unset uses the store default (3).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=50
	// +optional
	MaxRelatedPerMemory *int32 `json:"maxRelatedPerMemory,omitempty"`
}

// MemoryRecallHalfLife carries the per-tier half-life durations used
// by the recall recency-decay multiplier.
type MemoryRecallHalfLife struct {
	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	User string `json:"user,omitempty"`

	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	Agent string `json:"agent,omitempty"`

	// +kubebuilder:validation:Pattern=`^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	Institutional string `json:"institutional,omitempty"`
}

// MemoryDedupConfig tunes the write path's two dedup mechanisms:
// the structured-key requirement for identity-class kinds and the
// embedding-similarity thresholds.
type MemoryDedupConfig struct {
	// requireAboutForKinds enumerates kinds (e.g. "fact",
	// "preference") that must carry an `about={kind, key}` hint on
	// save. Without it the agent can't engage the structured-key
	// dedup path and identity-class memories pile up as duplicates.
	// Empty / unset disables the check.
	// +optional
	RequireAboutForKinds []string `json:"requireAboutForKinds,omitempty"`

	// embeddingSimilarity tunes the cosine-based dedup. Auto-disabled
	// when no embedding provider is configured.
	// +optional
	EmbeddingSimilarity *MemoryEmbeddingDedupConfig `json:"embeddingSimilarity,omitempty"`
}

// MemoryEmbeddingDedupConfig configures the cosine-similarity dedup
// thresholds and surface limits.
//
// surfaceDuplicatesAbove must be < autoSupersedeAbove. Without this
// the surface threshold is permanently shadowed by auto-supersede:
// every match that would surface as a `potential_duplicates` hint
// is already above the auto-supersede floor and never reaches the
// agent. The CEL guard fires at admission time so operators see the
// misconfig immediately rather than wondering why no candidates
// ever surface.
// +kubebuilder:validation:XValidation:rule="!has(self.autoSupersedeAbove) || !has(self.surfaceDuplicatesAbove) || double(self.surfaceDuplicatesAbove) < double(self.autoSupersedeAbove)",message="surfaceDuplicatesAbove must be strictly less than autoSupersedeAbove"
type MemoryEmbeddingDedupConfig struct {
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// autoSupersedeAbove: cosine ≥ this auto-supersedes the prior
	// match. Defaults to 0.95 — set lower to be more aggressive.
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	AutoSupersedeAbove string `json:"autoSupersedeAbove,omitempty"`

	// surfaceDuplicatesAbove: cosine ≥ this surfaces the match as
	// `potential_duplicates` for the agent to consider on a later
	// turn. Must be < autoSupersedeAbove. Defaults to 0.85.
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +optional
	SurfaceDuplicatesAbove string `json:"surfaceDuplicatesAbove,omitempty"`

	// candidateLimit caps the number of candidates returned to the
	// agent in `potential_duplicates`. 0 / unset uses the API default.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=50
	// +optional
	CandidateLimit *int32 `json:"candidateLimit,omitempty"`
}

// MemoryPolicySpec is the top-level spec. Workspaces opt in to a
// MemoryPolicy via Workspace.spec.services[].memory.policyRef — many
// workspaces may reference one policy, and a workspace with no
// policyRef falls back to the baked-in legacy interval policy.
type MemoryPolicySpec struct {
	// tiers configures per-tier retention behaviour. At least one tier
	// needs a mode for the policy to do anything useful.
	// +kubebuilder:validation:Required
	Tiers MemoryRetentionTierSet `json:"tiers"`

	// recall tunes the read path (half-life per tier, large-memory
	// preview cutoff, per-memory related[] cap). Optional — defaults
	// baked into the API apply when omitted.
	// +optional
	Recall *MemoryRecallConfig `json:"recall,omitempty"`

	// dedup tunes the write path's structured-key + embedding-
	// similarity dedup mechanisms. Optional — defaults apply when
	// omitted.
	// +optional
	Dedup *MemoryDedupConfig `json:"dedup,omitempty"`

	// tierPrecedence applies per-tier ranking multipliers to the
	// retrieval score. Unset tiers default to 1.0 (no-op).
	// +optional
	TierPrecedence *TierPrecedenceConfig `json:"tierPrecedence,omitempty"`

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
// +kubebuilder:printcolumn:name="Inst Mode",type=string,JSONPath=`.spec.tiers.institutional.mode`
// +kubebuilder:printcolumn:name="Agent Mode",type=string,JSONPath=`.spec.tiers.agent.mode`
// +kubebuilder:printcolumn:name="User Mode",type=string,JSONPath=`.spec.tiers.user.mode`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
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
