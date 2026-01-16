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

// LocalObjectReference contains enough information to locate the referenced resource.
type LocalObjectReference struct {
	// name is the name of the referenced resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// NamespacedObjectReference contains enough information to locate a resource in any namespace.
type NamespacedObjectReference struct {
	// name is the name of the referenced resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the referenced resource.
	// If not specified, defaults to the same namespace as the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ScenarioFilter defines include/exclude patterns for scenario selection.
type ScenarioFilter struct {
	// include specifies glob patterns for scenarios to include.
	// If empty, all scenarios are included by default.
	// Examples: ["scenarios/*.yaml", "tests/billing-*.yaml"]
	// +optional
	Include []string `json:"include,omitempty"`

	// exclude specifies glob patterns for scenarios to exclude.
	// Exclusions are applied after inclusions.
	// Examples: ["*-wip.yaml", "scenarios/experimental/*"]
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// SelfPlayConfig configures self-play evaluation settings.
type SelfPlayConfig struct {
	// enabled enables self-play mode where agents compete against each other.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// rounds specifies the number of self-play rounds per scenario.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Rounds int32 `json:"rounds,omitempty"`

	// swapRoles enables role swapping between rounds.
	// When true, agents alternate between roles across rounds.
	// +kubebuilder:default=false
	// +optional
	SwapRoles bool `json:"swapRoles,omitempty"`
}

// EvaluationConfig configures evaluation criteria and metrics.
type EvaluationConfig struct {
	// metrics specifies which metrics to collect during evaluation.
	// Available metrics: latency, tokens, cost, quality
	// +optional
	Metrics []string `json:"metrics,omitempty"`

	// timeout is the maximum duration for a single evaluation.
	// Format: duration string (e.g., "5m", "30s").
	// +kubebuilder:default="5m"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// maxRetries is the maximum number of retries for failed evaluations.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// concurrency is the number of parallel evaluations per worker.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	Concurrency int32 `json:"concurrency,omitempty"`
}

// ArenaConfigSpec defines the desired state of ArenaConfig.
type ArenaConfigSpec struct {
	// sourceRef references the ArenaSource containing the PromptKit bundle.
	// +kubebuilder:validation:Required
	SourceRef LocalObjectReference `json:"sourceRef"`

	// scenarios configures which scenarios to run from the bundle.
	// +optional
	Scenarios *ScenarioFilter `json:"scenarios,omitempty"`

	// providers lists the Provider CRDs to use for LLM credentials.
	// Each provider will be tested against all selected scenarios.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Providers []NamespacedObjectReference `json:"providers,omitempty"`

	// toolRegistries lists the ToolRegistry CRDs to make available during evaluation.
	// Tools from all listed registries are merged and available to agents.
	// +optional
	ToolRegistries []NamespacedObjectReference `json:"toolRegistries,omitempty"`

	// selfPlay configures self-play evaluation settings.
	// +optional
	SelfPlay *SelfPlayConfig `json:"selfPlay,omitempty"`

	// evaluation configures evaluation criteria and settings.
	// +optional
	Evaluation *EvaluationConfig `json:"evaluation,omitempty"`

	// suspend prevents new jobs from being created when set to true.
	// Existing jobs are not affected.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// ArenaConfigPhase represents the current phase of the ArenaConfig.
// +kubebuilder:validation:Enum=Pending;Ready;Invalid;Error
type ArenaConfigPhase string

const (
	// ArenaConfigPhasePending indicates the config is being validated.
	ArenaConfigPhasePending ArenaConfigPhase = "Pending"
	// ArenaConfigPhaseReady indicates the config is valid and ready for jobs.
	ArenaConfigPhaseReady ArenaConfigPhase = "Ready"
	// ArenaConfigPhaseInvalid indicates the config has validation errors.
	ArenaConfigPhaseInvalid ArenaConfigPhase = "Invalid"
	// ArenaConfigPhaseError indicates an error occurred during validation.
	ArenaConfigPhaseError ArenaConfigPhase = "Error"
)

// ResolvedSource contains information about the resolved ArenaSource.
type ResolvedSource struct {
	// revision is the resolved artifact revision from the source.
	// +optional
	Revision string `json:"revision,omitempty"`

	// url is the artifact download URL from the source.
	// +optional
	URL string `json:"url,omitempty"`

	// scenarioCount is the number of scenarios matching the filter.
	// +optional
	ScenarioCount int32 `json:"scenarioCount,omitempty"`
}

// ArenaConfigStatus defines the observed state of ArenaConfig.
type ArenaConfigStatus struct {
	// phase represents the current lifecycle phase of the ArenaConfig.
	// +optional
	Phase ArenaConfigPhase `json:"phase,omitempty"`

	// conditions represent the current state of the ArenaConfig resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// resolvedSource contains information about the resolved ArenaSource.
	// +optional
	ResolvedSource *ResolvedSource `json:"resolvedSource,omitempty"`

	// resolvedProviders lists the validated provider references.
	// +optional
	ResolvedProviders []string `json:"resolvedProviders,omitempty"`

	// lastValidatedAt is the timestamp of the last successful validation.
	// +optional
	LastValidatedAt *metav1.Time `json:"lastValidatedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Scenarios",type=integer,JSONPath=`.status.resolvedSource.scenarioCount`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ArenaConfig is the Schema for the arenaconfigs API.
// It defines a test configuration that combines an ArenaSource with providers and settings.
type ArenaConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ArenaConfig
	// +required
	Spec ArenaConfigSpec `json:"spec"`

	// status defines the observed state of ArenaConfig
	// +optional
	Status ArenaConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArenaConfigList contains a list of ArenaConfig.
type ArenaConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArenaConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArenaConfig{}, &ArenaConfigList{})
}
