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
	// concurrency is the maximum number of work items in flight across all workers.
	// Workers check the global in-flight count before popping new items.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Concurrency int32 `json:"concurrency,omitempty"`

	// vusPerWorker is the number of virtual users (concurrent goroutines) per worker pod.
	// Each VU independently pops, executes, and reports work items.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	VUsPerWorker int32 `json:"vusPerWorker,omitempty"`

	// ramp configures linear concurrency ramp-up and ramp-down.
	// +optional
	Ramp *RampConfig `json:"ramp,omitempty"`

	// budgetLimit is the maximum cost (in budgetCurrency) before the job is stopped.
	// The controller checks the cost accumulator periodically and cancels
	// remaining work if this limit is exceeded.
	// +optional
	BudgetLimit *string `json:"budgetLimit,omitempty"`

	// budgetCurrency is the currency for budgetLimit (e.g., "USD").
	// +kubebuilder:default="USD"
	// +optional
	BudgetCurrency string `json:"budgetCurrency,omitempty"`

	// thresholds define SLO targets evaluated after the load test completes.
	// The job fails if any threshold is violated, enabling CI/CD gating.
	// +optional
	Thresholds []LoadThreshold `json:"thresholds,omitempty"`
}

// RampConfig controls how concurrency changes over the course of a load test.
type RampConfig struct {
	// up is the duration to linearly ramp from 0 to target concurrency at the start.
	// Format: duration string (e.g., "2m", "30s").
	// +optional
	Up string `json:"up,omitempty"`

	// down is the duration to linearly ramp from target concurrency to 0 at the end.
	// Ramp-down is triggered when remaining pending items falls below concurrency × 2.
	// Format: duration string (e.g., "30s").
	// +optional
	Down string `json:"down,omitempty"`
}

// LoadThresholdMetric defines the allowed metric names for load test thresholds.
// +kubebuilder:validation:Enum=latency_avg;latency_p50;latency_p90;latency_p95;latency_p99;ttft_avg;ttft_p50;ttft_p90;ttft_p95;ttft_p99;error_rate;pass_rate;total_cost;rate_limit_rate
type LoadThresholdMetric string

const (
	LoadThresholdMetricLatencyAvg LoadThresholdMetric = "latency_avg"
	LoadThresholdMetricLatencyP50 LoadThresholdMetric = "latency_p50"
	LoadThresholdMetricLatencyP90 LoadThresholdMetric = "latency_p90"
	LoadThresholdMetricLatencyP95 LoadThresholdMetric = "latency_p95"
	LoadThresholdMetricLatencyP99 LoadThresholdMetric = "latency_p99"
	LoadThresholdMetricTTFTAvg    LoadThresholdMetric = "ttft_avg"
	LoadThresholdMetricTTFTP50    LoadThresholdMetric = "ttft_p50"
	LoadThresholdMetricTTFTP90    LoadThresholdMetric = "ttft_p90"
	LoadThresholdMetricTTFTP95    LoadThresholdMetric = "ttft_p95"
	LoadThresholdMetricTTFTP99    LoadThresholdMetric = "ttft_p99"
	LoadThresholdMetricErrorRate  LoadThresholdMetric = "error_rate"
	LoadThresholdMetricPassRate   LoadThresholdMetric = "pass_rate"
	LoadThresholdMetricTotalCost  LoadThresholdMetric = "total_cost"
	LoadThresholdMetricRateLimit  LoadThresholdMetric = "rate_limit_rate"
)

// LoadThresholdOperator defines the allowed comparison operators for thresholds.
// +kubebuilder:validation:Enum="<";">";"<=";">="
type LoadThresholdOperator string

const (
	LoadThresholdOperatorLT  LoadThresholdOperator = "<"
	LoadThresholdOperatorGT  LoadThresholdOperator = ">"
	LoadThresholdOperatorLTE LoadThresholdOperator = "<="
	LoadThresholdOperatorGTE LoadThresholdOperator = ">="
)

// LoadThreshold defines a single SLO threshold for load test pass/fail evaluation.
type LoadThreshold struct {
	// metric is the metric to evaluate (e.g., "latency_p95", "error_rate").
	// +kubebuilder:validation:Required
	Metric LoadThresholdMetric `json:"metric"`

	// operator is the comparison operator.
	// +kubebuilder:validation:Required
	Operator LoadThresholdOperator `json:"operator"`

	// value is the target value to compare against.
	// For latency metrics: a duration string (e.g., "3s", "500ms").
	// For rate metrics: a float string (e.g., "0.01", "0.95").
	// For cost metrics: a numeric string (e.g., "50.00").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
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

// ArenaProviderEntry references either a Provider CRD or an AgentRuntime CRD.
// Exactly one of providerRef or agentRef must be set.
// +kubebuilder:validation:XValidation:rule="has(self.providerRef) != has(self.agentRef)",message="exactly one of providerRef or agentRef must be set"
type ArenaProviderEntry struct {
	// providerRef references a Provider CRD for LLM access.
	// +optional
	ProviderRef *corev1alpha1.ProviderRef `json:"providerRef,omitempty"`

	// agentRef references an AgentRuntime CRD.
	// The worker connects via WebSocket (fleet mode).
	// +optional
	AgentRef *corev1alpha1.LocalObjectReference `json:"agentRef,omitempty"`
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

	// trials is the number of times to repeat each scenario × provider combination.
	// For evaluation jobs: provides statistical confidence (pass rate, flakiness score).
	// For load test jobs: defines total load volume, consumed under concurrency control.
	// Overrides per-scenario trials from scenario YAML files.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Trials *int32 `json:"trials,omitempty"`

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

	// providers maps group names to provider groups.
	// Each group is either an array (pool of test providers) or an object
	// (1:1 config-provider-ID → CRD mapping). Array mode is the default;
	// map mode lets the wizard set the exact provider ID the arena config expects.
	// Groups correspond to the arena config's provider groups (e.g., "default", "judge").
	// When set, provider YAML files from the arena project are ignored
	// and the worker resolves providers directly from CRDs.
	// An agentRef can appear in any provider position — agents and LLM providers
	// are interchangeable in the scenario × provider matrix.
	// +optional
	Providers map[string]ArenaProviderGroup `json:"providers,omitempty"`

	// toolRegistries lists ToolRegistry CRDs whose discovered tools replace
	// the arena config's tool/mcp_server file references.
	// When set, tool YAML files from the arena project are ignored.
	// +optional
	ToolRegistries []corev1alpha1.LocalObjectReference `json:"toolRegistries,omitempty"`

	// verbose enables verbose/debug logging for promptarena execution.
	// When enabled, workers will pass --verbose to promptarena for detailed output.
	// +optional
	Verbose bool `json:"verbose,omitempty"`

	// sessionRecording enables writing session data to session-api during execution.
	// When false (default), no sessions are created and no events are recorded,
	// reducing session-api load during high-volume load tests.
	// Telemetry and traces are unaffected.
	// +optional
	SessionRecording bool `json:"sessionRecording,omitempty"`
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
// Fields do NOT use omitempty so a zero count (e.g. a job that failed before
// any work items ran) still serializes as "0" rather than being stripped from
// the JSON, which previously caused jsonpath queries to return an empty string.
type JobProgress struct {
	// total is the total number of work items.
	// +optional
	Total int32 `json:"total"`

	// completed is the number of successfully completed work items.
	// +optional
	Completed int32 `json:"completed"`

	// failed is the number of failed work items.
	// +optional
	Failed int32 `json:"failed"`

	// pending is the number of pending work items.
	// +optional
	Pending int32 `json:"pending"`
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
