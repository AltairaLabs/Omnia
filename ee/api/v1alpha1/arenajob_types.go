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

// ArenaJobType represents the type of job to execute.
// +kubebuilder:validation:Enum=evaluation;loadtest;datagen
type ArenaJobType string

const (
	// ArenaJobTypeEvaluation runs prompt evaluation against test scenarios.
	ArenaJobTypeEvaluation ArenaJobType = "evaluation"
	// ArenaJobTypeLoadTest runs load testing against providers.
	ArenaJobTypeLoadTest ArenaJobType = "loadtest"
	// ArenaJobTypeDataGen generates synthetic data using prompts.
	ArenaJobTypeDataGen ArenaJobType = "datagen"
)

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

// EvaluationSettings configures evaluation-specific settings.
type EvaluationSettings struct {
	// outputFormats specifies the formats for evaluation results.
	// Supported formats: junit, json, csv
	// +optional
	OutputFormats []string `json:"outputFormats,omitempty"`
}

// LoadTestSettings configures load testing settings.
type LoadTestSettings struct {
	// rampUp is the duration to ramp up to target concurrency.
	// Format: duration string (e.g., "1m", "30s").
	// +kubebuilder:default="30s"
	// +optional
	RampUp string `json:"rampUp,omitempty"`

	// duration is the total duration of the load test.
	// Format: duration string (e.g., "5m", "1h").
	// +kubebuilder:default="5m"
	// +optional
	Duration string `json:"duration,omitempty"`

	// targetRPS is the target requests per second.
	// +kubebuilder:validation:Minimum=1
	// +optional
	TargetRPS int32 `json:"targetRPS,omitempty"`
}

// DataGenSettings configures data generation settings.
type DataGenSettings struct {
	// count is the number of data items to generate.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=100
	// +optional
	Count int32 `json:"count,omitempty"`

	// format specifies the output format for generated data.
	// Supported formats: json, jsonl, csv
	// +kubebuilder:default="jsonl"
	// +optional
	Format string `json:"format,omitempty"`
}

// WorkerConfig configures the worker pool for job execution.
type WorkerConfig struct {
	// replicas is the number of worker replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// minReplicas is the minimum number of replicas for autoscaling.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// maxReplicas is the maximum number of replicas for autoscaling.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

// OutputType represents the type of output destination.
// +kubebuilder:validation:Enum=s3;pvc
type OutputType string

const (
	// OutputTypeS3 stores results in an S3-compatible bucket.
	OutputTypeS3 OutputType = "s3"
	// OutputTypePVC stores results in a PersistentVolumeClaim.
	OutputTypePVC OutputType = "pvc"
)

// S3OutputConfig configures S3 output destination.
type S3OutputConfig struct {
	// bucket is the S3 bucket name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`

	// prefix is the key prefix for stored objects.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// region is the AWS region for the bucket.
	// +optional
	Region string `json:"region,omitempty"`

	// endpoint is a custom S3-compatible endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// secretRef references a Secret containing S3 credentials.
	// Expected keys: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
	// +optional
	SecretRef *corev1alpha1.LocalObjectReference `json:"secretRef,omitempty"`
}

// PVCOutputConfig configures PersistentVolumeClaim output destination.
type PVCOutputConfig struct {
	// claimName is the name of the PVC to use.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClaimName string `json:"claimName"`

	// subPath is the subdirectory within the PVC.
	// +optional
	SubPath string `json:"subPath,omitempty"`
}

// OutputConfig configures where job results are stored.
type OutputConfig struct {
	// type specifies the output destination type.
	// +kubebuilder:validation:Required
	Type OutputType `json:"type"`

	// s3 configures S3 output. Required when type is "s3".
	// +optional
	S3 *S3OutputConfig `json:"s3,omitempty"`

	// pvc configures PVC output. Required when type is "pvc".
	// +optional
	PVC *PVCOutputConfig `json:"pvc,omitempty"`
}

// ProviderGroupSelector defines how to select Provider CRDs for a specific group.
type ProviderGroupSelector struct {
	// selector is a label selector to match Provider CRDs in the workspace namespace.
	// All matching providers will be used for the group, with scenarios run against each.
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`
}

// ToolRegistrySelector defines how to select ToolRegistry CRDs.
type ToolRegistrySelector struct {
	// selector is a label selector to match ToolRegistry CRDs in the workspace namespace.
	// All tools from matching registries will override tools/mcp_servers defined in arena.config.yaml.
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`
}

// ScheduleConfig configures job scheduling.
type ScheduleConfig struct {
	// cron is a cron expression for scheduled execution.
	// Format: standard cron expression (e.g., "0 2 * * *" for 2am daily).
	// +kubebuilder:validation:MinLength=9
	// +optional
	Cron string `json:"cron,omitempty"`

	// timezone specifies the timezone for the cron schedule.
	// +kubebuilder:default="UTC"
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// concurrencyPolicy specifies how to treat concurrent executions.
	// +kubebuilder:validation:Enum=Allow;Forbid;Replace
	// +kubebuilder:default="Forbid"
	// +optional
	ConcurrencyPolicy string `json:"concurrencyPolicy,omitempty"`
}

// ArenaJobSpec defines the desired state of ArenaJob.
type ArenaJobSpec struct {
	// sourceRef references the ArenaSource containing test scenarios and configuration.
	// +kubebuilder:validation:Required
	SourceRef corev1alpha1.LocalObjectReference `json:"sourceRef"`

	// arenaFile is the path to the arena config file within the source.
	// Supports glob patterns for multi-file configs (e.g., "evals/*.arena.yaml").
	// +kubebuilder:default="config.arena.yaml"
	// +optional
	ArenaFile string `json:"arenaFile,omitempty"`

	// type specifies the type of job to execute.
	// +kubebuilder:default="evaluation"
	// +optional
	Type ArenaJobType `json:"type,omitempty"`

	// scenarios filters which scenarios to run from the arena file.
	// If not specified, runs all scenarios defined in the arena file.
	// +optional
	Scenarios *ScenarioFilter `json:"scenarios,omitempty"`

	// evaluation configures evaluation-specific settings.
	// Used when type is "evaluation".
	// +optional
	Evaluation *EvaluationSettings `json:"evaluation,omitempty"`

	// loadTest configures load testing settings.
	// Used when type is "loadtest".
	// +optional
	LoadTest *LoadTestSettings `json:"loadTest,omitempty"`

	// dataGen configures data generation settings.
	// Used when type is "datagen".
	// +optional
	DataGen *DataGenSettings `json:"dataGen,omitempty"`

	// workers configures the worker pool.
	// +optional
	Workers *WorkerConfig `json:"workers,omitempty"`

	// output configures where results are stored.
	// +optional
	Output *OutputConfig `json:"output,omitempty"`

	// schedule configures scheduled/recurring execution.
	// If not specified, the job runs once immediately.
	// +optional
	Schedule *ScheduleConfig `json:"schedule,omitempty"`

	// ttlSecondsAfterFinished specifies how long to keep completed jobs.
	// +kubebuilder:validation:Minimum=0
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// verbose enables verbose/debug logging for promptarena execution.
	// When enabled, workers will pass --verbose to promptarena for detailed output.
	// +optional
	Verbose bool `json:"verbose,omitempty"`

	// providerOverrides allows overriding provider groups defined in the arena config file.
	// Keys are group names from the arena config file (e.g., "default", "judge").
	// Use "*" as a catch-all for groups not explicitly specified.
	// Provider CRDs matching the label selector provide credentials for the matched groups.
	// +optional
	ProviderOverrides map[string]ProviderGroupSelector `json:"providerOverrides,omitempty"`

	// toolRegistryOverride allows overriding tools/mcp_servers defined in the arena config file
	// with handlers from ToolRegistry CRDs. All tools from matching registries will
	// override tools with matching names in the arena config.
	// +optional
	ToolRegistryOverride *ToolRegistrySelector `json:"toolRegistryOverride,omitempty"`
}

// ArenaJobPhase represents the current phase of the ArenaJob.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Cancelled
type ArenaJobPhase string

const (
	// ArenaJobPhasePending indicates the job is waiting to start.
	ArenaJobPhasePending ArenaJobPhase = "Pending"
	// ArenaJobPhaseRunning indicates the job is actively executing.
	ArenaJobPhaseRunning ArenaJobPhase = "Running"
	// ArenaJobPhaseSucceeded indicates the job completed successfully.
	ArenaJobPhaseSucceeded ArenaJobPhase = "Succeeded"
	// ArenaJobPhaseFailed indicates the job failed.
	ArenaJobPhaseFailed ArenaJobPhase = "Failed"
	// ArenaJobPhaseCancelled indicates the job was cancelled.
	ArenaJobPhaseCancelled ArenaJobPhase = "Cancelled"
)

// JobProgress tracks the progress of a job execution.
type JobProgress struct {
	// total is the total number of work items.
	// +optional
	Total int32 `json:"total,omitempty"`

	// completed is the number of successfully completed work items.
	// +optional
	Completed int32 `json:"completed,omitempty"`

	// failed is the number of failed work items.
	// +optional
	Failed int32 `json:"failed,omitempty"`

	// pending is the number of pending work items.
	// +optional
	Pending int32 `json:"pending,omitempty"`
}

// JobResult contains summary results for a completed job.
type JobResult struct {
	// url is the URL to access detailed results.
	// +optional
	URL string `json:"url,omitempty"`

	// summary contains aggregated result metrics.
	// +optional
	Summary map[string]string `json:"summary,omitempty"`
}

// ArenaJobStatus defines the observed state of ArenaJob.
type ArenaJobStatus struct {
	// phase represents the current lifecycle phase of the job.
	// +optional
	Phase ArenaJobPhase `json:"phase,omitempty"`

	// conditions represent the current state of the job.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// progress tracks job execution progress.
	// +optional
	Progress *JobProgress `json:"progress,omitempty"`

	// result contains the job result summary.
	// +optional
	Result *JobResult `json:"result,omitempty"`

	// startTime is the timestamp when the job started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// completionTime is the timestamp when the job completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// activeWorkers is the current number of active workers.
	// +optional
	ActiveWorkers int32 `json:"activeWorkers,omitempty"`

	// lastScheduleTime is the last time a scheduled job was triggered.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// nextScheduleTime is the next scheduled execution time.
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceRef.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress.completed`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ArenaJob is the Schema for the arenajobs API.
// It defines a test execution that runs scenarios from an ArenaSource.
type ArenaJob struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ArenaJob
	// +required
	Spec ArenaJobSpec `json:"spec"`

	// status defines the observed state of ArenaJob
	// +optional
	Status ArenaJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArenaJobList contains a list of ArenaJob.
type ArenaJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArenaJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArenaJob{}, &ArenaJobList{})
}
