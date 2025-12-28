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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PromptPackSourceType defines the type of source for prompt configuration
// +kubebuilder:validation:Enum=configmap
type PromptPackSourceType string

const (
	// PromptPackSourceTypeConfigMap indicates the prompt configuration is stored in a ConfigMap
	PromptPackSourceTypeConfigMap PromptPackSourceType = "configmap"
)

// RolloutStrategyType defines the type of rollout strategy
// +kubebuilder:validation:Enum=immediate;canary
type RolloutStrategyType string

const (
	// RolloutStrategyImmediate deploys all traffic to the new version immediately
	RolloutStrategyImmediate RolloutStrategyType = "immediate"
	// RolloutStrategyCanary gradually shifts traffic to the new version
	RolloutStrategyCanary RolloutStrategyType = "canary"
)

// PromptPackSource defines the source configuration for prompts
type PromptPackSource struct {
	// type specifies the type of source for the prompt configuration.
	// Currently only "configmap" is supported.
	// +kubebuilder:validation:Required
	Type PromptPackSourceType `json:"type"`

	// configMapRef references a ConfigMap containing the prompt configuration.
	// Required when type is "configmap".
	// +optional
	ConfigMapRef *corev1.LocalObjectReference `json:"configMapRef,omitempty"`
}

// CanaryConfig defines the canary rollout configuration
type CanaryConfig struct {
	// weight specifies the percentage of traffic to route to the canary version.
	// Must be between 0 and 100.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	Weight int32 `json:"weight"`

	// stepWeight specifies the percentage to increase canary traffic on each step.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	// +optional
	StepWeight *int32 `json:"stepWeight,omitempty"`

	// interval specifies the time to wait between canary steps.
	// +kubebuilder:default="5m"
	// +optional
	Interval *string `json:"interval,omitempty"`
}

// RolloutStrategy defines how new versions are rolled out
type RolloutStrategy struct {
	// type specifies the rollout strategy type.
	// "immediate" deploys all traffic to the new version at once.
	// "canary" gradually shifts traffic to the new version.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=immediate
	Type RolloutStrategyType `json:"type"`

	// canary specifies the canary rollout configuration.
	// Required when type is "canary".
	// +optional
	Canary *CanaryConfig `json:"canary,omitempty"`
}

// PromptPackSpec defines the desired state of PromptPack
type PromptPackSpec struct {
	// source specifies where the prompt configuration is stored.
	// +kubebuilder:validation:Required
	Source PromptPackSource `json:"source"`

	// version specifies the semantic version of this prompt pack.
	// Must follow semver format (e.g., "1.0.0", "2.1.0-beta.1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`
	Version string `json:"version"`

	// rollout specifies how this version should be rolled out.
	// +kubebuilder:validation:Required
	Rollout RolloutStrategy `json:"rollout"`
}

// PromptPackPhase represents the current phase of the PromptPack
// +kubebuilder:validation:Enum=Pending;Active;Canary;Superseded;Failed
type PromptPackPhase string

const (
	// PromptPackPhasePending indicates the PromptPack is being processed
	PromptPackPhasePending PromptPackPhase = "Pending"
	// PromptPackPhaseActive indicates the PromptPack is the active version
	PromptPackPhaseActive PromptPackPhase = "Active"
	// PromptPackPhaseCanary indicates the PromptPack is receiving canary traffic
	PromptPackPhaseCanary PromptPackPhase = "Canary"
	// PromptPackPhaseSuperseded indicates the PromptPack was replaced by a newer version
	PromptPackPhaseSuperseded PromptPackPhase = "Superseded"
	// PromptPackPhaseFailed indicates the PromptPack failed to deploy
	PromptPackPhaseFailed PromptPackPhase = "Failed"
)

// PromptPackStatus defines the observed state of PromptPack.
type PromptPackStatus struct {
	// phase represents the current lifecycle phase of the PromptPack.
	// +optional
	Phase PromptPackPhase `json:"phase,omitempty"`

	// activeVersion is the currently active version serving production traffic.
	// +optional
	ActiveVersion *string `json:"activeVersion,omitempty"`

	// canaryVersion is the version currently receiving canary traffic, if any.
	// +optional
	CanaryVersion *string `json:"canaryVersion,omitempty"`

	// canaryWeight is the current percentage of traffic going to the canary version.
	// +optional
	CanaryWeight *int32 `json:"canaryWeight,omitempty"`

	// lastUpdated is the timestamp of the last status update.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// conditions represent the current state of the PromptPack resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Prompt pack version"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.rollout.type",description="Rollout strategy"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PromptPack is the Schema for the promptpacks API.
// It defines a versioned prompt configuration with rollout strategy support.
type PromptPack struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of PromptPack
	// +required
	Spec PromptPackSpec `json:"spec"`

	// status defines the observed state of PromptPack
	// +optional
	Status PromptPackStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PromptPackList contains a list of PromptPack
type PromptPackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PromptPack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PromptPack{}, &PromptPackList{})
}
