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
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
	"github.com/altairalabs/omnia/ee/pkg/arena/partitioner"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/selector"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// Workspace label for namespace association
const labelWorkspace = "omnia.altairalabs.ai/workspace"

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
	// WorkerServiceAccountName is the ServiceAccount name for worker pods.
	// Used for workload identity authentication with hyperscaler providers.
	// When set and a provider uses workloadIdentity auth, worker pods will use this SA.
	WorkerServiceAccountName string
}

// getWorkspaceForNamespace looks up the workspace name from a namespace's labels.
// Returns the namespace name as fallback if workspace label is not found.
func (r *ArenaJobReconciler) getWorkspaceForNamespace(ctx context.Context, namespace string) string {
	// Handle nil client (e.g., in tests that don't set up the client)
	if r.Client == nil {
		return namespace
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		// Fallback to namespace name if we can't look it up
		return namespace
	}
	if wsName, ok := ns.Labels[labelWorkspace]; ok && wsName != "" {
		return wsName
	}
	// Fallback to namespace name
	return namespace
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
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
			r.setCondition(arenaJob, ArenaJobConditionTypeReady, metav1.ConditionFalse,
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
	r.setCondition(arenaJob, ArenaJobConditionTypeSourceValid, metav1.ConditionTrue,
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
			r.setCondition(arenaJob, ArenaJobConditionTypeJobCreated, metav1.ConditionFalse,
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
		r.setCondition(arenaJob, ArenaJobConditionTypeJobCreated, metav1.ConditionTrue,
			"JobCreated", "Worker job created successfully")
		r.setCondition(arenaJob, ArenaJobConditionTypeProgressing, metav1.ConditionTrue,
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

// getWorkerServiceAccountName returns the ServiceAccount name for worker pods if any
// provider uses workload identity authentication. Returns empty string if not needed.
func (r *ArenaJobReconciler) getWorkerServiceAccountName(providerCRDs []*corev1alpha1.Provider) string {
	if r.WorkerServiceAccountName == "" {
		return ""
	}
	for _, p := range providerCRDs {
		if p.Spec.Auth != nil && p.Spec.Auth.Type == corev1alpha1.AuthMethodWorkloadIdentity {
			return r.WorkerServiceAccountName
		}
	}
	return ""
}

// isFleetMode returns true if the ArenaJob is configured for fleet execution mode.
func isFleetMode(arenaJob *omniav1alpha1.ArenaJob) bool {
	return arenaJob.Spec.Execution != nil && arenaJob.Spec.Execution.Mode == omniav1alpha1.ExecutionModeFleet
}

// resolveFleetTarget resolves the fleet target AgentRuntime to a WebSocket URL.
// It reads the AgentRuntime's status.serviceEndpoint and constructs a ws:// URL.
func (r *ArenaJobReconciler) resolveFleetTarget(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (string, error) {
	if arenaJob.Spec.Execution == nil || arenaJob.Spec.Execution.Target == nil {
		return "", fmt.Errorf("fleet target not specified")
	}

	target := arenaJob.Spec.Execution.Target
	targetNamespace := target.Namespace
	if targetNamespace == "" {
		targetNamespace = arenaJob.Namespace
	}

	agentRuntime := &corev1alpha1.AgentRuntime{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      target.AgentRuntimeRef.Name,
		Namespace: targetNamespace,
	}, agentRuntime); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("agentRuntime %s/%s not found", targetNamespace, target.AgentRuntimeRef.Name)
		}
		return "", fmt.Errorf("failed to get agentRuntime %s/%s: %w", targetNamespace, target.AgentRuntimeRef.Name, err)
	}

	if agentRuntime.Status.ServiceEndpoint == "" {
		return "", fmt.Errorf("agentRuntime %s/%s has no service endpoint (not ready)", targetNamespace, target.AgentRuntimeRef.Name)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", agentRuntime.Status.ServiceEndpoint)
	return wsURL, nil
}

// resolveProviderOverrides resolves provider CRDs based on ArenaJob's providerOverrides.
// Returns providers grouped by their selector group name (e.g., "default", "judge").
// If no overrides are specified, returns nil (use ArenaConfig providers).
func (r *ArenaJobReconciler) resolveProviderOverrides(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (map[string][]*corev1alpha1.Provider, error) {
	if len(arenaJob.Spec.ProviderOverrides) == 0 {
		return nil, nil
	}

	log := logf.FromContext(ctx)
	log.V(1).Info("resolving provider overrides", "overrides", len(arenaJob.Spec.ProviderOverrides))

	// Resolve all provider overrides (returns map of group -> providers)
	resolvedByGroup, err := selector.ResolveProviderOverrides(ctx, r.Client, arenaJob.Namespace, arenaJob.Spec.ProviderOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve provider overrides: %w", err)
	}

	totalCount := 0
	for group, groupProviders := range resolvedByGroup {
		log.V(1).Info("resolved providers for group", "group", group, "count", len(groupProviders))
		totalCount += len(groupProviders)
	}

	log.Info("resolved provider overrides", "groups", len(resolvedByGroup), "totalProviders", totalCount)
	return resolvedByGroup, nil
}

// buildProviderEnvVarsFromCRDs builds environment variables for Provider CRDs.
// This extracts credentials from each provider's secretRef.
// Delegates to the shared providers.BuildEnvVarsFromProviders function.
func (r *ArenaJobReconciler) buildProviderEnvVarsFromCRDs(providerCRDs []*corev1alpha1.Provider) []corev1.EnvVar {
	return providers.BuildEnvVarsFromProviders(providerCRDs)
}

// getProviderIDsFromCRDs extracts provider IDs from Provider CRDs for work queue.
func (r *ArenaJobReconciler) getProviderIDsFromCRDs(providerCRDs []*corev1alpha1.Provider) []string {
	ids := make([]string, len(providerCRDs))
	for i, p := range providerCRDs {
		ids[i] = p.Name
	}
	return ids
}

// resolveBindingRegistry lists all Provider CRDs in the given namespace and converts them
// to a binding registry for annotation-based credential resolution. Returns the registry
// map and the list of Provider CRDs for env var injection.
func (r *ArenaJobReconciler) resolveBindingRegistry(ctx context.Context, namespace string) (map[string]overrides.ProviderOverride, []*corev1alpha1.Provider, error) {
	log := logf.FromContext(ctx)

	providerList := &corev1alpha1.ProviderList{}
	if err := r.List(ctx, providerList, client.InNamespace(namespace)); err != nil {
		return nil, nil, fmt.Errorf("failed to list providers in namespace %s: %w", namespace, err)
	}

	if len(providerList.Items) == 0 {
		return nil, nil, nil
	}

	registry := make(map[string]overrides.ProviderOverride, len(providerList.Items))
	providerCRDs := make([]*corev1alpha1.Provider, 0, len(providerList.Items))

	for i := range providerList.Items {
		p := &providerList.Items[i]
		key := p.Namespace + "/" + p.Name
		registry[key] = convertProviderToOverride(p)
		providerCRDs = append(providerCRDs, p)
	}

	log.V(1).Info("resolved binding registry", "namespace", namespace, "providers", len(registry))
	return registry, providerCRDs, nil
}

// deduplicateProviders merges additional providers into an existing list, skipping duplicates
// by namespace/name. This ensures env vars are injected for all relevant providers.
func deduplicateProviders(existing, additional []*corev1alpha1.Provider) []*corev1alpha1.Provider {
	if len(additional) == 0 {
		return existing
	}

	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p.Namespace+"/"+p.Name] = true
	}

	result := make([]*corev1alpha1.Provider, len(existing))
	copy(result, existing)

	for _, p := range additional {
		key := p.Namespace + "/" + p.Name
		if !seen[key] {
			result = append(result, p)
			seen[key] = true
		}
	}

	return result
}

// resolveToolRegistryOverride resolves tool registry CRDs based on ArenaJob's toolRegistryOverride.
// Returns the resolved tool overrides configuration for the worker.
func (r *ArenaJobReconciler) resolveToolRegistryOverride(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (map[string]selector.ToolOverrideConfig, error) {
	if arenaJob.Spec.ToolRegistryOverride == nil {
		return nil, nil
	}

	log := logf.FromContext(ctx)
	log.Info("resolving tool registry override")

	// Resolve tool registries matching the selector
	registries, err := selector.ResolveToolRegistryOverride(ctx, r.Client, arenaJob.Namespace, arenaJob.Spec.ToolRegistryOverride)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tool registry override: %w", err)
	}

	if len(registries) == 0 {
		log.Info("no tool registries matched the selector")
		return nil, nil
	}

	// Log resolved registries
	registryNames := make([]string, len(registries))
	for i, reg := range registries {
		registryNames[i] = reg.Name
	}
	log.Info("resolved tool registries", "registries", registryNames, "count", len(registries))

	// Extract tool override configurations
	toolOverrides := selector.GetToolOverridesFromRegistries(registries)

	// Log individual tool overrides
	for toolName, config := range toolOverrides {
		log.Info("tool override configured",
			"tool", toolName,
			"registry", config.RegistryName,
			"handler", config.HandlerName,
			"endpoint", config.Endpoint,
			"type", config.HandlerType,
		)
	}

	log.Info("resolved tool overrides", "totalTools", len(toolOverrides))
	return toolOverrides, nil
}

// getOverrideConfigMapName returns the name for the override ConfigMap.
func getOverrideConfigMapName(jobName string) string {
	return fmt.Sprintf("%s-overrides", jobName)
}

// convertProviderToOverride converts a Provider CRD to a ProviderOverride struct.
func convertProviderToOverride(p *corev1alpha1.Provider) overrides.ProviderOverride {
	override := overrides.ProviderOverride{
		ID:      p.Name,
		Type:    string(p.Spec.Type),
		Model:   p.Spec.Model,
		BaseURL: p.Spec.BaseURL,
	}

	// Set credential configuration
	if p.Spec.Credential != nil {
		if p.Spec.Credential.SecretRef != nil {
			// credential.secretRef — use the standard env var name
			secretRefs := providers.GetSecretRefsForProvider(string(p.Spec.Type))
			if len(secretRefs) > 0 {
				override.SecretEnvVar = secretRefs[0].EnvVar
			}
		} else if p.Spec.Credential.EnvVar != "" {
			// credential.envVar — the env var is pre-injected, pass it through
			override.SecretEnvVar = p.Spec.Credential.EnvVar
		} else if p.Spec.Credential.FilePath != "" {
			// credential.filePath — pass to worker via override config
			override.CredentialFile = p.Spec.Credential.FilePath
		}
	} else if p.Spec.SecretRef != nil {
		// Legacy secretRef
		secretRefs := providers.GetSecretRefsForProvider(string(p.Spec.Type))
		if len(secretRefs) > 0 {
			override.SecretEnvVar = secretRefs[0].EnvVar
		}
	}

	// Set platform configuration
	if p.Spec.Platform != nil {
		override.Platform = &overrides.PlatformOverride{
			Type:     string(p.Spec.Platform.Type),
			Region:   p.Spec.Platform.Region,
			Project:  p.Spec.Platform.Project,
			Endpoint: p.Spec.Platform.Endpoint,
		}
	}

	// Set auth configuration
	if p.Spec.Auth != nil {
		override.AuthMethod = string(p.Spec.Auth.Type)
		override.RoleARN = p.Spec.Auth.RoleArn
		override.ServiceAccountEmail = p.Spec.Auth.ServiceAccountEmail
	}

	// Set defaults if specified
	if p.Spec.Defaults != nil {
		if p.Spec.Defaults.Temperature != nil {
			if temp, err := strconv.ParseFloat(*p.Spec.Defaults.Temperature, 64); err == nil {
				override.Temperature = temp
			}
		}
		if p.Spec.Defaults.TopP != nil {
			if topP, err := strconv.ParseFloat(*p.Spec.Defaults.TopP, 64); err == nil {
				override.TopP = topP
			}
		}
		if p.Spec.Defaults.MaxTokens != nil {
			override.MaxTokens = int(*p.Spec.Defaults.MaxTokens)
		}
	}

	return override
}

// convertToolOverrides converts tool overrides to the override config format.
func convertToolOverrides(toolOverrides map[string]selector.ToolOverrideConfig) []overrides.ToolOverride {
	if len(toolOverrides) == 0 {
		return nil
	}

	tools := make([]overrides.ToolOverride, 0, len(toolOverrides))
	for _, t := range toolOverrides {
		tools = append(tools, overrides.ToolOverride{
			Name:         t.Name,
			Description:  t.Description,
			Endpoint:     t.Endpoint,
			HandlerType:  t.HandlerType,
			RegistryName: t.RegistryName,
			HandlerName:  t.HandlerName,
		})
	}
	return tools
}

// buildOverrideConfig creates the override config from resolved CRDs.
// providersByGroup maps group name (e.g., "default", "judge") to Provider CRDs.
// toolOverrides maps tool name to its override configuration.
func (r *ArenaJobReconciler) buildOverrideConfig(
	ctx context.Context,
	providersByGroup map[string][]*corev1alpha1.Provider,
	toolOverrides map[string]selector.ToolOverrideConfig,
) *overrides.OverrideConfig {
	log := logf.FromContext(ctx)

	// If no overrides, return nil (worker will use arena config providers)
	if len(providersByGroup) == 0 && len(toolOverrides) == 0 {
		return nil
	}

	cfg := &overrides.OverrideConfig{
		Providers: make(map[string][]overrides.ProviderOverride),
	}

	// Convert Provider CRDs to ProviderOverride structs
	for groupName, groupProviders := range providersByGroup {
		overrideList := make([]overrides.ProviderOverride, 0, len(groupProviders))
		for _, p := range groupProviders {
			override := convertProviderToOverride(p)
			overrideList = append(overrideList, override)
			log.V(1).Info("added provider override",
				"group", groupName,
				"id", override.ID,
				"type", override.Type,
				"model", override.Model,
			)
		}
		cfg.Providers[groupName] = overrideList
	}

	// Convert tool overrides
	cfg.Tools = convertToolOverrides(toolOverrides)

	return cfg
}

// createOverrideConfigMap creates or updates the ConfigMap containing provider/tool overrides.
// The ConfigMap is owned by the ArenaJob and will be garbage collected when the job is deleted.
func (r *ArenaJobReconciler) createOverrideConfigMap(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	config *overrides.OverrideConfig,
) error {
	log := logf.FromContext(ctx)

	// Marshal config to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal override config: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getOverrideConfigMapName(arenaJob.Name),
			Namespace: arenaJob.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "arena-overrides",
				"app.kubernetes.io/managed-by": "omnia-operator",
				"omnia.altairalabs.ai/job":     arenaJob.Name,
			},
		},
		Data: map[string]string{
			overrides.ConfigMapKey: string(configJSON),
		},
	}

	// Set ArenaJob as owner - ConfigMap will be GC'd when job is deleted
	if err := ctrl.SetControllerReference(arenaJob, cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Try to create, update if already exists
	if err := r.Create(ctx, cm); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Update existing ConfigMap
			existing := &corev1.ConfigMap{}
			if getErr := r.Get(ctx, types.NamespacedName{
				Name:      cm.Name,
				Namespace: cm.Namespace,
			}, existing); getErr != nil {
				return fmt.Errorf("failed to get existing ConfigMap: %w", getErr)
			}
			existing.Data = cm.Data
			if updateErr := r.Update(ctx, existing); updateErr != nil {
				return fmt.Errorf("failed to update ConfigMap: %w", updateErr)
			}
			log.V(1).Info("updated override ConfigMap", "name", cm.Name)
		} else {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
	} else {
		log.Info("created override ConfigMap", "name", cm.Name)
	}

	return nil
}

// createWorkerJob creates a K8s Job for the Arena workers.
//
//nolint:gocognit,gocyclo // Pre-existing complexity, scheduled for refactoring
func (r *ArenaJobReconciler) createWorkerJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) error {
	log := logf.FromContext(ctx)

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
		workspaceName := r.getWorkspaceForNamespace(ctx, arenaJob.Namespace)
		if _, err := r.StorageManager.EnsureWorkspacePVC(ctx, workspaceName); err != nil {
			log.Error(err, "failed to ensure workspace PVC exists", "workspace", workspaceName)
			return fmt.Errorf("failed to ensure workspace PVC: %w", err)
		}
		log.V(1).Info("workspace PVC ensured", "workspace", workspaceName)
	}

	// Resolve provider overrides if specified (grouped by selector group name)
	providersByGroup, err := r.resolveProviderOverrides(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to resolve provider overrides: %w", err)
	}

	// Resolve tool registry override if specified
	toolOverrides, err := r.resolveToolRegistryOverride(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to resolve tool registry override: %w", err)
	}

	// Resolve binding registry (all namespace providers for annotation-based binding)
	bindingRegistry, bindingProviders, err := r.resolveBindingRegistry(ctx, arenaJob.Namespace)
	if err != nil {
		log.Error(err, "failed to resolve binding registry, continuing without bindings")
		// Non-fatal: bindings are best-effort
	}

	// Build and create override ConfigMap if there are any overrides or bindings
	overrideConfig := r.buildOverrideConfig(ctx, providersByGroup, toolOverrides)
	if len(bindingRegistry) > 0 {
		if overrideConfig == nil {
			overrideConfig = &overrides.OverrideConfig{}
		}
		overrideConfig.Bindings = bindingRegistry
	}
	hasOverrides := overrideConfig != nil
	if hasOverrides {
		if err := r.createOverrideConfigMap(ctx, arenaJob, overrideConfig); err != nil {
			return fmt.Errorf("failed to create override ConfigMap: %w", err)
		}
	}

	// Flatten providers for env var injection (secrets still passed as env vars)
	// Merge explicit override providers with binding providers for env var injection
	providerCRDs := providers.FlattenProviderGroups(providersByGroup)
	providerCRDs = deduplicateProviders(providerCRDs, bindingProviders)

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
		workspaceName := r.getWorkspaceForNamespace(ctx, arenaJob.Namespace)
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

	// Add provider credential environment variables from provider overrides
	// Secrets are still passed as env vars for security (not in ConfigMap)
	var providerEnvVars []corev1.EnvVar
	if len(providerCRDs) > 0 {
		log.Info("using provider overrides for credentials", "count", len(providerCRDs))
		for _, p := range providerCRDs {
			log.V(1).Info("provider",
				"name", p.Name,
				"type", p.Spec.Type,
				"model", p.Spec.Model,
			)
		}
		providerEnvVars = r.buildProviderEnvVarsFromCRDs(providerCRDs)
	}
	env = append(env, providerEnvVars...)

	// Add platform environment variables for hyperscaler providers
	platformEnvVars := providers.BuildPlatformEnvVars(providerCRDs)
	env = append(env, platformEnvVars...)

	// Add fleet execution mode env vars if configured
	if isFleetMode(arenaJob) {
		wsURL, err := r.resolveFleetTarget(ctx, arenaJob)
		if err != nil {
			return fmt.Errorf("failed to resolve fleet target: %w", err)
		}
		env = append(env,
			corev1.EnvVar{Name: "ARENA_EXECUTION_MODE", Value: "fleet"},
			corev1.EnvVar{Name: "ARENA_FLEET_WS_URL", Value: wsURL},
		)
		log.Info("fleet mode enabled", "wsURL", wsURL)
	}

	// Add overrides path env var if ConfigMap was created
	if hasOverrides {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_OVERRIDES_PATH",
			Value: "/etc/arena/overrides.json",
		})
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

	// Add override ConfigMap volume if there are overrides
	if hasOverrides {
		volumes = append(volumes, corev1.Volume{
			Name: "arena-overrides",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: getOverrideConfigMapName(arenaJob.Name),
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "arena-overrides",
			MountPath: "/etc/arena",
			ReadOnly:  true,
		})
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
						RunAsNonRoot: ptr(true),
						RunAsUser:    ptr(int64(65532)), // nonroot user
						FSGroup:      ptr(int64(65532)),
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
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr(false),
								ReadOnlyRootFilesystem:   ptr(true),
								RunAsNonRoot:             ptr(true),
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

	// Set ServiceAccountName for workload identity
	if saName := r.getWorkerServiceAccountName(providerCRDs); saName != "" {
		job.Spec.Template.Spec.ServiceAccountName = saName
		log.Info("setting worker ServiceAccountName for workload identity", "serviceAccount", saName)
	}

	// Set TTL if specified
	if arenaJob.Spec.TTLSecondsAfterFinished != nil {
		job.Spec.TTLSecondsAfterFinished = arenaJob.Spec.TTLSecondsAfterFinished
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
	workItemCount, enqueueErr := r.enqueueWorkItems(ctx, arenaJob, source, providerCRDs)
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

	workspaceName := r.getWorkspaceForNamespace(ctx, arenaJob.Namespace)

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

// buildFleetWorkItems creates one work item per scenario (no provider dimension).
// Used in fleet mode where the agent handles its own provider configuration.
func buildFleetWorkItems(jobName, bundleURL string, scenarios []partitioner.Scenario) []queue.WorkItem {
	now := time.Now()
	if len(scenarios) == 0 {
		// Single default item when scenarios can't be enumerated
		return []queue.WorkItem{{
			ID:          fmt.Sprintf("%s-fleet-0", jobName),
			JobID:       jobName,
			ScenarioID:  "default",
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		}}
	}

	items := make([]queue.WorkItem, 0, len(scenarios))
	for i, s := range scenarios {
		items = append(items, queue.WorkItem{
			ID:          fmt.Sprintf("%s-%s-%d", jobName, s.ID, i),
			JobID:       jobName,
			ScenarioID:  s.ID,
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
func (r *ArenaJobReconciler) enqueueWorkItems(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource, providerCRDs []*corev1alpha1.Provider) (int, error) {
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

	// Build work items based on execution mode
	var items []queue.WorkItem

	if isFleetMode(arenaJob) {
		// Fleet mode: one work item per scenario (no provider dimension)
		items = buildFleetWorkItems(arenaJob.Name, bundleURL, scenarios)
	} else {
		// Direct mode: scenario × provider matrix
		var providerIDs []string
		if len(providerCRDs) > 0 {
			providerIDs = r.getProviderIDsFromCRDs(providerCRDs)
			log.V(1).Info("using providers for work items", "count", len(providerIDs))
		}

		if len(scenarios) > 0 && len(providerIDs) > 0 {
			items = r.buildMatrixWorkItems(ctx, arenaJob.Name, bundleURL, scenarios, providerCRDs)
		}
		if len(items) == 0 {
			items = buildFallbackWorkItems(arenaJob.Name, bundleURL, providerIDs)
		}
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
				var passedItems, failedItems int
				if r.Aggregator != nil {
					log.V(1).Info("aggregating results from queue", "jobID", arenaJob.Name)
					result, err := r.Aggregator.Aggregate(ctx, arenaJob.Name)
					if err != nil {
						log.Error(err, "failed to aggregate results")
					} else {
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
					r.setCondition(arenaJob, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
						"TestsFailed", "Job completed but some tests failed")
					r.setCondition(arenaJob, ArenaJobConditionTypeReady, metav1.ConditionFalse,
						"Failed", "Job completed but some tests failed")
					if r.Recorder != nil {
						r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
							"Job completed but some tests failed")
					}
					log.Info("job completed with test failures",
						"passed", passedItems,
						"failed", failedItems)
				} else {
					arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseSucceeded
					r.setCondition(arenaJob, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
						"JobSucceeded", "Job completed successfully")
					r.setCondition(arenaJob, ArenaJobConditionTypeReady, metav1.ConditionTrue,
						"Succeeded", "Job completed successfully")
					if r.Recorder != nil {
						r.Recorder.Event(arenaJob, corev1.EventTypeNormal, ArenaJobEventReasonJobSucceeded,
							"Job completed successfully")
					}
					log.Info("job completed successfully")
				}
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
				now := metav1.Now()
				arenaJob.Status.CompletionTime = &now
				r.setCondition(arenaJob, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
					"JobFailed", condition.Message)
				r.setCondition(arenaJob, ArenaJobConditionTypeReady, metav1.ConditionFalse,
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
		r.setCondition(arenaJob, ArenaJobConditionTypeProgressing, metav1.ConditionTrue,
			"JobRunning", fmt.Sprintf("Job running: %d/%d completed", job.Status.Succeeded, completions))
	}
}

// handleValidationError handles errors during validation.
func (r *ArenaJobReconciler) handleValidationError(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, conditionType string, err error) {
	log := logf.FromContext(ctx)

	arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
	r.setCondition(arenaJob, conditionType, metav1.ConditionFalse, "ValidationFailed", err.Error())
	r.setCondition(arenaJob, ArenaJobConditionTypeReady, metav1.ConditionFalse,
		"ValidationFailed", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonConfigNotReady, err.Error())
	}

	if statusErr := r.Status().Update(ctx, arenaJob); statusErr != nil {
		log.Error(statusErr, "failed to update status after validation error")
	}
}

// setCondition sets a condition on the ArenaJob status.
func (r *ArenaJobReconciler) setCondition(arenaJob *omniav1alpha1.ArenaJob, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&arenaJob.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: arenaJob.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// findArenaJobsForSource maps ArenaSource changes to ArenaJob reconcile requests.
func (r *ArenaJobReconciler) findArenaJobsForSource(ctx context.Context, obj client.Object) []ctrl.Request {
	source, ok := obj.(*omniav1alpha1.ArenaSource)
	if !ok {
		return nil
	}

	// Find all ArenaJobs in the same namespace that reference this source
	jobList := &omniav1alpha1.ArenaJobList{}
	if err := r.List(ctx, jobList, client.InNamespace(source.Namespace)); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, job := range jobList.Items {
		if job.Spec.SourceRef.Name == source.Name {
			// Only trigger reconcile for pending jobs
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
		For(&omniav1alpha1.ArenaJob{}).
		Owns(&batchv1.Job{}).
		Watches(
			&omniav1alpha1.ArenaSource{},
			handler.EnqueueRequestsFromMapFunc(r.findArenaJobsForSource),
		).
		Named("arenajob").
		Complete(r)
}
