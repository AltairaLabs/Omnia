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

	appsv1 "k8s.io/api/apps/v1"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	// AgentContainerName is the name of the agent container in the pod.
	AgentContainerName = "agent"
	// DefaultAgentImage is the default image for the agent container.
	DefaultAgentImage = "ghcr.io/altairalabs/omnia-agent:latest"
	// DefaultFacadePort is the default port for the WebSocket facade.
	DefaultFacadePort = 8080
	// FinalizerName is the finalizer for AgentRuntime resources.
	FinalizerName = "agentruntime.omnia.altairalabs.ai/finalizer"
)

// Condition types for AgentRuntime
const (
	ConditionTypeReady             = "Ready"
	ConditionTypeDeploymentReady   = "DeploymentReady"
	ConditionTypeServiceReady      = "ServiceReady"
	ConditionTypePromptPackReady   = "PromptPackReady"
	ConditionTypeToolRegistryReady = "ToolRegistryReady"
)

// AgentRuntimeReconciler reconciles a AgentRuntime object
type AgentRuntimeReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AgentImage string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

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
		// Requeue immediately to continue reconciliation
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}

	// Initialize status if needed
	if agentRuntime.Status.Phase == "" {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if err := r.Status().Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch referenced PromptPack
	promptPack, err := r.fetchPromptPack(ctx, agentRuntime)
	if err != nil {
		r.setCondition(agentRuntime, ConditionTypePromptPackReady, metav1.ConditionFalse,
			"PromptPackNotFound", err.Error())
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(agentRuntime, ConditionTypePromptPackReady, metav1.ConditionTrue,
		"PromptPackFound", "PromptPack resource found")

	// Fetch referenced ToolRegistry (optional)
	var toolRegistry *omniav1alpha1.ToolRegistry
	if agentRuntime.Spec.ToolRegistryRef != nil {
		toolRegistry, err = r.fetchToolRegistry(ctx, agentRuntime)
		if err != nil {
			r.setCondition(agentRuntime, ConditionTypeToolRegistryReady, metav1.ConditionFalse,
				"ToolRegistryNotFound", err.Error())
			// ToolRegistry is optional, so we continue with a warning
			log.Info("ToolRegistry not found, continuing without tools", "error", err)
		} else {
			r.setCondition(agentRuntime, ConditionTypeToolRegistryReady, metav1.ConditionTrue,
				"ToolRegistryFound", "ToolRegistry resource found")
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agentRuntime, promptPack, toolRegistry)
	if err != nil {
		r.setCondition(agentRuntime, ConditionTypeDeploymentReady, metav1.ConditionFalse,
			"DeploymentFailed", err.Error())
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(agentRuntime, ConditionTypeDeploymentReady, metav1.ConditionTrue,
		"DeploymentCreated", "Deployment created/updated successfully")

	// Reconcile Service
	if err := r.reconcileService(ctx, agentRuntime); err != nil {
		r.setCondition(agentRuntime, ConditionTypeServiceReady, metav1.ConditionFalse,
			"ServiceFailed", err.Error())
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(agentRuntime, ConditionTypeServiceReady, metav1.ConditionTrue,
		"ServiceCreated", "Service created/updated successfully")

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

func (r *AgentRuntimeReconciler) reconcileDeployment(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) (*appsv1.Deployment, error) {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, deployment, r.Scheme); err != nil {
			return err
		}

		// Build deployment spec
		r.buildDeploymentSpec(deployment, agentRuntime, promptPack, toolRegistry)
		return nil
	})

	if err != nil {
		return nil, err
	}

	log.Info("Deployment reconciled", "result", result)
	return deployment, nil
}

func (r *AgentRuntimeReconciler) buildDeploymentSpec(
	deployment *appsv1.Deployment,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) {
	labels := map[string]string{
		"app.kubernetes.io/name":         "omnia-agent",
		"app.kubernetes.io/instance":     agentRuntime.Name,
		"app.kubernetes.io/managed-by":   "omnia-operator",
		"omnia.altairalabs.ai/component": "agent",
	}

	replicas := int32(1)
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Replicas != nil {
		replicas = *agentRuntime.Spec.Runtime.Replicas
	}

	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

	image := r.AgentImage
	if image == "" {
		image = DefaultAgentImage
	}

	// Build container
	container := corev1.Container{
		Name:            AgentContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "facade",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: r.buildEnvVars(agentRuntime, promptPack, toolRegistry),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(8081), // Health server port
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(8081), // Health server port
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
	}

	// Add resources if specified
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Resources != nil {
		container.Resources = *agentRuntime.Spec.Runtime.Resources
	}

	// Build volume mounts
	volumeMounts, volumes := r.buildVolumes(agentRuntime, promptPack)
	container.VolumeMounts = volumeMounts

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		Volumes:    volumes,
	}

	// Add scheduling constraints if specified
	if agentRuntime.Spec.Runtime != nil {
		if agentRuntime.Spec.Runtime.NodeSelector != nil {
			podSpec.NodeSelector = agentRuntime.Spec.Runtime.NodeSelector
		}
		if agentRuntime.Spec.Runtime.Tolerations != nil {
			podSpec.Tolerations = agentRuntime.Spec.Runtime.Tolerations
		}
		if agentRuntime.Spec.Runtime.Affinity != nil {
			podSpec.Affinity = agentRuntime.Spec.Runtime.Affinity
		}
	}

	deployment.Labels = labels
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: podSpec,
		},
	}
}

func (r *AgentRuntimeReconciler) buildEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "OMNIA_AGENT_NAME",
			Value: agentRuntime.Name,
		},
		{
			Name:  "OMNIA_NAMESPACE",
			Value: agentRuntime.Namespace,
		},
		{
			Name:  "OMNIA_PROMPTPACK_NAME",
			Value: promptPack.Name,
		},
		{
			Name:  "OMNIA_PROMPTPACK_VERSION",
			Value: promptPack.Spec.Version,
		},
		{
			Name:  "OMNIA_FACADE_TYPE",
			Value: string(agentRuntime.Spec.Facade.Type),
		},
	}

	// Add facade port
	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_FACADE_PORT",
		Value: fmt.Sprintf("%d", port),
	})

	// Add handler mode (defaults to "runtime")
	handlerMode := omniav1alpha1.HandlerModeRuntime
	if agentRuntime.Spec.Facade.Handler != nil {
		handlerMode = *agentRuntime.Spec.Facade.Handler
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_HANDLER_MODE",
		Value: string(handlerMode),
	})

	// Add provider API key from secret
	envVars = append(envVars, corev1.EnvVar{
		Name: "OMNIA_PROVIDER_API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: agentRuntime.Spec.ProviderSecretRef,
				Key:                  "api-key",
			},
		},
	})

	// Add tool registry info if present
	if toolRegistry != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLREGISTRY_NAME",
			Value: toolRegistry.Name,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLREGISTRY_NAMESPACE",
			Value: toolRegistry.Namespace,
		})
	}

	// Add session config
	if agentRuntime.Spec.Session != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_SESSION_TYPE",
			Value: string(agentRuntime.Spec.Session.Type),
		})
		if agentRuntime.Spec.Session.TTL != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_SESSION_TTL",
				Value: *agentRuntime.Spec.Session.TTL,
			})
		}
		if agentRuntime.Spec.Session.StoreRef != nil {
			// Add session store connection string from secret
			envVars = append(envVars, corev1.EnvVar{
				Name: "OMNIA_SESSION_STORE_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: *agentRuntime.Spec.Session.StoreRef,
						Key:                  "url",
					},
				},
			})
		}
	}

	return envVars
}

func (r *AgentRuntimeReconciler) buildVolumes(
	_ *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
) ([]corev1.VolumeMount, []corev1.Volume) {
	var volumeMounts []corev1.VolumeMount
	var volumes []corev1.Volume

	// Mount PromptPack ConfigMap if source type is configmap
	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "promptpack-config",
			MountPath: "/etc/omnia/prompts",
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "promptpack-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *promptPack.Spec.Source.ConfigMapRef,
				},
			},
		})
	}

	return volumeMounts, volumes
}

func (r *AgentRuntimeReconciler) reconcileService(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, service, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			"app.kubernetes.io/name":         "omnia-agent",
			"app.kubernetes.io/instance":     agentRuntime.Name,
			"app.kubernetes.io/managed-by":   "omnia-operator",
			"omnia.altairalabs.ai/component": "agent",
		}

		port := int32(DefaultFacadePort)
		if agentRuntime.Spec.Facade.Port != nil {
			port = *agentRuntime.Spec.Facade.Port
		}

		service.Labels = labels
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

	log.Info("Service reconciled", "result", result)
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
		Named("agentruntime").
		Complete(r)
}
