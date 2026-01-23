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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/pkg/arena/providers"
	"github.com/altairalabs/omnia/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/license"
	"github.com/altairalabs/omnia/pkg/selector"
)

// ArenaJob condition types
const (
	ArenaJobConditionTypeReady       = "Ready"
	ArenaJobConditionTypeConfigValid = "ConfigValid"
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
	if workspace, ok := ns.Labels[labelWorkspace]; ok && workspace != "" {
		return workspace
	}
	// Fallback to namespace name
	return namespace
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenaconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
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
	}

	// Validate the referenced ArenaConfig
	config, err := r.validateConfig(ctx, arenaJob)
	if err != nil {
		log.Error(err, "failed to validate ArenaConfig")
		r.handleValidationError(ctx, arenaJob, ArenaJobConditionTypeConfigValid, err)
		return ctrl.Result{}, nil
	}
	r.setCondition(arenaJob, ArenaJobConditionTypeConfigValid, metav1.ConditionTrue,
		"ConfigValid", fmt.Sprintf("ArenaConfig %s is valid and ready", arenaJob.Spec.ConfigRef.Name))

	// Check if we already have a K8s Job
	existingJob, err := r.getExistingJob(ctx, arenaJob)
	if err != nil {
		log.Error(err, "failed to check for existing job")
		return ctrl.Result{}, err
	}

	if existingJob == nil {
		// Create the K8s Job
		if err := r.createWorkerJob(ctx, arenaJob, config); err != nil {
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

// validateConfig fetches and validates the referenced ArenaConfig.
func (r *ArenaJobReconciler) validateConfig(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) (*omniav1alpha1.ArenaConfig, error) {
	config := &omniav1alpha1.ArenaConfig{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      arenaJob.Spec.ConfigRef.Name,
		Namespace: arenaJob.Namespace,
	}, config); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("arenaConfig %s not found", arenaJob.Spec.ConfigRef.Name)
		}
		return nil, fmt.Errorf("failed to get arenaConfig %s: %w", arenaJob.Spec.ConfigRef.Name, err)
	}

	// Check if config is ready
	if config.Status.Phase != omniav1alpha1.ArenaConfigPhaseReady {
		return nil, fmt.Errorf("arenaConfig %s is not ready (phase: %s)", arenaJob.Spec.ConfigRef.Name, config.Status.Phase)
	}

	return config, nil
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

// resolveProviderOverrides resolves provider CRDs based on ArenaJob's providerOverrides.
// Returns the resolved providers and their environment variables.
// If no overrides are specified, returns nil (use ArenaConfig providers).
func (r *ArenaJobReconciler) resolveProviderOverrides(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) ([]*omniav1alpha1.Provider, error) {
	if len(arenaJob.Spec.ProviderOverrides) == 0 {
		return nil, nil
	}

	log := logf.FromContext(ctx)
	log.V(1).Info("resolving provider overrides", "overrides", len(arenaJob.Spec.ProviderOverrides))

	// Resolve all provider overrides
	resolvedByGroup, err := selector.ResolveProviderOverrides(ctx, r.Client, arenaJob.Namespace, arenaJob.Spec.ProviderOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve provider overrides: %w", err)
	}

	// Deduplicate providers across groups (same provider may match multiple selectors)
	seen := make(map[string]bool)
	var allProviders []*omniav1alpha1.Provider

	for group, groupProviders := range resolvedByGroup {
		log.V(1).Info("resolved providers for group", "group", group, "count", len(groupProviders))
		for _, p := range groupProviders {
			key := fmt.Sprintf("%s/%s", p.Namespace, p.Name)
			if !seen[key] {
				seen[key] = true
				allProviders = append(allProviders, p)
			}
		}
	}

	log.Info("resolved provider overrides", "totalProviders", len(allProviders))
	return allProviders, nil
}

// buildProviderEnvVarsFromCRDs builds environment variables for Provider CRDs.
// This extracts credentials from each provider's secretRef.
func (r *ArenaJobReconciler) buildProviderEnvVarsFromCRDs(providerCRDs []*omniav1alpha1.Provider) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	seen := make(map[string]bool)

	for _, provider := range providerCRDs {
		// Get the provider type for determining which env var to use
		providerType := string(provider.Spec.Type)

		// If provider has a secretRef, use it directly
		if provider.Spec.SecretRef != nil {
			// Get the expected env var name for this provider type
			secretRefs := providers.GetSecretRefsForProvider(providerType)
			for _, ref := range secretRefs {
				if seen[ref.EnvVar] {
					continue
				}
				seen[ref.EnvVar] = true

				// Use the provider's secretRef instead of the default
				envVars = append(envVars, corev1.EnvVar{
					Name: ref.EnvVar,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: provider.Spec.SecretRef.Name,
							},
							Key:      ref.Key,
							Optional: ptr(true),
						},
					},
				})
			}
		} else {
			// Fall back to default secret naming convention
			secretRefs := providers.GetSecretRefsForProvider(providerType)
			for _, ref := range secretRefs {
				if seen[ref.EnvVar] {
					continue
				}
				seen[ref.EnvVar] = true

				envVars = append(envVars, corev1.EnvVar{
					Name: ref.EnvVar,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: ref.SecretName,
							},
							Key:      ref.Key,
							Optional: ptr(true),
						},
					},
				})
			}
		}
	}

	return envVars
}

// getProviderIDsFromCRDs extracts provider IDs from Provider CRDs for work queue.
func (r *ArenaJobReconciler) getProviderIDsFromCRDs(providerCRDs []*omniav1alpha1.Provider) []string {
	ids := make([]string, len(providerCRDs))
	for i, p := range providerCRDs {
		ids[i] = p.Name
	}
	return ids
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

// createWorkerJob creates a K8s Job for the Arena workers.
func (r *ArenaJobReconciler) createWorkerJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, config *omniav1alpha1.ArenaConfig) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	if arenaJob.Spec.Workers != nil && arenaJob.Spec.Workers.Replicas > 0 {
		replicas = arenaJob.Spec.Workers.Replicas
	}

	// Resolve provider overrides if specified
	providerCRDs, err := r.resolveProviderOverrides(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to resolve provider overrides: %w", err)
	}

	// Resolve tool registry override if specified
	toolOverrides, err := r.resolveToolRegistryOverride(ctx, arenaJob)
	if err != nil {
		return fmt.Errorf("failed to resolve tool registry override: %w", err)
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
			Name:  "ARENA_CONFIG_NAME",
			Value: config.Name,
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
	if r.RedisPassword != "" {
		env = append(env, corev1.EnvVar{
			Name:  "REDIS_PASSWORD",
			Value: r.RedisPassword,
		})
	}

	// Add verbose flag for debug logging
	if arenaJob.Spec.Verbose {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_VERBOSE",
			Value: "true",
		})
	}

	// Add source artifact info if available
	if config.Status.ResolvedSource != nil {
		// Legacy: tar.gz URL for backwards compatibility
		if config.Status.ResolvedSource.URL != "" {
			env = append(env, corev1.EnvVar{
				Name:  "ARENA_ARTIFACT_URL",
				Value: config.Status.ResolvedSource.URL,
			})
		}
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_ARTIFACT_REVISION",
			Value: config.Status.ResolvedSource.Revision,
		})
		// New: filesystem-based content access
		if config.Status.ResolvedSource.ContentPath != "" {
			// Full path: {mount}/{workspace}/{namespace}/{contentPath}
			workspaceName := r.getWorkspaceForNamespace(ctx, arenaJob.Namespace)
			fullContentPath := fmt.Sprintf("/workspace-content/%s/%s/%s",
				workspaceName, arenaJob.Namespace, config.Status.ResolvedSource.ContentPath)
			env = append(env, corev1.EnvVar{
				Name:  "ARENA_CONTENT_PATH",
				Value: fullContentPath,
			})
		}
		if config.Status.ResolvedSource.Version != "" {
			env = append(env, corev1.EnvVar{
				Name:  "ARENA_CONTENT_VERSION",
				Value: config.Status.ResolvedSource.Version,
			})
		}
	}

	// Add provider credential environment variables
	// Use resolved CRDs if provider overrides are specified, otherwise use ArenaConfig providers
	var providerEnvVars []corev1.EnvVar
	if len(providerCRDs) > 0 {
		log.Info("using provider overrides for credentials", "count", len(providerCRDs))
		for _, p := range providerCRDs {
			log.Info("provider override",
				"name", p.Name,
				"type", p.Spec.Type,
				"model", p.Spec.Model,
			)
		}
		providerEnvVars = r.buildProviderEnvVarsFromCRDs(providerCRDs)
	} else {
		providerEnvVars = r.buildProviderEnvVars(config.Status.ResolvedProviders)
	}
	env = append(env, providerEnvVars...)

	// Add tool registry overrides as JSON if specified
	if len(toolOverrides) > 0 {
		toolOverridesJSON, err := json.Marshal(toolOverrides)
		if err != nil {
			return fmt.Errorf("failed to marshal tool overrides: %w", err)
		}
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_TOOL_OVERRIDES",
			Value: string(toolOverridesJSON),
		})
		log.Info("tool overrides configured", "count", len(toolOverrides))
	}

	// Determine if we should mount workspace content PVC
	useWorkspaceContent := r.WorkspaceContentPath != "" &&
		config.Status.ResolvedSource != nil &&
		config.Status.ResolvedSource.ContentPath != ""

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
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "workspace-content",
			MountPath: "/workspace-content",
			ReadOnly:  true,
		})
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
	if err := r.enqueueWorkItems(ctx, arenaJob, config, providerCRDs); err != nil {
		log.Error(err, "failed to enqueue work items")
		// Don't return error - job is created, workers will wait for items
	}

	return nil
}

// buildProviderEnvVars builds environment variables for provider credentials.
// Each provider type maps to specific API key environment variables, which are
// injected from corresponding Kubernetes secrets in the job's namespace.
// Secrets are referenced with optional: true to allow jobs to run even if
// some provider credentials are missing.
func (r *ArenaJobReconciler) buildProviderEnvVars(resolvedProviders []string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	seen := make(map[string]bool)

	for _, providerID := range resolvedProviders {
		// Extract provider type from ID (e.g., "gpt-4-turbo" might be type "openai")
		// For now, we use a simple heuristic - in a full implementation,
		// this would look up the provider type from the arena config
		providerType := inferProviderType(providerID)

		// Get secret references for this provider type
		secretRefs := providers.GetSecretRefsForProvider(providerType)

		for _, ref := range secretRefs {
			// Skip if we've already added this env var (multiple providers may share types)
			if seen[ref.EnvVar] {
				continue
			}
			seen[ref.EnvVar] = true

			envVars = append(envVars, corev1.EnvVar{
				Name: ref.EnvVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: ref.SecretName,
						},
						Key:      ref.Key,
						Optional: ptr(true), // Don't fail if secret doesn't exist
					},
				},
			})
		}
	}

	return envVars
}

// inferProviderType attempts to determine the provider type from a provider ID.
// This uses common naming conventions; in production, this should look up the
// actual provider configuration from the arena config file.
func inferProviderType(providerID string) string {
	// Common provider ID patterns
	id := providerID
	switch {
	case contains(id, "claude", "anthropic"):
		return "claude"
	case contains(id, "gpt", "openai", "o1", "o3"):
		return "openai"
	case contains(id, "gemini", "google"):
		return "gemini"
	case contains(id, "vllm"):
		return "vllm"
	case contains(id, "voyage"):
		return "voyageai"
	case contains(id, "azure"):
		return "azure"
	case contains(id, "bedrock", "aws"):
		return "bedrock"
	case contains(id, "groq"):
		return "groq"
	case contains(id, "together"):
		return "together"
	case contains(id, "ollama"):
		return "ollama"
	case contains(id, "mock"):
		return "mock"
	default:
		// Return the ID itself as the type if no pattern matches
		return providerID
	}
}

// contains checks if any of the substrings are contained in s (case-insensitive).
func contains(s string, substrs ...string) bool {
	sLower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(sLower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
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

// enqueueWorkItems creates and enqueues work items for the Arena job.
// Work items are scenario × provider combinations.
// If providerCRDs is non-nil, uses those providers; otherwise uses config.Status.ResolvedProviders.
func (r *ArenaJobReconciler) enqueueWorkItems(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, config *omniav1alpha1.ArenaConfig, providerCRDs []*omniav1alpha1.Provider) error {
	log := logf.FromContext(ctx)

	// Get queue (lazily connect if needed)
	q, err := r.getOrCreateQueue()
	if err != nil {
		return err
	}
	if q == nil {
		log.Info("no work queue configured, skipping work item enqueueing")
		return nil
	}

	// Get bundle URL from resolved source
	bundleURL := ""
	if config.Status.ResolvedSource != nil {
		bundleURL = config.Status.ResolvedSource.URL
	}

	// Get providers - use CRDs if overrides were resolved, otherwise use ArenaConfig providers
	var providerIDs []string
	if len(providerCRDs) > 0 {
		providerIDs = r.getProviderIDsFromCRDs(providerCRDs)
		log.V(1).Info("using provider overrides for work items", "count", len(providerIDs))
	} else {
		providerIDs = config.Status.ResolvedProviders
	}

	if len(providerIDs) == 0 {
		log.Info("no providers configured, skipping work item enqueueing")
		return nil
	}

	// For now, create one work item per provider with a "default" scenario
	// In the future, this should enumerate scenarios from the bundle
	items := make([]queue.WorkItem, 0, len(providerIDs))
	now := time.Now()
	for i, provider := range providerIDs {
		items = append(items, queue.WorkItem{
			ID:          fmt.Sprintf("%s-%s-%d", arenaJob.Name, provider, i),
			JobID:       arenaJob.Name,
			ScenarioID:  "default", // Worker will run all scenarios in the bundle
			ProviderID:  provider,
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		})
	}

	log.Info("enqueueing work items", "count", len(items), "providers", providerIDs)
	if err := q.Push(ctx, arenaJob.Name, items); err != nil {
		return fmt.Errorf("failed to push work items to queue: %w", err)
	}

	return nil
}

// updateStatusFromJob updates the ArenaJob status based on the K8s Job status.
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

// findArenaJobsForConfig maps ArenaConfig changes to ArenaJob reconcile requests.
func (r *ArenaJobReconciler) findArenaJobsForConfig(ctx context.Context, obj client.Object) []ctrl.Request {
	config, ok := obj.(*omniav1alpha1.ArenaConfig)
	if !ok {
		return nil
	}

	// Find all ArenaJobs in the same namespace that reference this config
	jobList := &omniav1alpha1.ArenaJobList{}
	if err := r.List(ctx, jobList, client.InNamespace(config.Namespace)); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, job := range jobList.Items {
		if job.Spec.ConfigRef.Name == config.Name {
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
			&omniav1alpha1.ArenaConfig{},
			handler.EnqueueRequestsFromMapFunc(r.findArenaJobsForConfig),
		).
		Named("arenajob").
		Complete(r)
}
