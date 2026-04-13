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

// ArenaSourceType defines the type of source for PromptKit bundles.
// +kubebuilder:validation:Enum=git;oci;configmap
type ArenaSourceType string

const (
	// ArenaSourceTypeGit fetches bundles from a Git repository.
	ArenaSourceTypeGit ArenaSourceType = "git"
	// ArenaSourceTypeOCI fetches bundles from an OCI registry.
	ArenaSourceTypeOCI ArenaSourceType = "oci"
	// ArenaSourceTypeConfigMap fetches bundles from a Kubernetes ConfigMap.
	ArenaSourceTypeConfigMap ArenaSourceType = "configmap"
)

// Source-sync types (GitReference, GitSource, OCISource, ConfigMapSource,
// Artifact) live in api/v1alpha1/sourcesync_types.go. Consumers reference
// them via the corev1alpha1 qualifier.

// ArenaSourceSpec defines the desired state of ArenaSource.
type ArenaSourceSpec struct {
	// type specifies the source type.
	// +kubebuilder:validation:Required
	Type ArenaSourceType `json:"type"`

	// git specifies the Git repository source.
	// Required when type is "git".
	// +optional
	Git *corev1alpha1.GitSource `json:"git,omitempty"`

	// oci specifies the OCI registry source.
	// Required when type is "oci".
	// +optional
	OCI *corev1alpha1.OCISource `json:"oci,omitempty"`

	// configMap specifies the ConfigMap source.
	// Required when type is "configmap".
	// +optional
	ConfigMap *corev1alpha1.ConfigMapSource `json:"configMap,omitempty"`

	// interval is the reconciliation interval for polling the source.
	// Format: duration string (e.g., "5m", "1h").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`
	Interval string `json:"interval"`

	// suspend prevents the source from being reconciled when set to true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// timeout is the timeout for fetch operations.
	// Defaults to 60s.
	// +kubebuilder:default="60s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// targetPath specifies where to sync content within the workspace content volume.
	// The path is relative to the workspace content root: /workspace-content/{workspace}/default/
	// If not specified, defaults to "arena/{source-name}".
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// createVersionOnSync controls whether to create a new version after each successful sync.
	// Versions are content-addressable using SHA256 hashes.
	// +kubebuilder:default=true
	// +optional
	CreateVersionOnSync *bool `json:"createVersionOnSync,omitempty"`
}

// Artifact is an alias for the core Artifact type; see
// api/v1alpha1/sourcesync_types.go for the canonical definition.
type Artifact = corev1alpha1.Artifact

// ArenaSourcePhase represents the current phase of the ArenaSource.
// +kubebuilder:validation:Enum=Pending;Initializing;Ready;Fetching;Error
type ArenaSourcePhase string

const (
	// ArenaSourcePhasePending indicates the source has not been fetched yet.
	ArenaSourcePhasePending ArenaSourcePhase = "Pending"
	// ArenaSourcePhaseInitializing indicates the first fetch is in progress.
	// Content is not yet available.
	ArenaSourcePhaseInitializing ArenaSourcePhase = "Initializing"
	// ArenaSourcePhaseReady indicates the source has been successfully fetched.
	ArenaSourcePhaseReady ArenaSourcePhase = "Ready"
	// ArenaSourcePhaseFetching indicates a re-sync is in progress.
	// Previous content remains available via HEAD.
	ArenaSourcePhaseFetching ArenaSourcePhase = "Fetching"
	// ArenaSourcePhaseError indicates an error occurred while fetching the source.
	ArenaSourcePhaseError ArenaSourcePhase = "Error"
)

// ArenaSourceStatus defines the observed state of ArenaSource.
type ArenaSourceStatus struct {
	// phase represents the current lifecycle phase of the ArenaSource.
	// +optional
	Phase ArenaSourcePhase `json:"phase,omitempty"`

	// conditions represent the current state of the ArenaSource resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// artifact contains information about the last successfully fetched artifact.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`

	// lastFetchTime is the timestamp of the last fetch attempt.
	// +optional
	LastFetchTime *metav1.Time `json:"lastFetchTime,omitempty"`

	// nextFetchTime is the scheduled time for the next fetch.
	// +optional
	NextFetchTime *metav1.Time `json:"nextFetchTime,omitempty"`

	// lastSyncRevision is the revision from the source that was last synced.
	// Used to detect when re-sync is needed.
	// +optional
	LastSyncRevision string `json:"lastSyncRevision,omitempty"`

	// lastVersionCreated is the version hash created on the last successful sync.
	// +optional
	LastVersionCreated string `json:"lastVersionCreated,omitempty"`

	// headVersion points to the current "latest" version.
	// This is updated atomically after a successful sync.
	// +optional
	HeadVersion string `json:"headVersion,omitempty"`

	// versionCount is the number of versions currently stored.
	// +optional
	VersionCount int `json:"versionCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.status.artifact.revision`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ArenaSource is the Schema for the arenasources API.
// It defines a source for fetching PromptKit bundles from external repositories.
type ArenaSource struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ArenaSource
	// +required
	Spec ArenaSourceSpec `json:"spec"`

	// status defines the observed state of ArenaSource
	// +optional
	Status ArenaSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArenaSourceList contains a list of ArenaSource.
type ArenaSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArenaSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArenaSource{}, &ArenaSourceList{})
}
