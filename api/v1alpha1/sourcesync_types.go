/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// GitSource specifies a Git repository as a content source.
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

	// path is the path within the repository to the content.
	// Defaults to the repository root.
	// +optional
	Path string `json:"path,omitempty"`

	// secretRef references a Secret containing Git credentials.
	// The Secret should contain 'username' and 'password' keys for HTTPS,
	// or 'identity' and 'known_hosts' keys for SSH.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// OCISource specifies an OCI registry as a content source.
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

// ConfigMapSource specifies a Kubernetes ConfigMap as a content source.
type ConfigMapSource struct {
	// name is the name of the ConfigMap.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the key within the ConfigMap containing the content.
	// Defaults to "pack.json".
	// +kubebuilder:default="pack.json"
	// +optional
	Key string `json:"key,omitempty"`
}

// Artifact represents a fetched content artifact.
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
