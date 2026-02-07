/*
Copyright 2025.

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

// PartitionStrategy defines how warm store data is partitioned.
// +kubebuilder:validation:Enum=week
type PartitionStrategy string

const (
	// PartitionStrategyWeek partitions data by ISO week.
	PartitionStrategyWeek PartitionStrategy = "week"
)

// SessionRetentionPolicyPhase represents the current phase of the policy.
// +kubebuilder:validation:Enum=Active;Error
type SessionRetentionPolicyPhase string

const (
	// SessionRetentionPolicyPhaseActive indicates the policy is valid and active.
	SessionRetentionPolicyPhaseActive SessionRetentionPolicyPhase = "Active"
	// SessionRetentionPolicyPhaseError indicates the policy has a configuration error.
	SessionRetentionPolicyPhaseError SessionRetentionPolicyPhase = "Error"
)

// HotCacheConfig defines configuration for the Redis hot cache tier.
type HotCacheConfig struct {
	// enabled specifies whether the hot cache is active.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ttlAfterInactive is the duration after which inactive sessions are evicted from the hot cache.
	// Must be a Go duration string (e.g., "24h", "30m", "1h30m").
	// +kubebuilder:default="24h"
	// +kubebuilder:validation:Pattern=`^([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +optional
	TTLAfterInactive string `json:"ttlAfterInactive,omitempty"`

	// maxSessions is the maximum number of sessions to keep in the hot cache.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxSessions *int32 `json:"maxSessions,omitempty"`

	// maxMessagesPerSession is the maximum number of messages per session in the hot cache.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxMessagesPerSession *int32 `json:"maxMessagesPerSession,omitempty"`
}

// WarmStoreConfig defines configuration for the Postgres warm store tier.
type WarmStoreConfig struct {
	// retentionDays is the number of days to retain data in the warm store.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3650
	// +kubebuilder:default=7
	// +optional
	RetentionDays int32 `json:"retentionDays,omitempty"`

	// partitionBy defines the partitioning strategy for warm store tables.
	// +kubebuilder:default=week
	// +optional
	PartitionBy PartitionStrategy `json:"partitionBy,omitempty"`
}

// ColdArchiveConfig defines configuration for the cold archive tier (e.g., S3, GCS).
// +kubebuilder:validation:XValidation:rule="!self.enabled || (has(self.retentionDays) && self.retentionDays > 0)",message="retentionDays is required when cold archive is enabled"
type ColdArchiveConfig struct {
	// enabled specifies whether cold archival is active.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// retentionDays is the number of days to retain data in the cold archive.
	// Required when enabled is true.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=36500
	// +optional
	RetentionDays *int32 `json:"retentionDays,omitempty"`

	// compactionSchedule is a cron expression for when to run compaction/archival.
	// +kubebuilder:default="0 2 * * *"
	// +optional
	CompactionSchedule string `json:"compactionSchedule,omitempty"`
}

// RetentionTierConfig defines the retention configuration across all storage tiers.
type RetentionTierConfig struct {
	// hotCache configures the Redis hot cache tier.
	// +optional
	HotCache *HotCacheConfig `json:"hotCache,omitempty"`

	// warmStore configures the Postgres warm store tier.
	// +optional
	WarmStore *WarmStoreConfig `json:"warmStore,omitempty"`

	// coldArchive configures the cold archive tier (e.g., S3, GCS).
	// +optional
	ColdArchive *ColdArchiveConfig `json:"coldArchive,omitempty"`
}

// WorkspaceRetentionOverride defines per-workspace retention overrides.
// HotCache is not overridable per workspace — it is a shared Redis instance.
type WorkspaceRetentionOverride struct {
	// warmStore overrides the warm store configuration for this workspace.
	// +optional
	WarmStore *WarmStoreConfig `json:"warmStore,omitempty"`

	// coldArchive overrides the cold archive configuration for this workspace.
	// +optional
	ColdArchive *ColdArchiveConfig `json:"coldArchive,omitempty"`
}

// SessionRetentionPolicySpec defines the desired state of SessionRetentionPolicy.
type SessionRetentionPolicySpec struct {
	// default defines the default retention configuration across all storage tiers.
	// +kubebuilder:validation:Required
	Default RetentionTierConfig `json:"default"`

	// perWorkspace defines per-workspace retention overrides.
	// Map keys are Workspace resource names.
	// +optional
	PerWorkspace map[string]WorkspaceRetentionOverride `json:"perWorkspace,omitempty"`
}

// SessionRetentionPolicyStatus defines the observed state of SessionRetentionPolicy.
type SessionRetentionPolicyStatus struct {
	// phase represents the current lifecycle phase of the policy.
	// +optional
	Phase SessionRetentionPolicyPhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// workspaceCount is the number of workspaces with per-workspace overrides that were resolved.
	// +optional
	WorkspaceCount int32 `json:"workspaceCount,omitempty"`

	// conditions represent the current state of the SessionRetentionPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Hot Cache TTL",type=string,JSONPath=`.spec.default.hotCache.ttlAfterInactive`
// +kubebuilder:printcolumn:name="Warm Days",type=integer,JSONPath=`.spec.default.warmStore.retentionDays`
// +kubebuilder:printcolumn:name="Cold Archive",type=boolean,JSONPath=`.spec.default.coldArchive.enabled`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SessionRetentionPolicy is the Schema for the sessionretentionpolicies API.
// It defines retention rules for session history data across hot/warm/cold storage tiers.
type SessionRetentionPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SessionRetentionPolicy
	// +required
	Spec SessionRetentionPolicySpec `json:"spec"`

	// status defines the observed state of SessionRetentionPolicy
	// +optional
	Status SessionRetentionPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SessionRetentionPolicyList contains a list of SessionRetentionPolicy.
type SessionRetentionPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SessionRetentionPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SessionRetentionPolicy{}, &SessionRetentionPolicyList{})
}
