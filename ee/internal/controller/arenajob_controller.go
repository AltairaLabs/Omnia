/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// Workspace label for namespace association
const labelWorkspace = "omnia.altairalabs.ai/workspace"

// maxWorkItems limits per job type to prevent runaway matrix expansion.
const (
	maxWorkItemsEvaluation = 10000
	maxWorkItemsLoadTest   = 100000
)

// arenaSourceRetryInterval is how often to re-check a referenced ArenaSource
// that isn't ready yet (missing or mid-fetch). A missing source is transient,
// not terminal: the job waits in Pending and is also re-triggered by the
// ArenaSource watch, but this requeue covers the create-before-source-exists
// race where the watch fired before the job's first reconcile.
const arenaSourceRetryInterval = 30 * time.Second

// ArenaJob condition types
const (
	ArenaJobConditionTypeReady       = "Ready"
	ArenaJobConditionTypeSourceValid = "SourceValid"
	ArenaJobConditionTypeJobCreated  = "JobCreated"
	ArenaJobConditionTypeProgressing = "Progressing"
)

// Event reasons for ArenaJob
const (
	ArenaJobEventReasonReconciling    = "Reconciling"
	ArenaJobEventReasonConfigNotFound = "ConfigNotFound"
	ArenaJobEventReasonConfigNotReady = "ConfigNotReady"
	ArenaJobEventReasonJobCreated     = "JobCreated"
	ArenaJobEventReasonJobRunning     = "JobRunning"
	ArenaJobEventReasonJobSucceeded   = "JobSucceeded"
	ArenaJobEventReasonJobFailed      = "JobFailed"
)

// Default worker image for Arena jobs
const (
	DefaultWorkerImage = "ghcr.io/altairalabs/arena-worker:latest"
)

// Shared string literals for worker Job/pod construction and work-item
// building. Extracted to satisfy goconst (relocated lines un-grandfather
// pre-existing duplicate-string warnings).
const (
	labelAppName           = "app.kubernetes.io/name"
	labelArenaJob          = "omnia.altairalabs.ai/job"
	componentWorker        = "worker"
	volumeNameTmp          = "tmp"
	envValueTrue           = "true"
	defaultArenaConfigFile = "config.arena.yaml"
	managedByOperator      = "omnia-operator"
	// defaultName is the literal "default" used for the default service group
	// name and the default scenario ID in fallback work items.
	defaultName = "default"
)

// Default resource requests and limits for Arena worker containers.
var (
	defaultWorkerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
)

// ArenaJobReconciler reconciles an ArenaJob object
type ArenaJobReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Recorder              record.EventRecorder
	WorkerImage           string
	WorkerImagePullPolicy corev1.PullPolicy
	Queue                 queue.WorkQueue
	Aggregator            *aggregator.Aggregator
	// LicenseValidator validates license for job types/replicas/scheduling (defense in depth)
	LicenseValidator *license.Validator
	// RedisURL is the Redis connection URL (redis:// or rediss://) for
	// lazy queue connection. Worker pods receive the same URL via env
	// (literal value or secretKeyRef from RedisURLSecretName).
	RedisURL string
	// RedisURLSecretName is the Kubernetes Secret holding the Redis
	// URL when the operator uses the existingSecret form. Worker pods
	// get REDIS_URL via valueFrom.secretKeyRef on this Secret. Empty
	// means workers receive the literal RedisURL as a plain env var.
	RedisURLSecretName string
	// RedisURLSecretKey is the key within RedisURLSecretName whose
	// value is the Redis URL.
	RedisURLSecretKey string
	// WorkspaceContentPath is the base path for workspace content volumes.
	// When set, workers mount the workspace content PVC and access content directly.
	// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}
	WorkspaceContentPath string
	// WorkspaceContentScoped indicates the per-workspace content volume is
	// already scoped to the workspace subtree (the new storage-enforced
	// isolation model, e.g. an Azure native NFS PV exported at
	// /<account>/<share>/<workspace>/<namespace>). When true, the mount
	// subPath is workspace-relative — it drops the {workspace}/{namespace}
	// prefix because the volume root IS that subtree. When false (default),
	// the volume is the legacy share root and the subPath carries the full
	// {workspace}/{namespace}/{contentPath} prefix for in-volume isolation.
	WorkspaceContentScoped bool
	// NFSServer is the NFS server address for workspace content (optional).
	// When set along with NFSPath, workers mount NFS directly instead of using a PVC.
	// This enables shared access across namespaces without per-workspace PVCs.
	NFSServer string
	// NFSPath is the NFS export path for workspace content (optional).
	NFSPath string
	// StorageManager handles lazy workspace PVC creation.
	// When set, the reconciler will ensure workspace PVC exists before creating worker jobs
	// that mount the PVC. Ignored when NFSServer/NFSPath are set (direct NFS mount).
	StorageManager *workspace.StorageManager
	// TracingEnabled enables OTel tracing for arena worker pods.
	TracingEnabled bool
	// TracingEndpoint is the OTLP gRPC endpoint for arena worker tracing.
	TracingEndpoint string

	// MgmtPlaneTokenURL is the dashboard's /api/auth/service-token endpoint.
	// Injected onto worker pods as OMNIA_MGMT_PLANE_SERVICE_TOKEN_URL so they
	// can mint mgmt-plane JWTs to authenticate fleet-mode WS dials to agent
	// facades. Empty disables fleet dial auth.
	MgmtPlaneTokenURL string

	// WorkerServiceAccount, when set, is the ServiceAccount the worker pod
	// runs as instead of the per-job arena-worker SA the controller creates.
	// Point it at the workspace's runtime ServiceAccount so workers inherit
	// the workspace cloud identity (Azure Workload Identity, AWS IRSA, GKE
	// Workload Identity) — otherwise evaluations against keyless providers
	// (auth.type: workloadIdentity) cannot mint a platform token. The worker
	// Role is still created and bound to this SA so it keeps the namespace-
	// scoped CRD-read permissions workers need. Empty preserves the default
	// per-job arena-worker SA.
	WorkerServiceAccount string

	// WorkerPodLabels are extra labels stamped onto the worker pod template.
	// Used to opt the pod into a cloud identity webhook, e.g.
	// {"azure.workload.identity/use": "true"}.
	WorkerPodLabels map[string]string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
//nolint:gocognit // Reconcile functions inherently have high complexity due to state machine logic
func (r *ArenaJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ArenaJob", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ArenaJob instance
	arenaJob := &omniav1alpha1.ArenaJob{}
	if err := r.Get(ctx, req.NamespacedName, arenaJob); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ArenaJob resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get ArenaJob")
		return ctrl.Result{}, err
	}

	// Skip if job is already completed or cancelled
	if arenaJob.Status.Phase == omniav1alpha1.ArenaJobPhaseSucceeded ||
		arenaJob.Status.Phase == omniav1alpha1.ArenaJobPhaseFailed ||
		arenaJob.Status.Phase == omniav1alpha1.ArenaJobPhaseCancelled {
		log.V(1).Info("ArenaJob already completed, skipping", "phase", arenaJob.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Honour a cancellation request (spec.cancelled): delete the worker Job and
	// transition to the Cancelled phase. Runs after the terminal-phase skip, so
	// a finished job can't be cancelled. (#1329)
	if arenaJob.Spec.Cancelled {
		log.Info("ArenaJob cancellation requested, stopping worker", "name", arenaJob.Name)
		if err := r.deleteWorkerJob(ctx, arenaJob); err != nil {
			log.Error(err, "failed to delete worker job during cancellation")
			return ctrl.Result{}, err
		}
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseCancelled
		now := metav1.Now()
		arenaJob.Status.CompletionTime = &now
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing,
			metav1.ConditionFalse, "Cancelled", "Job cancelled by user request")
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady,
			metav1.ConditionFalse, "Cancelled", "Job cancelled by user request")
		if err := r.Status().Update(ctx, arenaJob); err != nil {
			log.Error(err, "failed to update status after cancellation")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Initialize status if needed
	if arenaJob.Status.Phase == "" {
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhasePending
	}

	// Update observed generation
	arenaJob.Status.ObservedGeneration = arenaJob.Generation

	// License check (defense in depth - webhooks are primary enforcement)
	if r.LicenseValidator != nil {
		jobType := string(arenaJob.Spec.Type)
		if jobType == "" {
			jobType = "evaluation"
		}
		replicas := 1
		if arenaJob.Spec.Workers != nil && arenaJob.Spec.Workers.Replicas > 0 {
			replicas = int(arenaJob.Spec.Workers.Replicas)
		}
		hasSchedule := arenaJob.Spec.Schedule != nil && arenaJob.Spec.Schedule.Cron != ""

		if err := r.LicenseValidator.ValidateArenaJob(ctx, jobType, replicas, hasSchedule); err != nil {
			log.Info("Job configuration not allowed by license",
				"type", jobType, "replicas", replicas, "hasSchedule", hasSchedule, "error", err)
			arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
			SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
				"LicenseViolation", err.Error())
			if r.Recorder != nil {
				r.Recorder.Event(arenaJob, corev1.EventTypeWarning, "LicenseViolation",
					fmt.Sprintf("Job configuration requires Enterprise license: %s", err.Error()))
			}
			if statusErr := r.Status().Update(ctx, arenaJob); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			return ctrl.Result{}, nil // Don't requeue - license must change
		}

		if r.LicenseValidator.IsDevMode() && r.Recorder != nil {
			r.Recorder.Event(arenaJob, corev1.EventTypeWarning, "DevModeLicense",
				"Using development license - not licensed for production use")
		}
	}

	// Check if we already have a K8s Job
	existingJob, err := r.getExistingJob(ctx, arenaJob)
	if err != nil {
		log.Error(err, "failed to check for existing job")
		return ctrl.Result{}, err
	}

	if existingJob == nil {
		// Validate the referenced ArenaSource only when creating a new job.
		// Once workers are running, the content is pinned to a specific version
		// via the volume subPath — re-validating would race with periodic refetches.
		source, err := r.validateSource(ctx, arenaJob)
		if err != nil {
			// A missing or not-yet-fetched ArenaSource is transient, not terminal:
			// keep the job Pending and requeue so it proceeds once the source
			// becomes available. Marking it Failed here would strand the job — the
			// terminal-phase skip guard at the top of Reconcile ignores the
			// ArenaSource watch that is meant to re-trigger it.
			log.Info("ArenaSource not ready, will retry",
				"source", arenaJob.Spec.SourceRef.Name, "reason", err.Error())
			r.handleSourceNotReady(ctx, arenaJob, err)
			return ctrl.Result{RequeueAfter: arenaSourceRetryInterval}, nil
		}
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeSourceValid, metav1.ConditionTrue,
			"SourceValid", fmt.Sprintf("ArenaSource %s is valid and ready", arenaJob.Spec.SourceRef.Name))

		// Create the K8s Job
		if err := r.createWorkerJob(ctx, arenaJob, source); err != nil {
			log.Error(err, "failed to create worker job")
			SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeJobCreated, metav1.ConditionFalse,
				"JobCreationFailed", err.Error())
			if statusErr := r.Status().Update(ctx, arenaJob); statusErr != nil {
				log.Error(statusErr, "failed to update status")
			}
			return ctrl.Result{}, err
		}

		// Update status
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseRunning
		now := metav1.Now()
		arenaJob.Status.StartTime = &now
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeJobCreated, metav1.ConditionTrue,
			"JobCreated", "Worker job created successfully")
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionTrue,
			"JobRunning", "Job is running")

		if r.Recorder != nil {
			r.Recorder.Event(arenaJob, corev1.EventTypeNormal, ArenaJobEventReasonJobCreated,
				"Created worker job")
		}
	} else {
		// Update status based on existing job
		r.updateStatusFromJob(ctx, arenaJob, existingJob)
	}

	if err := r.Status().Update(ctx, arenaJob); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled ArenaJob", "phase", arenaJob.Status.Phase)
	return ctrl.Result{}, nil
}

// validateSource fetches and validates the referenced ArenaSource.
func (r *ArenaJobReconciler) validateSource(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (*omniav1alpha1.ArenaSource, error) {
	source := &omniav1alpha1.ArenaSource{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      arenaJob.Spec.SourceRef.Name,
		Namespace: arenaJob.Namespace,
	}, source); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("arenaSource %s not found", arenaJob.Spec.SourceRef.Name)
		}
		return nil, fmt.Errorf("failed to get arenaSource %s: %w", arenaJob.Spec.SourceRef.Name, err)
	}

	// Source must have a fetched artifact. We don't require phase=Ready because
	// the source may be mid-refetch (phase=Fetching) while a valid artifact
	// from the previous fetch still exists on disk.
	if source.Status.Artifact == nil {
		return nil, fmt.Errorf("arenaSource %s has no artifact (phase: %s)", arenaJob.Spec.SourceRef.Name, source.Status.Phase)
	}

	return source, nil
}

// getExistingJob checks if a K8s Job already exists for this ArenaJob.
func (r *ArenaJobReconciler) getExistingJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      r.getJobName(arenaJob),
		Namespace: arenaJob.Namespace,
	}, job)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

// outputPVCMountPath is the container path where the output PVC is mounted.
const outputPVCMountPath = "/arena-output"

// resolvedProviderGroup holds the resolved CRDs and agent WebSocket URLs for a provider group.
type resolvedProviderGroup struct {
	providers []*corev1alpha1.Provider
	// agentWSURLs maps agentRef name to its resolved WebSocket URL.
	agentWSURLs map[string]string
	// mapMode indicates this group uses 1:1 config-provider-ID mapping (judges, self-play).
	// Map-mode groups don't participate in the scenario × provider work item matrix.
	mapMode bool
}

// findArenaJobsForSource maps ArenaSource changes to ArenaJob reconcile requests.
//
// When a field index is available (production, via SetupIndexers), the list is
// scoped by index. Otherwise falls back to list-all + local filter (envtest).
func (r *ArenaJobReconciler) findArenaJobsForSource(ctx context.Context, obj client.Object) []ctrl.Request {
	source, ok := obj.(*omniav1alpha1.ArenaSource)
	if !ok {
		return nil
	}
	log := logf.FromContext(ctx)

	// Try indexed list first; fall back to unscoped list if no index is registered.
	jobList := &omniav1alpha1.ArenaJobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(source.Namespace),
		client.MatchingFields{IndexArenaJobBySourceRef: source.Name},
	); err != nil {
		// MatchingFields fails with a raw client (no index). Fall back to list+filter.
		if err2 := r.List(ctx, jobList, client.InNamespace(source.Namespace)); err2 != nil {
			log.Error(err2, "failed to list ArenaJobs for ArenaSource watch")
			return nil
		}
		return filterArenaJobsBySource(jobList, source.Name)
	}

	return filterPendingArenaJobs(jobList)
}

// filterArenaJobsBySource filters a list of ArenaJobs to pending ones that
// reference the given ArenaSource name.
func filterArenaJobsBySource(list *omniav1alpha1.ArenaJobList, sourceName string) []ctrl.Request {
	var requests []ctrl.Request
	for _, job := range list.Items {
		if job.Spec.SourceRef.Name == sourceName {
			if job.Status.Phase == omniav1alpha1.ArenaJobPhasePending || job.Status.Phase == "" {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      job.Name,
						Namespace: job.Namespace,
					},
				})
			}
		}
	}
	return requests
}

// filterPendingArenaJobs returns reconcile requests for pending ArenaJobs from an indexed list.
func filterPendingArenaJobs(list *omniav1alpha1.ArenaJobList) []ctrl.Request {
	var requests []ctrl.Request
	for _, job := range list.Items {
		if job.Status.Phase == omniav1alpha1.ArenaJobPhasePending || job.Status.Phase == "" {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      job.Name,
					Namespace: job.Namespace,
				},
			})
		}
	}
	return requests
}

// findArenaJobsForJob maps K8s Job changes to ArenaJob reconcile requests.
func (r *ArenaJobReconciler) findArenaJobsForJob(_ context.Context, obj client.Object) []ctrl.Request {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil
	}

	// Check if this job is managed by an ArenaJob
	arenaJobName, ok := job.Labels["omnia.altairalabs.ai/job"]
	if !ok {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      arenaJobName,
				Namespace: job.Namespace,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.ArenaJob{}).
		Owns(&batchv1.Job{}).
		Watches(
			&omniav1alpha1.ArenaSource{},
			handler.EnqueueRequestsFromMapFunc(r.findArenaJobsForSource),
		).
		Named("arenajob").
		Complete(r)
}
