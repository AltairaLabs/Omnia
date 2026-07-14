/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// PromptPackSourceType is the kind of upstream feed.
// +kubebuilder:validation:Enum=git;oci
type PromptPackSourceType string

const (
	PromptPackSourceTypeGit PromptPackSourceType = "git"
	PromptPackSourceTypeOCI PromptPackSourceType = "oci"
)

// PromptPackSourcePhase is the lifecycle phase of a PromptPackSource.
// +kubebuilder:validation:Enum=Pending;Ready;Fetching;Error
type PromptPackSourcePhase string

const (
	PromptPackSourcePhasePending  PromptPackSourcePhase = "Pending"
	PromptPackSourcePhaseReady    PromptPackSourcePhase = "Ready"
	PromptPackSourcePhaseFetching PromptPackSourcePhase = "Fetching"
	PromptPackSourcePhaseError    PromptPackSourcePhase = "Error"
)

// PromptPackSourceConditionReady is the ready condition type.
const PromptPackSourceConditionReady = "Ready"

// PromptPackSourceSpec defines a feed of pre-built packc releases for one logical pack.
// +kubebuilder:validation:XValidation:rule="(self.type == 'git' && has(self.git) && !has(self.oci)) || (self.type == 'oci' && has(self.oci) && !has(self.git))",message="exactly the source block matching type must be set"
type PromptPackSourceSpec struct {
	// type selects the upstream feed kind.
	// +kubebuilder:validation:Required
	Type PromptPackSourceType `json:"type"`

	// git points at a git repo whose configured path holds pack.json.
	// +optional
	Git *corev1alpha1.GitSource `json:"git,omitempty"`

	// oci points at an OCI artifact holding pack.json.
	// +optional
	OCI *corev1alpha1.OCISource `json:"oci,omitempty"`

	// packName is the logical pack this source publishes (one source : one pack).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PackName string `json:"packName"`

	// interval is the poll cadence, e.g. "5m".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`
	Interval string `json:"interval"`

	// timeout bounds a single fetch. Defaults to "60s".
	// +kubebuilder:default="60s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// suspend halts polling when true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// historyLimit caps retained Superseded version-objects per packName. Defaults to 10.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=0
	// +optional
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
}

// PromptPackSourceStatus is the observed state.
type PromptPackSourceStatus struct {
	// +optional
	Phase PromptPackSourcePhase `json:"phase,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Artifact *corev1alpha1.Artifact `json:"artifact,omitempty"`
	// lastSyncedVersion is the version most recently materialized.
	// +optional
	LastSyncedVersion string `json:"lastSyncedVersion,omitempty"`
	// versionsMaterialized counts version-objects this source has created.
	// +optional
	VersionsMaterialized int32 `json:"versionsMaterialized,omitempty"`
	// +optional
	LastFetchTime *metav1.Time `json:"lastFetchTime,omitempty"`
	// +optional
	NextFetchTime *metav1.Time `json:"nextFetchTime,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pps
// +kubebuilder:printcolumn:name="Pack",type=string,JSONPath=`.spec.packName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.lastSyncedVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PromptPackSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PromptPackSourceSpec   `json:"spec,omitempty"`
	Status            PromptPackSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PromptPackSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PromptPackSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PromptPackSource{}, &PromptPackSourceList{})
}
