/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArenaTemplateSourceType defines the type of source for templates.
// +kubebuilder:validation:Enum=git;oci;configmap
type ArenaTemplateSourceType string

const (
	// ArenaTemplateSourceTypeGit fetches templates from a Git repository.
	ArenaTemplateSourceTypeGit ArenaTemplateSourceType = "git"
	// ArenaTemplateSourceTypeOCI fetches templates from an OCI registry.
	ArenaTemplateSourceTypeOCI ArenaTemplateSourceType = "oci"
	// ArenaTemplateSourceTypeConfigMap fetches templates from a Kubernetes ConfigMap.
	ArenaTemplateSourceTypeConfigMap ArenaTemplateSourceType = "configmap"
)

// TemplateVariableType defines the type of a template variable.
// +kubebuilder:validation:Enum=string;number;boolean;enum
type TemplateVariableType string

const (
	// TemplateVariableTypeString is a string variable.
	TemplateVariableTypeString TemplateVariableType = "string"
	// TemplateVariableTypeNumber is a numeric variable.
	TemplateVariableTypeNumber TemplateVariableType = "number"
	// TemplateVariableTypeBoolean is a boolean variable.
	TemplateVariableTypeBoolean TemplateVariableType = "boolean"
	// TemplateVariableTypeEnum is an enumeration variable with predefined options.
	TemplateVariableTypeEnum TemplateVariableType = "enum"
)

// TemplateVariable defines a variable that can be used in templates.
type TemplateVariable struct {
	// name is the variable name used in templates.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z][a-zA-Z0-9_]*$`
	Name string `json:"name"`

	// type is the variable type.
	// +kubebuilder:validation:Required
	Type TemplateVariableType `json:"type"`

	// description explains the purpose of this variable.
	// +optional
	Description string `json:"description,omitempty"`

	// required indicates whether the variable must be provided.
	// +kubebuilder:default=false
	// +optional
	Required bool `json:"required,omitempty"`

	// default is the default value for the variable.
	// For boolean variables, use "true" or "false".
	// For number variables, use a numeric string.
	// +optional
	Default string `json:"default,omitempty"`

	// pattern is a regex pattern for validating string values.
	// Only applicable when type is "string".
	// +optional
	Pattern string `json:"pattern,omitempty"`

	// options are the allowed values for enum variables.
	// Only applicable when type is "enum".
	// +optional
	Options []string `json:"options,omitempty"`

	// min is the minimum value for number variables (as string).
	// Only applicable when type is "number".
	// +optional
	Min string `json:"min,omitempty"`

	// max is the maximum value for number variables (as string).
	// Only applicable when type is "number".
	// +optional
	Max string `json:"max,omitempty"`
}

// TemplateFileSpec defines how a file in the template should be processed.
type TemplateFileSpec struct {
	// path is the path to the file or directory within the template.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// render indicates whether to apply Go template rendering.
	// If false, the file is copied as-is.
	// +kubebuilder:default=true
	// +optional
	Render bool `json:"render,omitempty"`
}

// TemplateMetadata contains metadata about a discovered template.
type TemplateMetadata struct {
	// name is the unique name of the template.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// version is the semantic version of the template.
	// +optional
	Version string `json:"version,omitempty"`

	// displayName is the human-readable name.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// description explains what the template does.
	// +optional
	Description string `json:"description,omitempty"`

	// category groups templates by type (e.g., chatbot, agent, assistant).
	// +optional
	Category string `json:"category,omitempty"`

	// tags are searchable labels for the template.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// variables are the configurable parameters for the template.
	// +optional
	Variables []TemplateVariable `json:"variables,omitempty"`

	// files specifies which files to include and how to process them.
	// +optional
	Files []TemplateFileSpec `json:"files,omitempty"`

	// path is the path to the template within the source.
	// +kubebuilder:validation:Required
	Path string `json:"path"`
}

// ArenaTemplateSourceSpec defines the desired state of ArenaTemplateSource.
type ArenaTemplateSourceSpec struct {
	// type specifies the source type.
	// +kubebuilder:validation:Required
	Type ArenaTemplateSourceType `json:"type"`

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

	// syncInterval is the interval between sync operations.
	// Format: duration string (e.g., "5m", "1h").
	// Defaults to "1h".
	// +kubebuilder:default="1h"
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`
	// +optional
	SyncInterval string `json:"syncInterval,omitempty"`

	// suspend prevents the source from being reconciled when set to true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// timeout is the timeout for fetch operations.
	// Defaults to 60s.
	// +kubebuilder:default="60s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// templatesPath is the path within the source where templates are located.
	// Defaults to "templates/".
	// +kubebuilder:default="templates/"
	// +optional
	TemplatesPath string `json:"templatesPath,omitempty"`
}

// ArenaTemplateSourcePhase represents the current phase of the ArenaTemplateSource.
// +kubebuilder:validation:Enum=Pending;Ready;Fetching;Scanning;Error
type ArenaTemplateSourcePhase string

const (
	// ArenaTemplateSourcePhasePending indicates the source has not been fetched yet.
	ArenaTemplateSourcePhasePending ArenaTemplateSourcePhase = "Pending"
	// ArenaTemplateSourcePhaseReady indicates the source has been successfully fetched and templates discovered.
	ArenaTemplateSourcePhaseReady ArenaTemplateSourcePhase = "Ready"
	// ArenaTemplateSourcePhaseFetching indicates the source is currently being fetched.
	ArenaTemplateSourcePhaseFetching ArenaTemplateSourcePhase = "Fetching"
	// ArenaTemplateSourcePhaseScanning indicates templates are being discovered.
	ArenaTemplateSourcePhaseScanning ArenaTemplateSourcePhase = "Scanning"
	// ArenaTemplateSourcePhaseError indicates an error occurred.
	ArenaTemplateSourcePhaseError ArenaTemplateSourcePhase = "Error"
)

// ArenaTemplateSourceStatus defines the observed state of ArenaTemplateSource.
type ArenaTemplateSourceStatus struct {
	// phase represents the current lifecycle phase.
	// +optional
	Phase ArenaTemplateSourcePhase `json:"phase,omitempty"`

	// conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// templateCount is the number of templates discovered.
	// +optional
	TemplateCount int `json:"templateCount,omitempty"`

	// templates contains metadata for all discovered templates.
	// +optional
	Templates []TemplateMetadata `json:"templates,omitempty"`

	// lastFetchTime is the timestamp of the last fetch attempt.
	// +optional
	LastFetchTime *metav1.Time `json:"lastFetchTime,omitempty"`

	// nextFetchTime is the scheduled time for the next fetch.
	// +optional
	NextFetchTime *metav1.Time `json:"nextFetchTime,omitempty"`

	// headVersion is the current commit/revision hash.
	// +optional
	HeadVersion string `json:"headVersion,omitempty"`

	// artifact contains information about the fetched content.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`

	// message provides additional details about the current state.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Templates",type=integer,JSONPath=`.status.templateCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ArenaTemplateSource is the Schema for the arenatemplatesources API.
// It defines a source for discovering and fetching project templates.
type ArenaTemplateSource struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state
	// +required
	Spec ArenaTemplateSourceSpec `json:"spec"`

	// status defines the observed state
	// +optional
	Status ArenaTemplateSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArenaTemplateSourceList contains a list of ArenaTemplateSource.
type ArenaTemplateSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArenaTemplateSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArenaTemplateSource{}, &ArenaTemplateSourceList{})
}
