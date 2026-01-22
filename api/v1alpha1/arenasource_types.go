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

// GitReference specifies a Git reference (branch, tag, or commit).
type GitReference struct {
	// branch to checkout. Takes precedence over tag and commit.
	// +optional
	Branch string `json:"branch,omitempty"`

	// tag to checkout. Takes precedence over commit.
	// +optional
	Tag string `json:"tag,omitempty"`

	// commit SHA to checkout. Used when branch and tag are not specified.
	// +optional
	Commit string `json:"commit,omitempty"`
}

// GitSource specifies a Git repository as the source for PromptKit bundles.
type GitSource struct {
	// url is the Git repository URL.
	// Supports https:// and ssh:// protocols.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(https?|ssh)://.*$`
	URL string `json:"url"`

	// ref specifies the Git reference to checkout.
	// If not specified, defaults to the default branch.
	// +optional
	Ref *GitReference `json:"ref,omitempty"`

	// path is the path within the repository to the PromptKit bundle.
	// Defaults to the repository root.
	// +optional
	Path string `json:"path,omitempty"`

	// secretRef references a Secret containing Git credentials.
	// The Secret should contain 'username' and 'password' keys for HTTPS,
	// or 'identity' and 'known_hosts' keys for SSH.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// OCISource specifies an OCI registry as the source for PromptKit bundles.
type OCISource struct {
	// url is the OCI artifact URL.
	// Format: oci://registry/repository:tag or oci://registry/repository@digest
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^oci://.*$`
	URL string `json:"url"`

	// secretRef references a Secret containing registry credentials.
	// The Secret should contain 'username' and 'password' keys,
	// or a '.dockerconfigjson' key for Docker config format.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`

	// insecure allows connecting to registries without TLS verification.
	// +kubebuilder:default=false
	// +optional
	Insecure bool `json:"insecure,omitempty"`
}

// ConfigMapSource specifies a Kubernetes ConfigMap as the source for PromptKit bundles.
type ConfigMapSource struct {
	// name is the name of the ConfigMap.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the key within the ConfigMap containing the bundle.
	// Defaults to "pack.json".
	// +kubebuilder:default="pack.json"
	// +optional
	Key string `json:"key,omitempty"`
}

// ArenaSourceSpec defines the desired state of ArenaSource.
type ArenaSourceSpec struct {
	// type specifies the source type.
	// +kubebuilder:validation:Required
	Type ArenaSourceType `json:"type"`

	// git specifies the Git repository source.
	// Required when type is "git".
	// +optional
	Git *GitSource `json:"git,omitempty"`

	// oci specifies the OCI registry source.
	// Required when type is "oci".
	// +optional
	OCI *OCISource `json:"oci,omitempty"`

	// configMap specifies the ConfigMap source.
	// Required when type is "configmap".
	// +optional
	ConfigMap *ConfigMapSource `json:"configMap,omitempty"`

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

// Artifact represents a fetched PromptKit bundle artifact.
type Artifact struct {
	// revision is the source revision identifier.
	// For Git: branch@sha1:commit or tag@sha1:commit
	// For OCI: tag@sha256:digest
	// For ConfigMap: resourceVersion
	// +kubebuilder:validation:Required
	Revision string `json:"revision"`

	// url is the URL where the artifact can be downloaded (legacy tar.gz serving).
	// Deprecated: Use contentPath for filesystem-based access.
	// +optional
	URL string `json:"url,omitempty"`

	// contentPath is the filesystem path where the content is synced.
	// This is relative to the workspace content volume root.
	// Workers can mount the PVC directly and access content at this path.
	// +optional
	ContentPath string `json:"contentPath,omitempty"`

	// version is the content-addressable version hash (SHA256).
	// This identifies a specific immutable snapshot of the synced content.
	// +optional
	Version string `json:"version,omitempty"`

	// checksum is the SHA256 checksum of the artifact.
	// +optional
	Checksum string `json:"checksum,omitempty"`

	// size is the size of the artifact in bytes.
	// +optional
	Size int64 `json:"size,omitempty"`

	// lastUpdateTime is when the artifact was last updated.
	// +kubebuilder:validation:Required
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
}

// ArenaSourcePhase represents the current phase of the ArenaSource.
// +kubebuilder:validation:Enum=Pending;Ready;Fetching;Error
type ArenaSourcePhase string

const (
	// ArenaSourcePhasePending indicates the source has not been fetched yet.
	ArenaSourcePhasePending ArenaSourcePhase = "Pending"
	// ArenaSourcePhaseReady indicates the source has been successfully fetched.
	ArenaSourcePhaseReady ArenaSourcePhase = "Ready"
	// ArenaSourcePhaseFetching indicates the source is currently being fetched.
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
