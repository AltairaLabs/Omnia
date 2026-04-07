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

// RolloutAnalysisArg defines a template argument for a RolloutAnalysis.
type RolloutAnalysisArg struct {
	// name is the argument name. Referenced in provider queries/URLs using the args.name syntax.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// value is the default value for this argument. Can be overridden at reference time.
	// +optional
	Value *string `json:"value,omitempty"`
}

// PrometheusProvider configures a Prometheus metric query.
type PrometheusProvider struct {
	// address is the Prometheus server URL (e.g. "http://prometheus:9090").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// query is a PromQL query string. Supports args.name template substitution syntax.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`

	// timeout is the query timeout in seconds.
	// +kubebuilder:default=30
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`
}

// ArenaEvalProvider configures an Arena evaluation as a metric source.
type ArenaEvalProvider struct {
	// workspace is the name of the workspace that owns the evaluation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Workspace string `json:"workspace"`

	// evalDef is the name of the evaluation definition to run.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EvalDef string `json:"evalDef"`
}

// WebProvider configures an HTTP endpoint as a metric source.
type WebProvider struct {
	// url is the HTTP endpoint URL. Supports args.name template substitution syntax.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// method is the HTTP method to use.
	// +kubebuilder:validation:Enum=GET;POST
	// +kubebuilder:default="GET"
	// +optional
	Method string `json:"method,omitempty"`

	// headers is a map of HTTP headers to include in the request.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// jsonPath is a JSONPath expression to extract the metric value from the response body.
	// +optional
	JSONPath string `json:"jsonPath,omitempty"`

	// timeout is the request timeout in seconds.
	// +kubebuilder:default=30
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`
}

// MetricProvider is a union — exactly one field should be set.
type MetricProvider struct {
	// prometheus configures a Prometheus metric query.
	// +optional
	Prometheus *PrometheusProvider `json:"prometheus,omitempty"`

	// arenaEval configures an Arena evaluation as a metric source.
	// +optional
	ArenaEval *ArenaEvalProvider `json:"arenaEval,omitempty"`

	// web configures an HTTP endpoint as a metric source.
	// +optional
	Web *WebProvider `json:"web,omitempty"`
}

// AnalysisMetric defines a single metric measurement within a RolloutAnalysis.
type AnalysisMetric struct {
	// name is the identifier for this metric, used in logging and status reporting.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// interval is the duration between measurements (e.g. "1m", "5m").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Interval string `json:"interval"`

	// count is the total number of measurements to take.
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// failureLimit is the maximum number of failed measurements before the analysis fails.
	// +kubebuilder:validation:Minimum=0
	// +optional
	FailureLimit int32 `json:"failureLimit,omitempty"`

	// successCondition is a CEL/expression evaluated against the metric result to determine success
	// (e.g. "result[0] <= 0.05").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SuccessCondition string `json:"successCondition"`

	// failureCondition is an explicit expression that, when true, marks the measurement as failed.
	// +optional
	FailureCondition string `json:"failureCondition,omitempty"`

	// provider defines where the metric value is sourced from.
	// +kubebuilder:validation:Required
	Provider MetricProvider `json:"provider"`
}

// RolloutAnalysisSpec defines the desired state of RolloutAnalysis.
type RolloutAnalysisSpec struct {
	// args defines template arguments that can be overridden when referencing this analysis.
	// +optional
	Args []RolloutAnalysisArg `json:"args,omitempty"`

	// metrics defines the set of metric measurements to perform.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Metrics []AnalysisMetric `json:"metrics"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ra
// +kubebuilder:printcolumn:name="Metrics",type=integer,JSONPath=`.spec.metrics[*]`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RolloutAnalysis is the Schema for the rolloutanalyses API.
// It defines reusable metric templates for evaluating rollout health.
type RolloutAnalysis struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of RolloutAnalysis.
	// +required
	Spec RolloutAnalysisSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// RolloutAnalysisList contains a list of RolloutAnalysis.
type RolloutAnalysisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RolloutAnalysis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RolloutAnalysis{}, &RolloutAnalysisList{})
}
