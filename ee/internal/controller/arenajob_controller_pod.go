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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/internal/podoverrides"
)

// workerServiceAccountName returns the ServiceAccount the worker pod runs as:
// the configured workspace runtime SA when set, otherwise the per-job
// arena-worker SA the controller creates.
func (r *ArenaJobReconciler) workerServiceAccountName() string {
	if r.WorkerServiceAccount != "" {
		return r.WorkerServiceAccount
	}
	return arenaWorkerRBACName
}

// buildRedisURLEnvVar returns the REDIS_URL env var for worker pods.
// When RedisURLSecretName is set, uses a secretKeyRef for secure
// injection (the URL itself contains the password). Otherwise emits
// the literal RedisURL as a plain env var (acceptable for
// unauthenticated dev Redis).
func (r *ArenaJobReconciler) buildRedisURLEnvVar() []corev1.EnvVar {
	if r.RedisURLSecretName != "" && r.RedisURLSecretKey != "" {
		return []corev1.EnvVar{{
			Name: "REDIS_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.RedisURLSecretName,
					},
					Key: r.RedisURLSecretKey,
				},
			},
		}}
	}
	if r.RedisURL != "" {
		return []corev1.EnvVar{{
			Name:  "REDIS_URL",
			Value: r.RedisURL,
		}}
	}
	return nil
}

// buildMgmtPlaneTokenEnvVar returns the OMNIA_MGMT_PLANE_SERVICE_TOKEN_URL env
// var for worker pods when the operator was configured with the dashboard's
// service-token endpoint. The worker uses it to mint a mgmt-plane JWT (via its
// own SA token) and authenticate fleet-mode WS dials to agent facades. Empty
// config → no env var → fleet dials proceed unauthenticated.
func (r *ArenaJobReconciler) buildMgmtPlaneTokenEnvVar() []corev1.EnvVar {
	if r.MgmtPlaneTokenURL == "" {
		return nil
	}
	return []corev1.EnvVar{{
		Name:  "OMNIA_MGMT_PLANE_SERVICE_TOKEN_URL",
		Value: r.MgmtPlaneTokenURL,
	}}
}

// getJobName returns the name for the K8s Job.
func (r *ArenaJobReconciler) getJobName(arenaJob *omniav1alpha1.ArenaJob) string {
	return fmt.Sprintf("%s-worker", arenaJob.Name)
}

// deleteWorkerJob deletes the worker Job for an ArenaJob (used on
// cancellation). Foreground propagation removes the worker pods too. A
// missing Job is not an error — it may not have been created yet, or already
// be gone.
func (r *ArenaJobReconciler) deleteWorkerJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) error {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getJobName(arenaJob),
			Namespace: arenaJob.Namespace,
		},
	}
	policy := metav1.DeletePropagationForeground
	if err := r.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &policy}); err != nil &&
		!apierrors.IsNotFound(err) {
		return err
	}
	return nil
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

// buildOutputConfig returns env vars, volumes, and volume mounts required to
// persist worker output to the configured destination.
//
// PVC output: mounts the named PVC at /arena-output (read-write) and sets
// ARENA_OUTPUT_DIR so the worker writes engine output there directly.
//
// S3 output: injects ARENA_OUTPUT_CONFIG (JSON) so the worker can upload files
// after execution. Credentials from spec.output.s3.secretRef are injected as
// ARENA_S3_ACCESS_KEY_ID and ARENA_S3_SECRET_ACCESS_KEY via secretKeyRef.
//
// If no output config is set, all three returned slices are empty.
func (r *ArenaJobReconciler) buildOutputConfig(arenaJob *omniav1alpha1.ArenaJob) (
	env []corev1.EnvVar,
	volumes []corev1.Volume,
	volumeMounts []corev1.VolumeMount,
) {
	if arenaJob.Spec.Output == nil {
		return nil, nil, nil
	}

	// Always inject the JSON-encoded OutputConfig so the worker knows the destination.
	raw, err := json.Marshal(arenaJob.Spec.Output)
	if err == nil {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_OUTPUT_CONFIG",
			Value: string(raw),
		})
	}

	switch arenaJob.Spec.Output.Type {
	case omniav1alpha1.OutputTypePVC:
		if arenaJob.Spec.Output.PVC == nil {
			return env, nil, nil
		}
		pvc := arenaJob.Spec.Output.PVC
		subPath := pvc.SubPath

		volumes = append(volumes, corev1.Volume{
			Name: "arena-output",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.ClaimName,
					ReadOnly:  false,
				},
			},
		})
		vm := corev1.VolumeMount{
			Name:      "arena-output",
			MountPath: outputPVCMountPath,
			ReadOnly:  false,
		}
		if subPath != "" {
			vm.SubPath = subPath
		}
		volumeMounts = append(volumeMounts, vm)
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_OUTPUT_DIR",
			Value: outputPVCMountPath,
		})

	case omniav1alpha1.OutputTypeS3:
		if arenaJob.Spec.Output.S3 == nil {
			return env, nil, nil
		}
		s3Cfg := arenaJob.Spec.Output.S3
		if s3Cfg.SecretRef != nil {
			env = append(env,
				corev1.EnvVar{
					Name: "ARENA_S3_ACCESS_KEY_ID",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3Cfg.SecretRef.Name,
							},
							Key:      "AWS_ACCESS_KEY_ID",
							Optional: ptr.To(true),
						},
					},
				},
				corev1.EnvVar{
					Name: "ARENA_S3_SECRET_ACCESS_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3Cfg.SecretRef.Name,
							},
							Key:      "AWS_SECRET_ACCESS_KEY",
							Optional: ptr.To(true),
						},
					},
				},
			)
		}
	}

	return env, volumes, volumeMounts
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
		arenaFile = defaultArenaConfigFile
	}

	// Resolve workspace name for session recording metadata.
	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)

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
			Name:  "ARENA_WORKSPACE_NAME",
			Value: workspaceName,
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

	// Add Redis URL config (literal value or secret-sourced env). The
	// arena-worker binary picks the URL up via REDIS_URL env fallback
	// on its --redis-url flag.
	env = append(env, r.buildRedisURLEnvVar()...)

	// Authenticate fleet-mode WS dials (mgmt-plane token URL → worker env).
	env = append(env, r.buildMgmtPlaneTokenEnvVar()...)

	// Add verbose flag for debug logging
	if arenaJob.Spec.Verbose {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_VERBOSE",
			Value: envValueTrue,
		}, corev1.EnvVar{
			Name:  "LOG_LEVEL",
			Value: "debug",
		})
	}

	// Add VU pool configuration from loadTest settings
	if arenaJob.Spec.LoadTest != nil {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_VUS_PER_WORKER",
			Value: fmt.Sprintf("%d", arenaJob.Spec.LoadTest.VUsPerWorker),
		}, corev1.EnvVar{
			Name:  "ARENA_CONCURRENCY",
			Value: fmt.Sprintf("%d", arenaJob.Spec.LoadTest.Concurrency),
		})
		if arenaJob.Spec.LoadTest.Ramp != nil {
			if arenaJob.Spec.LoadTest.Ramp.Up != "" {
				env = append(env, corev1.EnvVar{
					Name:  "ARENA_RAMP_UP",
					Value: arenaJob.Spec.LoadTest.Ramp.Up,
				})
			}
			if arenaJob.Spec.LoadTest.Ramp.Down != "" {
				env = append(env, corev1.EnvVar{
					Name:  "ARENA_RAMP_DOWN",
					Value: arenaJob.Spec.LoadTest.Ramp.Down,
				})
			}
		}
	}

	// Inject SESSION_API_URL only when session recording is explicitly enabled.
	// Default is off to avoid overwhelming session-api during load tests.
	if arenaJob.Spec.SessionRecording {
		sessionURL := r.resolveSessionURLForWorkspace(ctx, arenaJob.Namespace)
		if sessionURL != "" {
			env = append(env, corev1.EnvVar{
				Name:  "SESSION_API_URL",
				Value: sessionURL,
			})
		}
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

	// Calculate the volume subPath for content isolation.
	//
	// Two modes, selected by WorkspaceContentScoped:
	//   - Legacy (share-root volume): the volume root is the whole share, so
	//     the subPath carries the full {workspace}/{namespace}/{contentPath}
	//     prefix and isolation is a mount-time subPath convention.
	//   - Scoped (workspace-scoped volume): the volume root IS the
	//     {workspace}/{namespace} subtree (storage-enforced isolation, e.g.
	//     an Azure native NFS PV). The subPath is therefore workspace-relative
	//     ({contentPath}/{rootPath}); adding the {workspace}/{namespace} prefix
	//     would double-prefix (demo/omnia-demo/demo/omnia-demo/...).
	var contentSubPath string
	if source.Status.Artifact != nil && source.Status.Artifact.ContentPath != "" {
		if r.WorkspaceContentScoped {
			contentSubPath = source.Status.Artifact.ContentPath
		} else {
			workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)
			contentSubPath = fmt.Sprintf("%s/%s/%s",
				workspaceName, arenaJob.Namespace, source.Status.Artifact.ContentPath)
		}
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
			corev1.EnvVar{Name: "TRACING_ENABLED", Value: envValueTrue},
			corev1.EnvVar{Name: "TRACING_ENDPOINT", Value: r.TracingEndpoint},
			corev1.EnvVar{Name: "TRACING_INSECURE", Value: envValueTrue},
		)
	}

	// Build volumes list
	volumes := []corev1.Volume{
		{
			Name: volumeNameTmp,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Build volume mounts list
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      volumeNameTmp,
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
		// This provides content isolation between jobs.
		//
		// Subtree pre-creation: a native NFS mount to a non-existent export
		// path fails (NFS won't auto-create the directory the way kubelet
		// auto-creates a subPath). No explicit mkdir is needed here, though,
		// because the worker only runs after its ArenaSource has produced an
		// artifact (the reconcile bails with "has no artifact" otherwise — see
		// validateSource). Producing that artifact runs the FilesystemSyncer
		// (internal/sourcesync/syncer.go) under the operator's share-root
		// WorkspaceContentPath, whose storeVersion/UpdateHEAD os.MkdirAll the
		// full {workspace}/{namespace}/{contentPath}/.arena/... subtree on the
		// share. So in both modes the subtree already exists before any tenant
		// pod mounts it.
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "workspace-content",
			MountPath: "/workspace-content",
			SubPath:   contentSubPath,
			ReadOnly:  true,
		})
		log.Info("mounting content with isolation",
			"subPath", contentSubPath, "workspaceScoped", r.WorkspaceContentScoped)
	}

	// Wire output destination — PVC mount or S3 env vars.
	outputEnv, outputVolumes, outputVolumeMounts := r.buildOutputConfig(arenaJob)
	env = append(env, outputEnv...)
	volumes = append(volumes, outputVolumes...)
	volumeMounts = append(volumeMounts, outputVolumeMounts...)

	// Create the Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getJobName(arenaJob),
			Namespace: arenaJob.Namespace,
			Labels: map[string]string{
				labelAppName:                  arenaWorkerRBACName,
				"app.kubernetes.io/instance":  arenaJob.Name,
				"app.kubernetes.io/component": componentWorker,
				labelManagedBy:                managedByOperator,
				labelArenaJob:                 arenaJob.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism: &replicas,
			Completions: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelAppName:                     arenaWorkerRBACName,
						"app.kubernetes.io/instance":     arenaJob.Name,
						"app.kubernetes.io/component":    componentWorker,
						labelArenaJob:                    arenaJob.Name,
						"omnia.altairalabs.ai/component": arenaWorkerRBACName,
					},
					Annotations: map[string]string{
						"prometheus.io/scrape": envValueTrue,
						"prometheus.io/port":   "9090",
						"prometheus.io/path":   "/metrics",
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

	// Set ServiceAccountName for CRD reads (created by reconcileWorkerRBAC above).
	// This is the pod's identity SA: the configured workspace runtime SA when
	// set (so the worker inherits the workspace cloud identity), otherwise the
	// per-job arena-worker SA.
	job.Spec.Template.Spec.ServiceAccountName = workerSAName

	// Stamp configured worker pod labels (e.g. the cloud-identity webhook
	// opt-in label) onto the pod template. Done before PodOverrides so an
	// explicit per-job override still wins on conflict.
	for k, v := range r.WorkerPodLabels {
		job.Spec.Template.Labels[k] = v
	}

	// Apply user-supplied PodOverrides.
	applyWorkerPodOverrides(job, arenaJob)

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

// resolveSessionURLForWorkspace looks up the session-api URL from the Workspace CRD status
// for the workspace that owns the given namespace.
func (r *ArenaJobReconciler) resolveSessionURLForWorkspace(ctx context.Context, namespace string) string {
	var list corev1alpha1.WorkspaceList
	if err := r.List(ctx, &list); err != nil {
		return ""
	}
	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			for _, sg := range ws.Status.Services {
				if sg.Name == defaultName && sg.SessionURL != "" {
					return sg.SessionURL
				}
			}
		}
	}
	return ""
}

// applyWorkerPodOverrides merges ArenaJob.spec.workers.podOverrides onto the
// worker Job's pod template.
func applyWorkerPodOverrides(job *batchv1.Job, arenaJob *omniav1alpha1.ArenaJob) {
	if arenaJob.Spec.Workers == nil || arenaJob.Spec.Workers.PodOverrides == nil {
		return
	}
	overrides := arenaJob.Spec.Workers.PodOverrides
	podoverrides.ApplyPod(&job.Spec.Template.Spec, &job.Spec.Template.ObjectMeta, overrides)
	for i := range job.Spec.Template.Spec.Containers {
		podoverrides.ApplyContainer(&job.Spec.Template.Spec.Containers[i], overrides)
	}
}
