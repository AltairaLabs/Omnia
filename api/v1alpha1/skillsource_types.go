/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SkillSourceType defines the type of source for skill content.
// Matches the variants shared with ArenaSource in sourcesync_types.go.
// +kubebuilder:validation:Enum=git;oci;configmap
type SkillSourceType string

const (
	// SkillSourceTypeGit fetches skills from a Git repository.
	SkillSourceTypeGit SkillSourceType = "git"
	// SkillSourceTypeOCI fetches skills from an OCI registry.
	SkillSourceTypeOCI SkillSourceType = "oci"
	// SkillSourceTypeConfigMap fetches skills from a Kubernetes ConfigMap.
	SkillSourceTypeConfigMap SkillSourceType = "configmap"
)

// SkillFilter narrows which skills from the synced tree are kept.
// A skill is a directory containing a SKILL.md file.
type SkillFilter struct {
	// include is a list of glob patterns (matched against each skill's
	// directory path relative to targetPath). An empty list means include all.
	// +optional
	Include []string `json:"include,omitempty"`

	// exclude is a list of glob patterns to drop after include is applied.
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// names pins individual skills by frontmatter name. Applied after
	// include/exclude.
	// +optional
	Names []string `json:"names,omitempty"`
}

// SkillSourceSpec defines the desired state of a SkillSource.
// +kubebuilder:validation:XValidation:rule="self.type != 'git' || has(self.git)",message="git source requires spec.git"
// +kubebuilder:validation:XValidation:rule="self.type != 'oci' || has(self.oci)",message="oci source requires spec.oci"
// +kubebuilder:validation:XValidation:rule="self.type != 'configmap' || has(self.configMap)",message="configmap source requires spec.configMap"
type SkillSourceSpec struct {
	// type selects the source variant. Exactly one of git/oci/configMap must be set.
	// +kubebuilder:validation:Required
	Type SkillSourceType `json:"type"`

	// git specifies a Git repository source.
	// +optional
	Git *GitSource `json:"git,omitempty"`

	// oci specifies an OCI registry source.
	// +optional
	OCI *OCISource `json:"oci,omitempty"`

	// configMap specifies a ConfigMap source.
	// +optional
	ConfigMap *ConfigMapSource `json:"configMap,omitempty"`

	// interval is the reconciliation poll interval (e.g. "1h").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`
	Interval string `json:"interval"`

	// timeout is the maximum duration for a single fetch (e.g. "5m").
	// +kubebuilder:default="60s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// suspend prevents the source from being reconciled when set to true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// targetPath is the path under the workspace content PVC where synced
	// content lands, e.g. "skills/anthropic". Defaults to "skills/{source-name}".
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// filter narrows which skills from the synced tree are exposed.
	// +optional
	Filter *SkillFilter `json:"filter,omitempty"`

	// createVersionOnSync mirrors the ArenaSource field: when true, each sync
	// produces a content-addressable snapshot alongside the HEAD pointer.
	// +kubebuilder:default=true
	// +optional
	CreateVersionOnSync *bool `json:"createVersionOnSync,omitempty"`
}

// SkillSourcePhase reports the current lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Initializing;Ready;Fetching;Error
type SkillSourcePhase string

const (
	// SkillSourcePhasePending indicates the source has not yet been fetched.
	SkillSourcePhasePending SkillSourcePhase = "Pending"
	// SkillSourcePhaseInitializing indicates the first fetch is in progress.
	SkillSourcePhaseInitializing SkillSourcePhase = "Initializing"
	// SkillSourcePhaseReady indicates the source has been successfully fetched.
	SkillSourcePhaseReady SkillSourcePhase = "Ready"
	// SkillSourcePhaseFetching indicates a re-sync is in progress.
	SkillSourcePhaseFetching SkillSourcePhase = "Fetching"
	// SkillSourcePhaseError indicates an error occurred during fetch or sync.
	SkillSourcePhaseError SkillSourcePhase = "Error"
)

// SkillSourceStatus defines the observed state of a SkillSource.
type SkillSourceStatus struct {
	// phase reports the lifecycle phase.
	// +optional
	Phase SkillSourcePhase `json:"phase,omitempty"`

	// observedGeneration tracks the last spec generation the controller saw.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// artifact describes the last successfully fetched artifact.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`

	// skillCount is the number of SKILL.md directories that pass the filter
	// and parse successfully.
	// +optional
	SkillCount int32 `json:"skillCount,omitempty"`

	// conditions report detailed status. Known types:
	//   SourceAvailable — upstream reachable + fetched
	//   ContentValid    — every resolved SKILL.md frontmatter parses
	//                     cleanly, no duplicate names inside this source
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// lastFetchTime is the timestamp of the last fetch attempt.
	// +optional
	LastFetchTime *metav1.Time `json:"lastFetchTime,omitempty"`

	// nextFetchTime is the scheduled time for the next fetch.
	// +optional
	NextFetchTime *metav1.Time `json:"nextFetchTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Skills",type=integer,JSONPath=`.status.skillCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SkillSource is a reusable, namespaced declaration of skill content fetched
// from an upstream source (Git, OCI, or ConfigMap) into the workspace PVC.
// Referenced from PromptPack.spec.skills[].source.
type SkillSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the SkillSource.
	Spec SkillSourceSpec `json:"spec"`

	// Status reports the observed state of the SkillSource.
	Status SkillSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SkillSourceList contains a list of SkillSource.
type SkillSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SkillSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SkillSource{}, &SkillSourceList{})
}
