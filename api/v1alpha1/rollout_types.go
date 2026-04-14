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

// RolloutConfig defines the rollout strategy for an AgentRuntime.
// When Candidate is nil, no rollout is active (idle state).
// The step sequence determines the rollout shape — canary, blue-green,
// and experiments are all expressed as different step sequences.
type RolloutConfig struct {
	// candidate defines the sparse overrides for the candidate version.
	// When nil, no rollout is active. Only fields that differ from the
	// stable version need to be set.
	// +optional
	Candidate *CandidateOverrides `json:"candidate,omitempty"`

	// steps defines the sequence of rollout actions to execute.
	// Each step sets traffic weight, introduces a pause, or triggers analysis.
	// +kubebuilder:validation:MinItems=1
	Steps []RolloutStep `json:"steps"`

	// stickySession configures consistent hashing so a given user always
	// reaches the same version during a rollout.
	// +optional
	StickySession *StickySessionConfig `json:"stickySession,omitempty"`

	// rollback configures automatic or manual rollback behavior.
	// +optional
	Rollback *RollbackConfig `json:"rollback,omitempty"`

	// trafficRouting configures integration with a service mesh for traffic splitting.
	// +optional
	TrafficRouting *TrafficRoutingConfig `json:"trafficRouting,omitempty"`
}

// CandidateOverrides defines the sparse overrides for the candidate version.
// Only fields that differ from the stable version need to be set.
// An empty CandidateOverrides (all fields nil) is valid and means the
// candidate is identical to stable — useful for testing the rollout pipeline itself.
type CandidateOverrides struct {
	// promptPackVersion overrides the PromptPack version for the candidate.
	// When set, the candidate runs a different prompt version than stable.
	// +optional
	PromptPackVersion *string `json:"promptPackVersion,omitempty"`

	// providerRefs overrides the provider list for the candidate.
	// Use this to test a different LLM model or provider configuration.
	// +optional
	ProviderRefs []NamedProviderRef `json:"providerRefs,omitempty"`

	// toolRegistryRef overrides the tool registry for the candidate.
	// +optional
	ToolRegistryRef *ToolRegistryRef `json:"toolRegistryRef,omitempty"`
}

// RolloutStep is a union type — exactly one field should be set per step.
// A sequence of steps defines the rollout shape (canary, blue-green, etc.).
type RolloutStep struct {
	// setWeight sets the percentage of traffic sent to the candidate version.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	SetWeight *int32 `json:"setWeight,omitempty"`

	// pause introduces a hold point in the rollout.
	// If duration is set, the controller waits that long before advancing.
	// If duration is nil, the rollout waits indefinitely for manual promotion.
	// +optional
	Pause *RolloutPause `json:"pause,omitempty"`

	// analysis triggers a RolloutAnalysis evaluation at this step.
	// The rollout only advances if the analysis passes.
	// +optional
	Analysis *RolloutAnalysisStep `json:"analysis,omitempty"`
}

// RolloutPause defines a hold point in the rollout sequence.
type RolloutPause struct {
	// duration is the time to wait before automatically advancing to the next step.
	// Uses Go duration format (e.g., "5m", "1h").
	// When nil, the rollout pauses indefinitely and requires manual promotion.
	// +optional
	Duration *string `json:"duration,omitempty"`
}

// RolloutAnalysisStep references a RolloutAnalysis CRD to run at this step.
type RolloutAnalysisStep struct {
	// templateName is the name of the RolloutAnalysis CRD to instantiate.
	// The analysis runs in the same namespace as the AgentRuntime.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TemplateName string `json:"templateName"`

	// args provides argument overrides for the analysis template.
	// +optional
	Args []AnalysisArg `json:"args,omitempty"`
}

// AnalysisArg is a name/value pair passed to a RolloutAnalysis template.
type AnalysisArg struct {
	// name is the argument name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// value is the argument value.
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

// StickySessionConfig configures consistent hashing for session stickiness.
// During a rollout, this ensures a given user always reaches the same version.
type StickySessionConfig struct {
	// hashOn is the HTTP header name used as the consistent hashing key.
	// Typically "x-user-id". Requests with the same header value are always
	// routed to the same version.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	HashOn string `json:"hashOn"`
}

// RollbackMode defines how rollback is triggered.
// +kubebuilder:validation:Enum=automatic;manual;disabled
type RollbackMode string

const (
	// RollbackModeAutomatic triggers rollback automatically on analysis failure
	// or when error rates exceed configured thresholds.
	RollbackModeAutomatic RollbackMode = "automatic"

	// RollbackModeManual requires explicit operator action to trigger rollback.
	RollbackModeManual RollbackMode = "manual"

	// RollbackModeDisabled disables rollback entirely.
	RollbackModeDisabled RollbackMode = "disabled"
)

// RollbackConfig defines rollback behavior for a rollout.
type RollbackConfig struct {
	// mode controls how rollback is triggered.
	// "automatic" rolls back on analysis failure or error thresholds.
	// "manual" requires explicit operator action.
	// "disabled" prevents any rollback.
	// +kubebuilder:default="manual"
	// +optional
	Mode RollbackMode `json:"mode,omitempty"`

	// cooldown is the minimum time to wait before allowing another rollout
	// after a rollback. Uses Go duration format (e.g., "5m", "1h").
	// +kubebuilder:default="5m"
	// +optional
	Cooldown *string `json:"cooldown,omitempty"`
}

// TrafficRoutingConfig configures integration with a service mesh for traffic splitting.
type TrafficRoutingConfig struct {
	// istio configures Istio VirtualService and DestinationRule mutation.
	// The controller patches existing resources — it does not create them.
	// +optional
	Istio *IstioTrafficRouting `json:"istio,omitempty"`
}

// IstioTrafficRouting configures Istio-based traffic splitting.
// The controller patches existing VirtualService and DestinationRule resources
// to adjust traffic weights. Both resources must exist before the rollout begins.
type IstioTrafficRouting struct {
	// virtualService references the Istio VirtualService to patch.
	VirtualService IstioVirtualServiceRef `json:"virtualService"`

	// destinationRule references the Istio DestinationRule to patch.
	DestinationRule IstioDestinationRuleRef `json:"destinationRule"`
}

// IstioVirtualServiceRef references an Istio VirtualService resource.
type IstioVirtualServiceRef struct {
	// name is the VirtualService resource name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// routes lists the HTTP route names within the VirtualService to patch.
	// +kubebuilder:validation:MinItems=1
	Routes []string `json:"routes"`
}

// IstioDestinationRuleRef references an Istio DestinationRule resource.
type IstioDestinationRuleRef struct {
	// name is the DestinationRule resource name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// stableSubset is the subset name for stable (current) traffic.
	// +kubebuilder:default="stable"
	// +optional
	StableSubset string `json:"stableSubset,omitempty"`

	// candidateSubset is the subset name for candidate (new) traffic.
	// +kubebuilder:default="canary"
	// +optional
	CandidateSubset string `json:"candidateSubset,omitempty"`
}

// RolloutStatus reports the observed state of an active rollout.
type RolloutStatus struct {
	// active indicates whether a rollout is currently in progress.
	// +optional
	Active bool `json:"active,omitempty"`

	// currentStep is the zero-based index of the step currently being executed.
	// +optional
	CurrentStep *int32 `json:"currentStep,omitempty"`

	// currentWeight is the percentage of traffic currently sent to the candidate.
	// +optional
	CurrentWeight *int32 `json:"currentWeight,omitempty"`

	// stableVersion is the version identifier of the current stable deployment.
	// +optional
	StableVersion string `json:"stableVersion,omitempty"`

	// candidateVersion is the version identifier of the candidate being rolled out.
	// +optional
	CandidateVersion string `json:"candidateVersion,omitempty"`

	// startedAt is the RFC3339 timestamp when the rollout began.
	// +optional
	StartedAt *string `json:"startedAt,omitempty"`

	// stepStartedAt is the RFC3339 timestamp when the controller entered
	// the step pointed to by currentStep. Used to honour pause durations
	// across reconciles: a pause step only advances once
	// (now - stepStartedAt) >= the pause duration.
	// +optional
	StepStartedAt *string `json:"stepStartedAt,omitempty"`

	// message is a human-readable description of the current rollout state.
	// +optional
	Message string `json:"message,omitempty"`
}
