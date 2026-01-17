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
	"fmt"

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
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenajobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenaconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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

// createWorkerJob creates a K8s Job for the Arena workers.
func (r *ArenaJobReconciler) createWorkerJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, config *omniav1alpha1.ArenaConfig) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	if arenaJob.Spec.Workers != nil && arenaJob.Spec.Workers.Replicas > 0 {
		replicas = arenaJob.Spec.Workers.Replicas
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

	// Add source artifact URL if available
	if config.Status.ResolvedSource != nil {
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_ARTIFACT_URL",
			Value: config.Status.ResolvedSource.URL,
		})
		env = append(env, corev1.EnvVar{
			Name:  "ARENA_ARTIFACT_REVISION",
			Value: config.Status.ResolvedSource.Revision,
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
					Containers: []corev1.Container{
						{
							Name:            "worker",
							Image:           r.getWorkerImage(),
							ImagePullPolicy: r.getWorkerImagePullPolicy(),
							Env:             env,
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
				arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseSucceeded
				now := metav1.Now()
				arenaJob.Status.CompletionTime = &now
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
