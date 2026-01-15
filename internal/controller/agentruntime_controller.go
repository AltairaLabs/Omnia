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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// AgentRuntimeReconciler reconciles a AgentRuntime object
type AgentRuntimeReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	FacadeImage              string
	FacadeImagePullPolicy    corev1.PullPolicy
	FrameworkImage           string
	FrameworkImagePullPolicy corev1.PullPolicy
	// Tracing configuration for runtime containers
	TracingEnabled  bool
	TracingEndpoint string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete

// reconcileReferences fetches and validates all referenced resources.
// Returns promptPack (required), toolRegistry (optional), provider (optional), and any error.
func (r *AgentRuntimeReconciler) reconcileReferences(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) (*omniav1alpha1.PromptPack, *omniav1alpha1.ToolRegistry, *omniav1alpha1.Provider, ctrl.Result, error) {
	// Fetch required PromptPack
	promptPack, err := r.fetchPromptPack(ctx, agentRuntime)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypePromptPackReady, "PromptPackNotFound", err)
		return nil, nil, nil, ctrl.Result{}, err
	}
	r.setCondition(agentRuntime, ConditionTypePromptPackReady, metav1.ConditionTrue,
		"PromptPackFound", "PromptPack resource found")

	// Fetch optional ToolRegistry
	var toolRegistry *omniav1alpha1.ToolRegistry
	if agentRuntime.Spec.ToolRegistryRef != nil {
		toolRegistry, err = r.fetchToolRegistry(ctx, agentRuntime)
		if err != nil {
			r.setCondition(agentRuntime, ConditionTypeToolRegistryReady, metav1.ConditionFalse,
				"ToolRegistryNotFound", err.Error())
			log.Info("ToolRegistry not found, continuing without tools", "error", err)
		} else {
			r.setCondition(agentRuntime, ConditionTypeToolRegistryReady, metav1.ConditionTrue,
				"ToolRegistryFound", "ToolRegistry resource found")
		}
	}

	// Fetch optional Provider
	var provider *omniav1alpha1.Provider
	if agentRuntime.Spec.ProviderRef != nil {
		provider, result, err := r.reconcileProviderRef(ctx, log, agentRuntime)
		if err != nil || result.RequeueAfter > 0 {
			return nil, nil, nil, result, err
		}
		return promptPack, toolRegistry, provider, ctrl.Result{}, nil
	}

	return promptPack, toolRegistry, provider, ctrl.Result{}, nil
}

// reconcileProviderRef fetches and validates the Provider reference.
func (r *AgentRuntimeReconciler) reconcileProviderRef(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) (*omniav1alpha1.Provider, ctrl.Result, error) {
	provider, err := r.fetchProvider(ctx, agentRuntime)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeProviderReady, "ProviderNotFound", err)
		return nil, ctrl.Result{}, err
	}
	if provider.Status.Phase != omniav1alpha1.ProviderPhaseReady {
		r.setCondition(agentRuntime, ConditionTypeProviderReady, metav1.ConditionFalse,
			"ProviderNotReady", fmt.Sprintf("Provider %s is in %s phase", provider.Name, provider.Status.Phase))
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.setCondition(agentRuntime, ConditionTypeProviderReady, metav1.ConditionTrue,
		"ProviderFound", "Provider resource found and ready")
	return provider, ctrl.Result{}, nil
}

// handleRefError handles reference fetch errors by setting condition, updating status, and logging.
func (r *AgentRuntimeReconciler) handleRefError(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	condType string,
	reason string,
	err error,
) {
	r.setCondition(agentRuntime, condType, metav1.ConditionFalse, reason, err.Error())
	agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
	if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
}

// reconcileResources creates/updates Deployment and Service.
func (r *AgentRuntimeReconciler) reconcileResources(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
) (*appsv1.Deployment, error) {
	// Reconcile tools ConfigMap
	if toolRegistry != nil {
		if err := r.reconcileToolsConfigMap(ctx, agentRuntime, toolRegistry); err != nil {
			log.Error(err, "Failed to reconcile tools ConfigMap")
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agentRuntime, promptPack, toolRegistry, provider)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeDeploymentReady, "DeploymentFailed", err)
		return nil, err
	}
	r.setCondition(agentRuntime, ConditionTypeDeploymentReady, metav1.ConditionTrue,
		"DeploymentCreated", "Deployment created/updated successfully")

	// Reconcile Service
	if err := r.reconcileService(ctx, agentRuntime); err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeServiceReady, "ServiceFailed", err)
		return nil, err
	}
	r.setCondition(agentRuntime, ConditionTypeServiceReady, metav1.ConditionTrue,
		"ServiceCreated", "Service created/updated successfully")

	return deployment, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentRuntimeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentRuntime instance
	agentRuntime := &omniav1alpha1.AgentRuntime{}
	if err := r.Get(ctx, req.NamespacedName, agentRuntime); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AgentRuntime resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AgentRuntime")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !agentRuntime.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, agentRuntime)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(agentRuntime, FinalizerName) {
		controllerutil.AddFinalizer(agentRuntime, FinalizerName)
		if err := r.Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}

	// Initialize status if needed
	if agentRuntime.Status.Phase == "" {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if err := r.Status().Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch all references
	promptPack, toolRegistry, provider, result, err := r.reconcileReferences(ctx, log, agentRuntime)
	if err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Reconcile resources
	deployment, err := r.reconcileResources(ctx, log, agentRuntime, promptPack, toolRegistry, provider)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile autoscaling (HPA or KEDA if enabled)
	if err := r.reconcileAutoscaling(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile autoscaling")
		// Don't fail the reconciliation for autoscaling errors, just log
	}

	// Update status from deployment
	r.updateStatusFromDeployment(agentRuntime, deployment, promptPack)

	// Set overall Ready condition
	if agentRuntime.Status.Replicas != nil && agentRuntime.Status.Replicas.Ready > 0 {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseRunning
		r.setCondition(agentRuntime, ConditionTypeReady, metav1.ConditionTrue,
			"RuntimeReady", "AgentRuntime is ready")
	} else {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		r.setCondition(agentRuntime, ConditionTypeReady, metav1.ConditionFalse,
			"RuntimeNotReady", "Waiting for pods to be ready")
	}

	agentRuntime.Status.ObservedGeneration = agentRuntime.Generation
	if err := r.Status().Update(ctx, agentRuntime); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentRuntimeReconciler) reconcileDelete(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling deletion of AgentRuntime")

	// Owned resources (Deployment, Service) will be garbage collected automatically
	// due to OwnerReferences

	// Remove finalizer
	controllerutil.RemoveFinalizer(agentRuntime, FinalizerName)
	if err := r.Update(ctx, agentRuntime); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentRuntimeReconciler) fetchPromptPack(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (*omniav1alpha1.PromptPack, error) {
	promptPack := &omniav1alpha1.PromptPack{}
	key := types.NamespacedName{
		Name:      agentRuntime.Spec.PromptPackRef.Name,
		Namespace: agentRuntime.Namespace,
	}
	if err := r.Get(ctx, key, promptPack); err != nil {
		return nil, fmt.Errorf("failed to get PromptPack %s: %w", key, err)
	}
	return promptPack, nil
}

func (r *AgentRuntimeReconciler) fetchToolRegistry(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (*omniav1alpha1.ToolRegistry, error) {
	ref := agentRuntime.Spec.ToolRegistryRef
	toolRegistry := &omniav1alpha1.ToolRegistry{}

	namespace := agentRuntime.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}
	if err := r.Get(ctx, key, toolRegistry); err != nil {
		return nil, fmt.Errorf("failed to get ToolRegistry %s: %w", key, err)
	}
	return toolRegistry, nil
}

func (r *AgentRuntimeReconciler) fetchProvider(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (*omniav1alpha1.Provider, error) {
	ref := agentRuntime.Spec.ProviderRef
	provider := &omniav1alpha1.Provider{}

	namespace := agentRuntime.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}
	if err := r.Get(ctx, key, provider); err != nil {
		return nil, fmt.Errorf("failed to get Provider %s: %w", key, err)
	}
	return provider, nil
}

func (r *AgentRuntimeReconciler) reconcileService(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, service, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
			labelOmniaComp:    "agent",
		}

		// Prometheus scrape annotations on Service (not pod, as Istio overrides pod annotations)
		annotations := map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   fmt.Sprintf("%d", port),
			"prometheus.io/path":   "/metrics",
		}

		service.Labels = labels
		service.Annotations = annotations
		service.Spec = corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "facade",
					Port:       port,
					TargetPort: intstr.FromString("facade"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Set the service endpoint in status for dashboard/client connections
	agentRuntime.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		agentRuntime.Name, agentRuntime.Namespace, port)

	log.Info("Service reconciled", "result", result, "endpoint", agentRuntime.Status.ServiceEndpoint)
	return nil
}

func (r *AgentRuntimeReconciler) updateStatusFromDeployment(
	agentRuntime *omniav1alpha1.AgentRuntime,
	deployment *appsv1.Deployment,
	promptPack *omniav1alpha1.PromptPack,
) {
	agentRuntime.Status.Replicas = &omniav1alpha1.ReplicaStatus{
		Desired:   deployment.Status.Replicas,
		Ready:     deployment.Status.ReadyReplicas,
		Available: deployment.Status.AvailableReplicas,
	}

	version := promptPack.Spec.Version
	agentRuntime.Status.ActiveVersion = &version
}

func (r *AgentRuntimeReconciler) setCondition(
	agentRuntime *omniav1alpha1.AgentRuntime,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&agentRuntime.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: agentRuntime.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.AgentRuntime{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		// Watch Provider changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.Provider{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForProvider),
		).
		// Watch PromptPack changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.PromptPack{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForPromptPack),
		).
		Named("agentruntime").
		Complete(r)
}
