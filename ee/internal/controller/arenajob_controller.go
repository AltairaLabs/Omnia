/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/partitioner"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// Workspace label for namespace association
const labelWorkspace = "omnia.altairalabs.ai/workspace"

// maxWorkItems is the maximum number of scenario x provider work items allowed.
const maxWorkItems = 10000

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
	// Redis configuration for lazy connection
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	// RedisPasswordSecret is the name of the Kubernetes Secret containing the Redis password.
	// When set, worker pods receive the password via a secretKeyRef instead of a plain-text env var.
	// The secret must have a key named "redis-password".
	RedisPasswordSecret string
	// WorkspaceContentPath is the base path for workspace content volumes.
	// When set, workers mount the workspace content PVC and access content directly.
	// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}
	WorkspaceContentPath string
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

	// Validate the referenced ArenaSource
	source, err := r.validateSource(ctx, arenaJob)
	if err != nil {
		log.Error(err, "failed to validate ArenaSource")
		r.handleValidationError(ctx, arenaJob, ArenaJobConditionTypeSourceValid, err)
		return ctrl.Result{}, nil
	}
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeSourceValid, metav1.ConditionTrue,
		"SourceValid", fmt.Sprintf("ArenaSource %s is valid and ready", arenaJob.Spec.SourceRef.Name))

	// Check if we already have a K8s Job
	existingJob, err := r.getExistingJob(ctx, arenaJob)
	if err != nil {
		log.Error(err, "failed to check for existing job")
		return ctrl.Result{}, err
	}

	if existingJob == nil {
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

	// Check if source is ready
	if source.Status.Phase != omniav1alpha1.ArenaSourcePhaseReady {
		return nil, fmt.Errorf("arenaSource %s is not ready (phase: %s)", arenaJob.Spec.SourceRef.Name, source.Status.Phase)
	}

	// Verify source has an artifact
	if source.Status.Artifact == nil {
		return nil, fmt.Errorf("arenaSource %s has no artifact", arenaJob.Spec.SourceRef.Name)
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

// redisPasswordSecretKey is the key within the Kubernetes Secret that holds the Redis password.
const redisPasswordSecretKey = "redis-password"

// buildRedisPasswordEnvVar returns the REDIS_PASSWORD env var for worker pods.
// When RedisPasswordSecret is set, uses a secretKeyRef for secure injection.
// Falls back to plain-text value from RedisPassword for backward compatibility.
func (r *ArenaJobReconciler) buildRedisPasswordEnvVar() []corev1.EnvVar {
	if r.RedisPasswordSecret != "" {
		return []corev1.EnvVar{{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.RedisPasswordSecret,
					},
					Key: redisPasswordSecretKey,
				},
			},
		}}
	}
	if r.RedisPassword != "" {
		return []corev1.EnvVar{{
			Name:  "REDIS_PASSWORD",
			Value: r.RedisPassword,
		}}
	}
	return nil
}

// getJobName returns the name for the K8s Job.
func (r *ArenaJobReconciler) getJobName(arenaJob *omniav1alpha1.ArenaJob) string {
	return fmt.Sprintf("%s-worker", arenaJob.Name)
}

// getWorkerImage returns the worker image to use, with fallback to default.
func (r *ArenaJobReconciler) getWorkerImage() string {
	if r.WorkerImage != "" {
		return r.WorkerImage
	}
	return DefaultWorkerImage
}

// getWorkerImagePullPolicy returns the worker image pull policy, with fallback to IfNotPresent.
func (r *ArenaJobReconciler) getWorkerImagePullPolicy() corev1.PullPolicy {
	if r.WorkerImagePullPolicy != "" {
		return r.WorkerImagePullPolicy
	}
	return corev1.PullIfNotPresent
}

// getWorkerResources returns the resource requirements for the worker container.
// Uses the default resource requests/limits (100m/128Mi requests, 500m/512Mi limits).
func (r *ArenaJobReconciler) getWorkerResources(_ *omniav1alpha1.ArenaJob) corev1.ResourceRequirements {
	return defaultWorkerResources
}

// resolvedProviderGroup holds the resolved CRDs and agent WebSocket URLs for a provider group.
type resolvedProviderGroup struct {
	providers []*corev1alpha1.Provider
	// agentWSURLs maps agentRef name to its resolved WebSocket URL.
	agentWSURLs map[string]string
	// mapMode indicates this group uses 1:1 config-provider-ID mapping (judges, self-play).
	// Map-mode groups don't participate in the scenario × provider work item matrix.
	mapMode bool
}

// resolveProviderGroups resolves the new spec.Providers field.
// For each entry, it fetches the Provider CRD (providerRef) or validates the AgentRuntime (agentRef).
// Returns resolved groups and a flat list of all Provider CRDs (for env var injection).
func (r *ArenaJobReconciler) resolveProviderGroups(
	ctx context.Context, arenaJob *omniav1alpha1.ArenaJob,
) (map[string]*resolvedProviderGroup, []*corev1alpha1.Provider, error) {
	log := logf.FromContext(ctx)

	groups := make(map[string]*resolvedProviderGroup, len(arenaJob.Spec.Providers))
	var allProviders []*corev1alpha1.Provider
	seen := make(map[string]bool)

	for groupName, pg := range arenaJob.Spec.Providers {
		grp := &resolvedProviderGroup{
			agentWSURLs: make(map[string]string),
			mapMode:     pg.IsMapMode(),
		}

		for _, entry := range pg.AllEntries() {
			if entry.ProviderRef != nil {
				provider, err := r.resolveProviderEntry(ctx, arenaJob.Namespace, *entry.ProviderRef)
				if err != nil {
					return nil, nil, fmt.Errorf("group %q: %w", groupName, err)
				}
				grp.providers = append(grp.providers, provider)
				key := provider.Namespace + "/" + provider.Name
				if !seen[key] {
					seen[key] = true
					allProviders = append(allProviders, provider)
				}
			} else if entry.AgentRef != nil {
				wsURL, err := r.resolveAgentEntry(ctx, arenaJob.Namespace, entry.AgentRef.Name)
				if err != nil {
					return nil, nil, fmt.Errorf("group %q: %w", groupName, err)
				}
				grp.agentWSURLs[entry.AgentRef.Name] = wsURL
			}
		}

		groups[groupName] = grp
		log.V(1).Info("resolved provider group",
			"group", groupName,
			"providers", len(grp.providers),
			"agents", len(grp.agentWSURLs),
		)
	}

	return groups, allProviders, nil
}

// resolveProviderEntry fetches a Provider CRD from a ProviderRef.
func (r *ArenaJobReconciler) resolveProviderEntry(
	ctx context.Context, defaultNamespace string, ref corev1alpha1.ProviderRef,
) (*corev1alpha1.Provider, error) {
	ns := defaultNamespace
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}

	provider := &corev1alpha1.Provider{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, provider); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("provider %s/%s not found", ns, ref.Name)
		}
		return nil, fmt.Errorf("failed to get provider %s/%s: %w", ns, ref.Name, err)
	}
	return provider, nil
}

// resolveAgentEntry validates an AgentRuntime exists and is ready, returning its WebSocket URL.
func (r *ArenaJobReconciler) resolveAgentEntry(
	ctx context.Context, namespace, name string,
) (string, error) {
	agentRuntime := &corev1alpha1.AgentRuntime{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, agentRuntime); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("agentRuntime %s/%s not found", namespace, name)
		}
		return "", fmt.Errorf("failed to get agentRuntime %s/%s: %w", namespace, name, err)
	}

	if agentRuntime.Status.ServiceEndpoint == "" {
		return "", fmt.Errorf("agentRuntime %s/%s has no service endpoint (not ready)", namespace, name)
	}

	return fmt.Sprintf("ws://%s/ws?agent=%s&namespace=%s",
		agentRuntime.Status.ServiceEndpoint, name, namespace), nil
}

// buildProviderGroupEnvVars builds environment variables that encode the provider groups
// for the worker to read. The worker uses ARENA_JOB_NAME and ARENA_JOB_NAMESPACE
// to read the ArenaJob CRD directly and resolve providers itself.
// This method only adds agent WebSocket URLs as env vars since the worker needs them
// at startup before it can connect to the K8s API.
func buildProviderGroupEnvVars(groups map[string]*resolvedProviderGroup) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Encode agent WebSocket URLs as JSON for the worker
	agentURLs := make(map[string]string)
	for _, grp := range groups {
		for name, url := range grp.agentWSURLs {
			agentURLs[name] = url
		}
	}

	if len(agentURLs) > 0 {
		urlsJSON, err := json.Marshal(agentURLs)
		if err == nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ARENA_AGENT_WS_URLS",
				Value: string(urlsJSON),
			})
		}
	}

	return envVars
}

// getProviderIDsFromGroups extracts provider IDs from resolved provider groups.
// Only array-mode groups participate in the scenario × provider work item matrix.
// Map-mode groups (judges, self-play) are 1:1 config references and are excluded.
// Provider CRDs use their CRD name; agent entries use "agent-{name}".
func getProviderIDsFromGroups(groups map[string]*resolvedProviderGroup) []string {
	seen := make(map[string]bool)
	var ids []string

	for _, grp := range groups {
		if grp.mapMode {
			continue
		}
		for _, p := range grp.providers {
			if !seen[p.Name] {
				seen[p.Name] = true
				ids = append(ids, p.Name)
			}
		}
		for name := range grp.agentWSURLs {
			id := "agent-" + name
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}

	return ids
}

// buildProviderEnvVarsFromCRDs builds environment variables for Provider CRDs.
// This extracts credentials from each provider's secretRef.
// Delegates to the shared providers.BuildEnvVarsFromProviders function.
func (r *ArenaJobReconciler) buildProviderEnvVarsFromCRDs(providerCRDs []*corev1alpha1.Provider) []corev1.EnvVar {
	return providers.BuildEnvVarsFromProviders(providerCRDs)
}

// createWorkerJob creates a K8s Job for the Arena workers.
//
//nolint:gocognit,gocyclo // Pre-existing complexity, scheduled for refactoring
func (r *ArenaJobReconciler) createWorkerJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) error {
	log := logf.FromContext(ctx)

	// Create ServiceAccount + Role + RoleBinding for worker CRD reads (namespace-scoped)
	workerSAName, err := r.reconcileWorkerRBAC(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to reconcile worker RBAC: %w", err)
	}

	replicas := int32(1)
	if arenaJob.Spec.Workers != nil && arenaJob.Spec.Workers.Replicas > 0 {
		replicas = arenaJob.Spec.Workers.Replicas
	}

	// Ensure workspace PVC exists (lazy creation) when using PVC mode (not NFS direct mount)
	// This is only needed when:
	// 1. WorkspaceContentPath is set (filesystem mode enabled)
	// 2. NFSServer/NFSPath are NOT set (not using direct NFS mount)
	// 3. StorageManager is available
	useWorkspaceContent := r.WorkspaceContentPath != "" &&
		source.Status.Artifact != nil &&
		source.Status.Artifact.ContentPath != ""
	usePVCMount := r.NFSServer == "" || r.NFSPath == ""

	if useWorkspaceContent && usePVCMount && r.StorageManager != nil {
		workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)
		if _, err := r.StorageManager.EnsureWorkspacePVC(ctx, workspaceName); err != nil {
			log.Error(err, "failed to ensure workspace PVC exists", "workspace", workspaceName)
			return fmt.Errorf("failed to ensure workspace PVC: %w", err)
		}
		log.V(1).Info("workspace PVC ensured", "workspace", workspaceName)
	}

	// Resolve providers directly from CRD refs
	resolvedGroups, providerCRDs, err := r.resolveProviderGroups(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to resolve provider groups: %w", err)
	}
	log.Info("resolved provider groups from CRDs",
		"groups", len(resolvedGroups),
		"providerCRDs", len(providerCRDs),
	)

	// Validate that spec.providers covers all groups required by the arena config.
	// This catches misconfiguration early (controller-side) instead of failing in the worker.
	basePath := r.getContentBasePath(ctx, arenaJob, source)
	if configPath := r.getArenaConfigPath(arenaJob, basePath); configPath != "" {
		if validationMsg := r.validateProviderGroups(arenaJob, configPath); validationMsg != "" {
			return fmt.Errorf("provider group validation failed: %s", validationMsg)
		}
	}

	// Determine arena file path
	arenaFile := arenaJob.Spec.ArenaFile
	if arenaFile == "" {
		arenaFile = "config.arena.yaml"
	}

	// Build environment variables
	env := []corev1.EnvVar{
		{
			Name:  "ARENA_JOB_NAME",
			Value: arenaJob.Name,
		},
		{
			Name:  "ARENA_JOB_NAMESPACE",
			Value: arenaJob.Namespace,
		},
		{
			Name:  "ARENA_SOURCE_NAME",
			Value: source.Name,
		},
		{
			Name:  "ARENA_FILE",
			Value: arenaFile,
		},
		{
			Name:  "ARENA_JOB_TYPE",
			Value: string(arenaJob.Spec.Type),
		},
	}

	// Add Redis configuration if available
	if r.RedisAddr != "" {
		env = append(env, corev1.EnvVar{
			Name:  "REDIS_ADDR",
			Value: r.RedisAddr,
		})
	}
	env = append(env, r.buildRedisPasswordEnvVar()...)

	// Add verbose flag for debug logging
	if arenaJob.Spec.Verbose {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_VERBOSE",
			Value: "true",
		}, corev1.EnvVar{
			Name:  "LOG_LEVEL",
			Value: "debug",
		})
	}

	// Extract root path from arenaFile (directory containing the config file)
	// This is used to restrict the volume mount to only the job's root folder
	var rootPath string
	if arenaJob.Spec.ArenaFile != "" {
		rootPath = filepath.Dir(arenaJob.Spec.ArenaFile)
		if rootPath == "." {
			rootPath = "" // No subdirectory, use the whole content
		}
	}

	// Calculate the volume subPath for content isolation
	// Structure: {workspace}/{namespace}/{contentPath}/{rootPath}
	var contentSubPath string
	if source.Status.Artifact != nil && source.Status.Artifact.ContentPath != "" {
		workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)
		contentSubPath = fmt.Sprintf("%s/%s/%s",
			workspaceName, arenaJob.Namespace, source.Status.Artifact.ContentPath)
		if rootPath != "" {
			contentSubPath = contentSubPath + "/" + rootPath
		}
	}

	// Add source content info if available (filesystem-based content access)
	if source.Status.Artifact != nil {
		if source.Status.Artifact.ContentPath != "" {
			// Content path is now just the mount point since subPath handles isolation
			env = append(env, corev1.EnvVar{
				Name:  "ARENA_CONTENT_PATH",
				Value: "/workspace-content",
			})
			// Store the arena config filename (not the full path since we're in root folder)
			if arenaJob.Spec.ArenaFile != "" {
				arenaFileName := filepath.Base(arenaJob.Spec.ArenaFile)
				env = append(env, corev1.EnvVar{
					Name:  "ARENA_CONFIG_FILE",
					Value: arenaFileName,
				})
			}
		}
		if source.Status.Artifact.Version != "" {
			env = append(env, corev1.EnvVar{
				Name:  "ARENA_CONTENT_VERSION",
				Value: source.Status.Artifact.Version,
			})
		}
	}

	// Add provider credential environment variables
	if len(providerCRDs) > 0 {
		log.Info("injecting provider credentials", "count", len(providerCRDs))
		for _, p := range providerCRDs {
			log.V(1).Info("provider",
				"name", p.Name,
				"type", p.Spec.Type,
				"model", p.Spec.Model,
			)
		}
		env = append(env, r.buildProviderEnvVarsFromCRDs(providerCRDs)...)
	}

	// Add platform environment variables for hyperscaler providers
	env = append(env, providers.BuildPlatformEnvVars(providerCRDs)...)

	// Pass agent WebSocket URLs directly (worker needs them before K8s API access)
	env = append(env, buildProviderGroupEnvVars(resolvedGroups)...)

	// Add tracing env vars for arena worker pods
	if r.TracingEnabled && r.TracingEndpoint != "" {
		env = append(env,
			corev1.EnvVar{Name: "TRACING_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "TRACING_ENDPOINT", Value: r.TracingEndpoint},
			corev1.EnvVar{Name: "TRACING_INSECURE", Value: "true"},
		)
	}

	// Build volumes list
	volumes := []corev1.Volume{
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Build volume mounts list
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	// Add workspace content volume if using filesystem mode
	if useWorkspaceContent {
		var volumeSource corev1.VolumeSource
		if r.NFSServer != "" && r.NFSPath != "" {
			// Use NFS directly - enables shared access across namespaces
			// without requiring per-workspace PVCs
			volumeSource = corev1.VolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server:   r.NFSServer,
					Path:     r.NFSPath,
					ReadOnly: true,
				},
			}
		} else {
			// Use per-workspace PVC (requires NFS-backed storage class for RWX)
			pvcName := fmt.Sprintf("workspace-%s-content", arenaJob.Namespace)
			volumeSource = corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
					ReadOnly:  true, // Workers only read content
				},
			}
		}
		volumes = append(volumes, corev1.Volume{
			Name:         "workspace-content",
			VolumeSource: volumeSource,
		})
		// Use subPath to restrict access to only the job's root folder
		// This provides content isolation between jobs
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "workspace-content",
			MountPath: "/workspace-content",
			SubPath:   contentSubPath,
			ReadOnly:  true,
		})
		log.Info("mounting content with isolation", "subPath", contentSubPath)
	}

	// Create the Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getJobName(arenaJob),
			Namespace: arenaJob.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "arena-worker",
				"app.kubernetes.io/instance":   arenaJob.Name,
				"app.kubernetes.io/component":  "worker",
				"app.kubernetes.io/managed-by": "omnia-operator",
				"omnia.altairalabs.ai/job":     arenaJob.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism: &replicas,
			Completions: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "arena-worker",
						"app.kubernetes.io/instance":  arenaJob.Name,
						"app.kubernetes.io/component": "worker",
						"omnia.altairalabs.ai/job":    arenaJob.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(65532)), // nonroot user
						FSGroup:      ptr.To(int64(65532)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Volumes: volumes,
					Containers: []corev1.Container{
						{
							Name:            "worker",
							Image:           r.getWorkerImage(),
							ImagePullPolicy: r.getWorkerImagePullPolicy(),
							Env:             env,
							Resources:       r.getWorkerResources(arenaJob),
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}

	// Set ServiceAccountName for CRD reads (created by reconcileWorkerRBAC above)
	job.Spec.Template.Spec.ServiceAccountName = workerSAName

	// Set TTL for automatic cleanup after completion (default: 1 hour)
	if arenaJob.Spec.TTLSecondsAfterFinished != nil {
		job.Spec.TTLSecondsAfterFinished = arenaJob.Spec.TTLSecondsAfterFinished
	} else {
		defaultTTL := int32(3600)
		job.Spec.TTLSecondsAfterFinished = &defaultTTL
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(arenaJob, job, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	log.Info("creating worker job", "job", job.Name, "replicas", replicas)
	if err := r.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	// Enqueue work items (lazily connects to queue if configured)
	workItemCount, enqueueErr := r.enqueueWorkItems(ctx, arenaJob, source, providerCRDs, resolvedGroups)
	if enqueueErr != nil {
		log.Error(enqueueErr, "failed to enqueue work items")
		// Don't return error - job is created, workers will wait for items
	}

	// Set initial progress based on work item count
	if workItemCount > 0 {
		arenaJob.Status.Progress = &omniav1alpha1.JobProgress{
			Total:   int32(workItemCount),
			Pending: int32(workItemCount),
		}
	}

	return nil
}

// getOrCreateQueue returns the work queue, creating it lazily if needed.
func (r *ArenaJobReconciler) getOrCreateQueue() (queue.WorkQueue, error) {
	// Return existing queue if already connected
	if r.Queue != nil {
		return r.Queue, nil
	}

	// No Redis configured
	if r.RedisAddr == "" {
		return nil, nil
	}

	// Try to connect lazily
	q, err := queue.NewRedisQueue(queue.RedisOptions{
		Addr:     r.RedisAddr,
		Password: r.RedisPassword,
		DB:       r.RedisDB,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	r.Queue = q
	return q, nil
}

// getContentBasePath computes the filesystem base path for workspace content.
// Returns empty string if filesystem access is not configured.
func (r *ArenaJobReconciler) getContentBasePath(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) string {
	if r.WorkspaceContentPath == "" {
		return ""
	}
	if source.Status.Artifact == nil || source.Status.Artifact.ContentPath == "" {
		return ""
	}

	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)

	// Extract root path from arenaFile
	var rootPath string
	if arenaJob.Spec.ArenaFile != "" {
		rootPath = filepath.Dir(arenaJob.Spec.ArenaFile)
		if rootPath == "." {
			rootPath = ""
		}
	}

	// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}/{rootPath}
	basePath := filepath.Join(r.WorkspaceContentPath, workspaceName, arenaJob.Namespace, source.Status.Artifact.ContentPath)
	if rootPath != "" {
		basePath = filepath.Join(basePath, rootPath)
	}
	return basePath
}

// listScenarios attempts to enumerate scenarios from the arena config file on the filesystem.
// Returns nil if filesystem access is unavailable or scenarios cannot be enumerated.
func (r *ArenaJobReconciler) listScenarios(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) ([]partitioner.Scenario, error) {
	basePath := r.getContentBasePath(ctx, arenaJob, source)
	if basePath == "" {
		return nil, nil
	}

	arenaFile := arenaJob.Spec.ArenaFile
	if arenaFile == "" {
		arenaFile = "config.arena.yaml"
	}
	// Use just the filename since basePath already includes rootPath
	arenaFileName := filepath.Base(arenaFile)
	fullPath := filepath.Join(basePath, arenaFileName)

	scenarios, err := partitioner.ListScenariosFromConfig(fullPath)
	if err != nil {
		return nil, err
	}

	return scenarios, nil
}

// buildMatrixWorkItems creates scenario × provider work items using the partitioner.
// Returns nil if partitioning fails or inputs are empty.
func (r *ArenaJobReconciler) buildMatrixWorkItems(ctx context.Context, jobName, bundleURL string, scenarios []partitioner.Scenario, providerCRDs []*corev1alpha1.Provider) []queue.WorkItem {
	log := logf.FromContext(ctx)

	partProviders := make([]partitioner.Provider, len(providerCRDs))
	for i, p := range providerCRDs {
		partProviders[i] = partitioner.Provider{
			ID:        p.Name,
			Name:      p.Name,
			Namespace: p.Namespace,
		}
	}

	result, err := partitioner.Partition(partitioner.PartitionInput{
		JobID:      jobName,
		BundleURL:  bundleURL,
		Scenarios:  scenarios,
		Providers:  partProviders,
		MaxRetries: 3,
	})
	if err != nil {
		log.Error(err, "partitioning failed, falling back to per-provider mode")
		return nil
	}

	if result.TotalCombinations > maxWorkItems {
		log.Error(nil, "work item matrix exceeds limit, falling back to per-provider mode",
			"totalCombinations", result.TotalCombinations,
			"maxWorkItems", maxWorkItems)
		return nil
	}

	now := time.Now()
	for i := range result.Items {
		result.Items[i].Status = queue.ItemStatusPending
		result.Items[i].Attempt = 1
		result.Items[i].CreatedAt = now
	}
	log.Info("created scenario × provider work items",
		"scenarios", result.ScenarioCount,
		"providers", result.ProviderCount,
		"items", result.TotalCombinations)
	return result.Items
}

// buildFallbackWorkItems creates per-provider work items (or a single default item).
// Used when scenario enumeration is not available.
func buildFallbackWorkItems(jobName, bundleURL string, providerIDs []string) []queue.WorkItem {
	now := time.Now()
	if len(providerIDs) == 0 {
		return []queue.WorkItem{{
			ID:          fmt.Sprintf("%s-default-0", jobName),
			JobID:       jobName,
			ScenarioID:  "default",
			ProviderID:  "",
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		}}
	}

	items := make([]queue.WorkItem, 0, len(providerIDs))
	for i, provider := range providerIDs {
		items = append(items, queue.WorkItem{
			ID:          fmt.Sprintf("%s-%s-%d", jobName, provider, i),
			JobID:       jobName,
			ScenarioID:  "default",
			ProviderID:  provider,
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		})
	}
	return items
}

// enqueueWorkItems creates and enqueues work items for the Arena job.
// When scenarios can be enumerated from the filesystem, work items are created
// for each scenario × provider combination for maximum parallelism.
// Falls back to per-provider items (with ScenarioID "default") when filesystem
// access is unavailable.
// Returns the number of work items enqueued and any error.
func (r *ArenaJobReconciler) enqueueWorkItems(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	source *omniav1alpha1.ArenaSource,
	providerCRDs []*corev1alpha1.Provider,
	resolvedGroups map[string]*resolvedProviderGroup,
) (int, error) {
	log := logf.FromContext(ctx)

	// Get queue (lazily connect if needed)
	q, err := r.getOrCreateQueue()
	if err != nil {
		return 0, err
	}
	if q == nil {
		log.Info("no work queue configured, skipping work item enqueueing")
		return 0, nil
	}

	// Get bundle URL from source artifact
	bundleURL := ""
	if source.Status.Artifact != nil {
		bundleURL = source.Status.Artifact.URL
	}

	// Attempt scenario enumeration from the filesystem
	scenarios, scenarioErr := r.listScenarios(ctx, arenaJob, source)
	if scenarioErr != nil {
		log.V(1).Info("could not enumerate scenarios, falling back to per-provider mode", "error", scenarioErr)
	}

	// Apply scenario filters if scenarios were enumerated
	if len(scenarios) > 0 && arenaJob.Spec.Scenarios != nil {
		filtered, filterErr := partitioner.Filter(scenarios, arenaJob.Spec.Scenarios.Include, arenaJob.Spec.Scenarios.Exclude)
		if filterErr != nil {
			log.Error(filterErr, "failed to filter scenarios, using unfiltered list")
		} else {
			scenarios = filtered
		}
	}

	// Build work items: unified scenario × provider matrix
	providerIDs := getProviderIDsFromGroups(resolvedGroups)
	log.V(1).Info("building work items", "providerIDs", providerIDs)

	var items []queue.WorkItem
	if len(scenarios) > 0 && len(providerCRDs) > 0 {
		items = r.buildMatrixWorkItems(ctx, arenaJob.Name, bundleURL, scenarios, providerCRDs)
	}
	if len(items) == 0 {
		items = buildFallbackWorkItems(arenaJob.Name, bundleURL, providerIDs)
	}

	log.Info("enqueueing work items", "count", len(items))
	if err := q.Push(ctx, arenaJob.Name, items); err != nil {
		return 0, fmt.Errorf("failed to push work items to queue: %w", err)
	}

	return len(items), nil
}

// updateStatusFromJob updates the ArenaJob status based on the K8s Job status.
//
//nolint:gocognit // Status update functions inherently handle many conditions
func (r *ArenaJobReconciler) updateStatusFromJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, job *batchv1.Job) {
	log := logf.FromContext(ctx)

	// Update active workers count
	arenaJob.Status.ActiveWorkers = job.Status.Active

	// Update progress
	if arenaJob.Status.Progress == nil {
		arenaJob.Status.Progress = &omniav1alpha1.JobProgress{}
	}

	completions := int32(1)
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}
	arenaJob.Status.Progress.Total = completions
	arenaJob.Status.Progress.Completed = job.Status.Succeeded
	arenaJob.Status.Progress.Failed = job.Status.Failed
	arenaJob.Status.Progress.Pending = completions - job.Status.Succeeded - job.Status.Failed

	// Check job conditions
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				now := metav1.Now()
				arenaJob.Status.CompletionTime = &now

				// Aggregate results from queue if aggregator is available
				// The aggregated results determine actual success/failure based on test outcomes
				var hasTestFailures bool
				var hasAggregation bool
				var passedItems, failedItems int
				if r.Aggregator != nil {
					log.V(1).Info("aggregating results from queue", "jobID", arenaJob.Name)
					result, err := r.Aggregator.Aggregate(ctx, arenaJob.Name)
					if err != nil {
						log.Error(err, "failed to aggregate results")
					} else {
						hasAggregation = true
						log.V(1).Info("aggregation complete",
							"totalItems", result.TotalItems,
							"passedItems", result.PassedItems,
							"failedItems", result.FailedItems)
						arenaJob.Status.Result = r.Aggregator.ToJobResult(result)
						// Check if any tests actually failed
						hasTestFailures = result.FailedItems > 0
						passedItems = result.PassedItems
						failedItems = result.FailedItems
					}
				} else {
					log.V(1).Info("aggregator not available, skipping result aggregation")
				}

				// Set phase based on aggregated test results, not just K8s job completion
				if hasTestFailures {
					arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
						"TestsFailed", "Job completed but some tests failed")
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
						"Failed", "Job completed but some tests failed")
					if r.Recorder != nil {
						r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
							"Job completed but some tests failed")
					}
					log.Info("job completed with test failures",
						"passed", passedItems,
						"failed", failedItems)
				} else if hasAggregation && passedItems == 0 {
					arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
						"NoTestsRan", "Job completed but no tests produced results")
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
						"Failed", "Job completed but no tests produced results")
					if r.Recorder != nil {
						r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
							"Job completed but no tests produced results")
					}
					log.Info("job completed with no test results",
						"passed", passedItems,
						"failed", failedItems)
				} else {
					arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseSucceeded
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
						"JobSucceeded", "Job completed successfully")
					SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionTrue,
						"Succeeded", "Job completed successfully")
					if r.Recorder != nil {
						r.Recorder.Event(arenaJob, corev1.EventTypeNormal, ArenaJobEventReasonJobSucceeded,
							"Job completed successfully")
					}
					log.Info("job completed successfully",
						"passed", passedItems)
				}
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
				now := metav1.Now()
				arenaJob.Status.CompletionTime = &now
				SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
					"JobFailed", condition.Message)
				SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
					"Failed", condition.Message)
				if r.Recorder != nil {
					r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
						fmt.Sprintf("Job failed: %s", condition.Message))
				}
				log.Info("job failed", "reason", condition.Reason, "message", condition.Message)
			}
		}
	}

	// If job is still running
	if arenaJob.Status.Phase == omniav1alpha1.ArenaJobPhaseRunning {
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionTrue,
			"JobRunning", fmt.Sprintf("Job running: %d/%d completed", job.Status.Succeeded, completions))
	}
}

// handleValidationError handles errors during validation.
func (r *ArenaJobReconciler) handleValidationError(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, conditionType string, err error) {
	log := logf.FromContext(ctx)

	arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, conditionType, metav1.ConditionFalse, "ValidationFailed", err.Error())
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
		"ValidationFailed", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonConfigNotReady, err.Error())
	}

	if statusErr := r.Status().Update(ctx, arenaJob); statusErr != nil {
		log.Error(statusErr, "failed to update status after validation error")
	}
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
