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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	// FacadeContainerName is the name of the facade container in the pod.
	FacadeContainerName = "facade"
	// RuntimeContainerName is the name of the runtime container in the pod.
	RuntimeContainerName = "runtime"
	// DefaultFacadeImage is the default image for the facade container.
	DefaultFacadeImage = "ghcr.io/altairalabs/omnia-facade:latest"
	// DefaultFrameworkImage is the default image for the framework container.
	DefaultFrameworkImage = "ghcr.io/altairalabs/omnia-runtime:latest"
	// DefaultFacadePort is the default port for the WebSocket facade.
	DefaultFacadePort = 8080
	// DefaultFacadeHealthPort is the health port for the facade container.
	DefaultFacadeHealthPort = 8081
	// DefaultRuntimeGRPCPort is the gRPC port for the runtime container.
	DefaultRuntimeGRPCPort = 9000
	// DefaultRuntimeHealthPort is the health port for the runtime container.
	DefaultRuntimeHealthPort = 9001
	// FinalizerName is the finalizer for AgentRuntime resources.
	FinalizerName = "agentruntime.omnia.altairalabs.ai/finalizer"
	// ToolsConfigMapSuffix is the suffix for the tools ConfigMap name.
	ToolsConfigMapSuffix = "-tools"
	// ToolsConfigFileName is the filename for tools configuration.
	ToolsConfigFileName = "tools.yaml"
	// ToolsMountPath is the mount path for tools configuration.
	ToolsMountPath = "/etc/omnia/tools"
	// PromptPackMountPath is the mount path for PromptPack files.
	PromptPackMountPath = "/etc/omnia/pack"
	// MockProviderAnnotation enables mock provider for testing.
	MockProviderAnnotation = "omnia.altairalabs.ai/mock-provider"
)

// Helper functions for creating pointers
func ptr[T any](v T) *T {
	return &v
}

func ptrSelectPolicy(p autoscalingv2.ScalingPolicySelect) *autoscalingv2.ScalingPolicySelect {
	return &p
}

// buildSessionEnvVars creates environment variables for session configuration.
// The urlEnvName parameter allows different env var names for different containers.
func buildSessionEnvVars(session *omniav1alpha1.SessionConfig, urlEnvName string) []corev1.EnvVar {
	if session == nil {
		return nil
	}

	envVars := []corev1.EnvVar{
		{
			Name:  "OMNIA_SESSION_TYPE",
			Value: string(session.Type),
		},
	}

	if session.TTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_SESSION_TTL",
			Value: *session.TTL,
		})
	}

	if session.StoreRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: urlEnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *session.StoreRef,
					Key:                  "url",
				},
			},
		})
	}

	return envVars
}

// buildProviderEnvVars creates environment variables for LLM provider configuration.
func buildProviderEnvVars(provider *omniav1alpha1.ProviderConfig) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Default to auto-detection if provider not specified
	providerType := omniav1alpha1.ProviderTypeAuto
	if provider != nil && provider.Type != "" {
		providerType = provider.Type
	}

	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_PROVIDER_TYPE",
		Value: string(providerType),
	})

	// Add model if specified
	if provider != nil && provider.Model != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PROVIDER_MODEL",
			Value: provider.Model,
		})
	}

	// Add base URL if specified
	if provider != nil && provider.BaseURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PROVIDER_BASE_URL",
			Value: provider.BaseURL,
		})
	}

	// Add provider config (temperature, topP, maxTokens)
	if provider != nil && provider.Config != nil {
		if provider.Config.Temperature != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_TEMPERATURE",
				Value: *provider.Config.Temperature,
			})
		}
		if provider.Config.TopP != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_TOP_P",
				Value: *provider.Config.TopP,
			})
		}
		if provider.Config.MaxTokens != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_MAX_TOKENS",
				Value: fmt.Sprintf("%d", *provider.Config.MaxTokens),
			})
		}
	}

	// Add pricing config
	if provider != nil && provider.Pricing != nil {
		if provider.Pricing.InputCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_INPUT_COST",
				Value: *provider.Pricing.InputCostPer1K,
			})
		}
		if provider.Pricing.OutputCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_OUTPUT_COST",
				Value: *provider.Pricing.OutputCostPer1K,
			})
		}
		if provider.Pricing.CachedCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_CACHED_COST",
				Value: *provider.Pricing.CachedCostPer1K,
			})
		}
	}

	// Add API key from secret
	// Determine which secret and key to use based on provider type
	if provider != nil && provider.SecretRef != nil {
		// User specified a secret - inject all keys from it as env vars
		// The secret should contain the appropriate key (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)
		envVars = append(envVars, buildSecretEnvVars(provider.SecretRef, providerType)...)
	}

	return envVars
}

// buildSecretEnvVars creates environment variables from a provider secret.
// It maps secret keys to the appropriate environment variable names expected by PromptKit.
func buildSecretEnvVars(secretRef *corev1.LocalObjectReference, providerType omniav1alpha1.ProviderType) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Map of provider types to their expected API key env var names
	providerKeyNames := map[omniav1alpha1.ProviderType][]string{
		omniav1alpha1.ProviderTypeClaude: {"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"},
		omniav1alpha1.ProviderTypeOpenAI: {"OPENAI_API_KEY", "OPENAI_TOKEN"},
		omniav1alpha1.ProviderTypeGemini: {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	}

	// For explicit provider type, try to inject the primary key
	if keyNames, ok := providerKeyNames[providerType]; ok && len(keyNames) > 0 {
		// Try to get the primary key name from the secret
		envVars = append(envVars, corev1.EnvVar{
			Name: keyNames[0],
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *secretRef,
					Key:                  keyNames[0],
					Optional:             boolPtr(true), // Optional in case user uses different key
				},
			},
		})
		// Also try "api-key" as a fallback
		envVars = append(envVars, corev1.EnvVar{
			Name: keyNames[0],
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *secretRef,
					Key:                  "api-key",
					Optional:             boolPtr(true),
				},
			},
		})
	}

	// For auto-detection, inject all possible API key env vars
	if providerType == omniav1alpha1.ProviderTypeAuto {
		for _, keyNames := range providerKeyNames {
			if len(keyNames) > 0 {
				envVars = append(envVars, corev1.EnvVar{
					Name: keyNames[0],
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: *secretRef,
							Key:                  keyNames[0],
							Optional:             boolPtr(true),
						},
					},
				})
			}
		}
	}

	return envVars
}

func boolPtr(b bool) *bool {
	return &b
}

// buildProviderEnvVarsFromCRD creates environment variables from a Provider CRD.
// This is used when an AgentRuntime references a Provider resource.
func buildProviderEnvVarsFromCRD(provider *omniav1alpha1.Provider) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Provider type
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_PROVIDER_TYPE",
		Value: string(provider.Spec.Type),
	})

	// Model
	if provider.Spec.Model != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PROVIDER_MODEL",
			Value: provider.Spec.Model,
		})
	}

	// Base URL
	if provider.Spec.BaseURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PROVIDER_BASE_URL",
			Value: provider.Spec.BaseURL,
		})
	}

	// Provider defaults (temperature, topP, maxTokens)
	if provider.Spec.Defaults != nil {
		if provider.Spec.Defaults.Temperature != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_TEMPERATURE",
				Value: *provider.Spec.Defaults.Temperature,
			})
		}
		if provider.Spec.Defaults.TopP != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_TOP_P",
				Value: *provider.Spec.Defaults.TopP,
			})
		}
		if provider.Spec.Defaults.MaxTokens != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_MAX_TOKENS",
				Value: fmt.Sprintf("%d", *provider.Spec.Defaults.MaxTokens),
			})
		}
	}

	// Pricing
	if provider.Spec.Pricing != nil {
		if provider.Spec.Pricing.InputCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_INPUT_COST",
				Value: *provider.Spec.Pricing.InputCostPer1K,
			})
		}
		if provider.Spec.Pricing.OutputCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_OUTPUT_COST",
				Value: *provider.Spec.Pricing.OutputCostPer1K,
			})
		}
		if provider.Spec.Pricing.CachedCostPer1K != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_PROVIDER_CACHED_COST",
				Value: *provider.Spec.Pricing.CachedCostPer1K,
			})
		}
	}

	// API key from secret
	// Use the specific key if provided, otherwise use provider-appropriate keys
	secretRef := corev1.LocalObjectReference{Name: provider.Spec.SecretRef.Name}
	if provider.Spec.SecretRef.Key != nil {
		// User specified a specific key to use
		envVars = append(envVars, buildSecretEnvVarsWithKey(&secretRef, provider.Spec.Type, *provider.Spec.SecretRef.Key)...)
	} else {
		// Use provider-appropriate keys
		envVars = append(envVars, buildSecretEnvVars(&secretRef, provider.Spec.Type)...)
	}

	return envVars
}

// buildSecretEnvVarsWithKey creates environment variables from a secret using a specific key.
func buildSecretEnvVarsWithKey(secretRef *corev1.LocalObjectReference, providerType omniav1alpha1.ProviderType, key string) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Map of provider types to their expected API key env var names
	providerKeyNames := map[omniav1alpha1.ProviderType]string{
		omniav1alpha1.ProviderTypeClaude: "ANTHROPIC_API_KEY",
		omniav1alpha1.ProviderTypeOpenAI: "OPENAI_API_KEY",
		omniav1alpha1.ProviderTypeGemini: "GEMINI_API_KEY",
	}

	// Get the target env var name for this provider type
	envVarName := "ANTHROPIC_API_KEY" // Default
	if name, ok := providerKeyNames[providerType]; ok {
		envVarName = name
	}

	envVars = append(envVars, corev1.EnvVar{
		Name: envVarName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: *secretRef,
				Key:                  key,
			},
		},
	})

	return envVars
}

// Condition types for AgentRuntime
const (
	ConditionTypeReady             = "Ready"
	ConditionTypeDeploymentReady   = "DeploymentReady"
	ConditionTypeServiceReady      = "ServiceReady"
	ConditionTypePromptPackReady   = "PromptPackReady"
	ConditionTypeToolRegistryReady = "ToolRegistryReady"
	ConditionTypeProviderReady     = "ProviderReady"
)

// AgentRuntimeReconciler reconciles a AgentRuntime object
type AgentRuntimeReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	FacadeImage    string
	FrameworkImage string
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

	// Fetch referenced Provider (optional, takes precedence over inline provider)
	var provider *omniav1alpha1.Provider
	if agentRuntime.Spec.ProviderRef != nil {
		provider, err = r.fetchProvider(ctx, agentRuntime)
		if err != nil {
			r.setCondition(agentRuntime, ConditionTypeProviderReady, metav1.ConditionFalse,
				"ProviderNotFound", err.Error())
			agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
			if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, err
		}
		// Check if Provider is ready
		if provider.Status.Phase != omniav1alpha1.ProviderPhaseReady {
			r.setCondition(agentRuntime, ConditionTypeProviderReady, metav1.ConditionFalse,
				"ProviderNotReady", fmt.Sprintf("Provider %s is in %s phase", provider.Name, provider.Status.Phase))
			agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
			if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			// Requeue to wait for Provider to become ready
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		r.setCondition(agentRuntime, ConditionTypeProviderReady, metav1.ConditionTrue,
			"ProviderFound", "Provider resource found and ready")
	}

	// Reconcile tools ConfigMap (if ToolRegistry is present)
	if toolRegistry != nil {
		if err := r.reconcileToolsConfigMap(ctx, agentRuntime, toolRegistry); err != nil {
			log.Error(err, "Failed to reconcile tools ConfigMap")
			// Don't fail the entire reconciliation, tools are optional
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agentRuntime, promptPack, toolRegistry, provider)
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

func (r *AgentRuntimeReconciler) reconcileDeployment(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
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
		r.buildDeploymentSpec(deployment, agentRuntime, promptPack, toolRegistry, provider)
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
	provider *omniav1alpha1.Provider,
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

	facadePort := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		facadePort = *agentRuntime.Spec.Facade.Port
	}

	// Build volumes (shared between containers)
	volumes := r.buildVolumes(agentRuntime, promptPack, toolRegistry)

	// Build facade container
	facadeContainer := r.buildFacadeContainer(agentRuntime, promptPack, facadePort)

	// Build runtime container
	runtimeContainer := r.buildRuntimeContainer(agentRuntime, promptPack, toolRegistry, provider)

	// Build pod spec with both containers
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{facadeContainer, runtimeContainer},
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

	// Prometheus scrape annotations for metrics collection
	podAnnotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   fmt.Sprintf("%d", facadePort),
		"prometheus.io/path":   "/metrics",
	}

	deployment.Labels = labels
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: podAnnotations,
			},
			Spec: podSpec,
		},
	}
}

// buildFacadeContainer creates the facade container spec.
func (r *AgentRuntimeReconciler) buildFacadeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	facadePort int32,
) corev1.Container {
	// Check for CRD image override first, then operator default, then hardcoded default
	facadeImage := ""
	if agentRuntime.Spec.Facade.Image != "" {
		facadeImage = agentRuntime.Spec.Facade.Image
	} else if r.FacadeImage != "" {
		facadeImage = r.FacadeImage
	} else {
		facadeImage = DefaultFacadeImage
	}

	container := corev1.Container{
		Name:            FacadeContainerName,
		Image:           facadeImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "facade",
				ContainerPort: facadePort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "facade-health",
				ContainerPort: DefaultFacadeHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: r.buildFacadeEnvVars(agentRuntime, promptPack),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(DefaultFacadeHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(DefaultFacadeHealthPort),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
	}

	return container
}

// buildRuntimeContainer creates the runtime container spec.
func (r *AgentRuntimeReconciler) buildRuntimeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
) corev1.Container {
	// Check for CRD image override first, then operator default, then hardcoded default
	frameworkImage := ""
	if agentRuntime.Spec.Framework != nil && agentRuntime.Spec.Framework.Image != "" {
		frameworkImage = agentRuntime.Spec.Framework.Image
	} else if r.FrameworkImage != "" {
		frameworkImage = r.FrameworkImage
	} else {
		frameworkImage = DefaultFrameworkImage
	}

	container := corev1.Container{
		Name:            RuntimeContainerName,
		Image:           frameworkImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: DefaultRuntimeGRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "runtime-health",
				ContainerPort: DefaultRuntimeHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          r.buildRuntimeEnvVars(agentRuntime, promptPack, toolRegistry, provider),
		VolumeMounts: r.buildRuntimeVolumeMounts(promptPack, toolRegistry),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(DefaultRuntimeHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(DefaultRuntimeHealthPort),
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

	return container
}

// buildFacadeEnvVars creates environment variables for the facade container.
func (r *AgentRuntimeReconciler) buildFacadeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
) []corev1.EnvVar {
	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

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
		{
			Name:  "OMNIA_FACADE_PORT",
			Value: fmt.Sprintf("%d", port),
		},
		{
			Name:  "OMNIA_HEALTH_PORT",
			Value: fmt.Sprintf("%d", DefaultFacadeHealthPort),
		},
	}

	// Determine handler mode - default to runtime if not specified
	handlerMode := omniav1alpha1.HandlerModeRuntime
	if agentRuntime.Spec.Facade.Handler != nil {
		handlerMode = *agentRuntime.Spec.Facade.Handler
	}

	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_HANDLER_MODE",
		Value: string(handlerMode),
	})

	// Only add runtime address if using runtime handler mode
	if handlerMode == omniav1alpha1.HandlerModeRuntime {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_RUNTIME_ADDRESS",
			Value: fmt.Sprintf("localhost:%d", DefaultRuntimeGRPCPort),
		})
	}

	// Add session config (facade needs this for session management)
	envVars = append(envVars, buildSessionEnvVars(agentRuntime.Spec.Session, "OMNIA_SESSION_STORE_URL")...)

	return envVars
}

// buildRuntimeEnvVars creates environment variables for the runtime container.
func (r *AgentRuntimeReconciler) buildRuntimeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
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
		// PromptPack path for the runtime to load
		{
			Name:  "OMNIA_PROMPTPACK_PATH",
			Value: PromptPackMountPath + "/pack.json",
		},
		// Default prompt name (can be overridden per-request)
		{
			Name:  "OMNIA_PROMPT_NAME",
			Value: "default",
		},
		// gRPC port for the runtime server
		{
			Name:  "OMNIA_GRPC_PORT",
			Value: fmt.Sprintf("%d", DefaultRuntimeGRPCPort),
		},
		// Health check port
		{
			Name:  "OMNIA_HEALTH_PORT",
			Value: fmt.Sprintf("%d", DefaultRuntimeHealthPort),
		},
	}

	// Add provider configuration
	// Provider CRD takes precedence over inline provider config
	if provider != nil {
		envVars = append(envVars, buildProviderEnvVarsFromCRD(provider)...)
	} else {
		envVars = append(envVars, buildProviderEnvVars(agentRuntime.Spec.Provider)...)
	}

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
		// Tools config path
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLS_CONFIG_PATH",
			Value: ToolsMountPath + "/" + ToolsConfigFileName,
		})
	}

	// Add session config for conversation persistence
	envVars = append(envVars, buildSessionEnvVars(agentRuntime.Spec.Session, "OMNIA_SESSION_URL")...)

	// Check for mock provider annotation (for E2E testing)
	if mockProvider, ok := agentRuntime.Annotations[MockProviderAnnotation]; ok && mockProvider == "true" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_MOCK_PROVIDER",
			Value: "true",
		})
	}

	return envVars
}

func (r *AgentRuntimeReconciler) buildVolumes(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.Volume {
	var volumes []corev1.Volume

	// Mount PromptPack ConfigMap if source type is configmap
	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "promptpack-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *promptPack.Spec.Source.ConfigMapRef,
				},
			},
		})
	}

	// Mount tools ConfigMap if ToolRegistry is present
	if toolRegistry != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "tools-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agentRuntime.Name + ToolsConfigMapSuffix,
					},
				},
			},
		})
	}

	return volumes
}

// buildRuntimeVolumeMounts creates volume mounts for the runtime container.
func (r *AgentRuntimeReconciler) buildRuntimeVolumeMounts(
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	// Mount PromptPack ConfigMap
	if promptPack.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
		promptPack.Spec.Source.ConfigMapRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "promptpack-config",
			MountPath: PromptPackMountPath,
			ReadOnly:  true,
		})
	}

	// Mount tools ConfigMap if ToolRegistry is present
	if toolRegistry != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "tools-config",
			MountPath: ToolsMountPath,
			ReadOnly:  true,
		})
	}

	return volumeMounts
}

// ToolConfig represents the tools configuration file format for the runtime.
// This is passed to the runtime container as a YAML file.
type ToolConfig struct {
	Handlers []HandlerEntry `json:"handlers"`
}

// HandlerEntry represents a single handler in the config.
type HandlerEntry struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Endpoint      string          `json:"endpoint"`
	Tool          *ToolDefinition `json:"tool,omitempty"` // For http/grpc handlers
	HTTPConfig    *ToolHTTP       `json:"httpConfig,omitempty"`
	GRPCConfig    *ToolGRPC       `json:"grpcConfig,omitempty"`
	MCPConfig     *ToolMCP        `json:"mcpConfig,omitempty"`
	OpenAPIConfig *ToolOpenAPI    `json:"openAPIConfig,omitempty"`
	Timeout       string          `json:"timeout,omitempty"`
	Retries       int32           `json:"retries,omitempty"`
}

// ToolDefinition represents the tool interface for HTTP/gRPC handlers.
type ToolDefinition struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	InputSchema  interface{} `json:"inputSchema"`
	OutputSchema interface{} `json:"outputSchema,omitempty"`
}

// ToolHTTP represents HTTP configuration for a handler.
type ToolHTTP struct {
	Endpoint    string            `json:"endpoint"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
}

// ToolGRPC represents gRPC configuration for a handler.
type ToolGRPC struct {
	Endpoint              string `json:"endpoint"`
	TLS                   bool   `json:"tls,omitempty"`
	TLSCertPath           string `json:"tlsCertPath,omitempty"`
	TLSKeyPath            string `json:"tlsKeyPath,omitempty"`
	TLSCAPath             string `json:"tlsCAPath,omitempty"`
	TLSInsecureSkipVerify bool   `json:"tlsInsecureSkipVerify,omitempty"`
}

// ToolMCP represents MCP configuration for a handler.
type ToolMCP struct {
	Transport string            `json:"transport"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	WorkDir   string            `json:"workDir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// ToolOpenAPI represents OpenAPI configuration for a handler.
type ToolOpenAPI struct {
	SpecURL         string   `json:"specURL"`
	BaseURL         string   `json:"baseURL,omitempty"`
	OperationFilter []string `json:"operationFilter,omitempty"`
}

// reconcileToolsConfigMap creates or updates the tools ConfigMap from ToolRegistry.
func (r *AgentRuntimeReconciler) reconcileToolsConfigMap(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	toolRegistry *omniav1alpha1.ToolRegistry,
) error {
	log := logf.FromContext(ctx)

	// Build tools config from ToolRegistry
	toolsConfig := r.buildToolsConfig(toolRegistry)

	// Serialize to YAML
	configData, err := yaml.Marshal(toolsConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal tools config: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name + ToolsConfigMapSuffix,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, configMap, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			"app.kubernetes.io/name":         "omnia-agent",
			"app.kubernetes.io/instance":     agentRuntime.Name,
			"app.kubernetes.io/managed-by":   "omnia-operator",
			"omnia.altairalabs.ai/component": "tools-config",
		}

		configMap.Labels = labels
		configMap.Data = map[string]string{
			ToolsConfigFileName: string(configData),
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile tools ConfigMap: %w", err)
	}

	log.Info("Tools ConfigMap reconciled", "result", result, "handlers", len(toolsConfig.Handlers))
	return nil
}

// buildToolsConfig builds the tools configuration from ToolRegistry spec and status.
func (r *AgentRuntimeReconciler) buildToolsConfig(toolRegistry *omniav1alpha1.ToolRegistry) ToolConfig {
	config := ToolConfig{
		Handlers: make([]HandlerEntry, 0, len(toolRegistry.Spec.Handlers)),
	}

	// Build handler entries from spec, using discovered endpoints from status
	for _, h := range toolRegistry.Spec.Handlers {
		// Find the corresponding discovered tool to get the resolved endpoint
		var endpoint string
		for _, discovered := range toolRegistry.Status.DiscoveredTools {
			if discovered.HandlerName == h.Name && discovered.Status == omniav1alpha1.ToolStatusAvailable {
				endpoint = discovered.Endpoint
				break
			}
		}

		// Skip if endpoint couldn't be resolved
		if endpoint == "" {
			continue
		}

		entry := HandlerEntry{
			Name:     h.Name,
			Type:     string(h.Type),
			Endpoint: endpoint,
		}

		// Set timeout and retries
		if h.Timeout != nil {
			entry.Timeout = *h.Timeout
		}
		if h.Retries != nil {
			entry.Retries = *h.Retries
		}

		// Set type-specific configuration
		switch h.Type {
		case omniav1alpha1.HandlerTypeHTTP:
			if h.HTTPConfig != nil {
				entry.HTTPConfig = &ToolHTTP{
					Endpoint:    endpoint,
					Method:      h.HTTPConfig.Method,
					Headers:     h.HTTPConfig.Headers,
					ContentType: h.HTTPConfig.ContentType,
				}
			}
			// Include tool definition for HTTP handlers
			if h.Tool != nil {
				entry.Tool = &ToolDefinition{
					Name:        h.Tool.Name,
					Description: h.Tool.Description,
					InputSchema: h.Tool.InputSchema.Raw,
				}
				if h.Tool.OutputSchema != nil {
					entry.Tool.OutputSchema = h.Tool.OutputSchema.Raw
				}
			}

		case omniav1alpha1.HandlerTypeGRPC:
			if h.GRPCConfig != nil {
				entry.GRPCConfig = &ToolGRPC{
					Endpoint:              endpoint,
					TLS:                   h.GRPCConfig.TLS,
					TLSInsecureSkipVerify: h.GRPCConfig.TLSInsecureSkipVerify,
				}
				if h.GRPCConfig.TLSCertPath != nil {
					entry.GRPCConfig.TLSCertPath = *h.GRPCConfig.TLSCertPath
				}
				if h.GRPCConfig.TLSKeyPath != nil {
					entry.GRPCConfig.TLSKeyPath = *h.GRPCConfig.TLSKeyPath
				}
				if h.GRPCConfig.TLSCAPath != nil {
					entry.GRPCConfig.TLSCAPath = *h.GRPCConfig.TLSCAPath
				}
			}
			// Include tool definition for gRPC handlers
			if h.Tool != nil {
				entry.Tool = &ToolDefinition{
					Name:        h.Tool.Name,
					Description: h.Tool.Description,
					InputSchema: h.Tool.InputSchema.Raw,
				}
				if h.Tool.OutputSchema != nil {
					entry.Tool.OutputSchema = h.Tool.OutputSchema.Raw
				}
			}

		case omniav1alpha1.HandlerTypeMCP:
			if h.MCPConfig != nil {
				entry.MCPConfig = &ToolMCP{
					Transport: string(h.MCPConfig.Transport),
					Env:       h.MCPConfig.Env,
				}
				if h.MCPConfig.Endpoint != nil {
					entry.MCPConfig.Endpoint = *h.MCPConfig.Endpoint
				}
				if h.MCPConfig.Command != nil {
					entry.MCPConfig.Command = *h.MCPConfig.Command
				}
				if len(h.MCPConfig.Args) > 0 {
					entry.MCPConfig.Args = h.MCPConfig.Args
				}
				if h.MCPConfig.WorkDir != nil {
					entry.MCPConfig.WorkDir = *h.MCPConfig.WorkDir
				}
			}

		case omniav1alpha1.HandlerTypeOpenAPI:
			if h.OpenAPIConfig != nil {
				entry.OpenAPIConfig = &ToolOpenAPI{
					SpecURL:         h.OpenAPIConfig.SpecURL,
					OperationFilter: h.OpenAPIConfig.OperationFilter,
				}
				if h.OpenAPIConfig.BaseURL != nil {
					entry.OpenAPIConfig.BaseURL = *h.OpenAPIConfig.BaseURL
				}
			}
		}

		config.Handlers = append(config.Handlers, entry)
	}

	return config
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
			"app.kubernetes.io/name":         "omnia-agent",
			"app.kubernetes.io/instance":     agentRuntime.Name,
			"app.kubernetes.io/managed-by":   "omnia-operator",
			"omnia.altairalabs.ai/component": "agent",
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

func (r *AgentRuntimeReconciler) reconcileAutoscaling(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	// Check if autoscaling is enabled and what type
	if agentRuntime.Spec.Runtime == nil ||
		agentRuntime.Spec.Runtime.Autoscaling == nil ||
		!agentRuntime.Spec.Runtime.Autoscaling.Enabled {
		// Autoscaling disabled - clean up any autoscalers
		if err := r.cleanupHPA(ctx, agentRuntime); err != nil {
			return err
		}
		return r.cleanupKEDA(ctx, agentRuntime)
	}

	autoscaling := agentRuntime.Spec.Runtime.Autoscaling

	// Route based on autoscaler type
	if autoscaling.Type == omniav1alpha1.AutoscalerTypeKEDA {
		// Clean up HPA if switching to KEDA
		if err := r.cleanupHPA(ctx, agentRuntime); err != nil {
			return err
		}
		return r.reconcileKEDA(ctx, agentRuntime)
	}

	// Default to HPA - clean up KEDA if switching from KEDA
	if err := r.cleanupKEDA(ctx, agentRuntime); err != nil {
		return err
	}
	return r.reconcileHPA(ctx, agentRuntime)
}

func (r *AgentRuntimeReconciler) cleanupHPA(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) error {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}
	if err := r.Delete(ctx, hpa); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete HPA: %w", err)
	}
	return nil
}

func (r *AgentRuntimeReconciler) cleanupKEDA(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) error {
	// KEDA ScaledObject cleanup using unstructured
	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(agentRuntime.Name)
	scaledObject.SetNamespace(agentRuntime.Namespace)

	if err := r.Delete(ctx, scaledObject); err != nil {
		// Ignore NotFound (object doesn't exist) and NoMatch (KEDA CRDs not installed)
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to delete ScaledObject: %w", err)
	}
	return nil
}

func (r *AgentRuntimeReconciler) reconcileKEDA(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)

	autoscaling := agentRuntime.Spec.Runtime.Autoscaling

	// Build KEDA ScaledObject using unstructured to avoid dependency on KEDA API
	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(agentRuntime.Name)
	scaledObject.SetNamespace(agentRuntime.Namespace)

	labels := map[string]string{
		"app.kubernetes.io/name":         "omnia-agent",
		"app.kubernetes.io/instance":     agentRuntime.Name,
		"app.kubernetes.io/managed-by":   "omnia-operator",
		"omnia.altairalabs.ai/component": "agent",
	}
	scaledObject.SetLabels(labels)

	// Set owner reference for garbage collection
	ownerRef := metav1.OwnerReference{
		APIVersion:         agentRuntime.APIVersion,
		Kind:               agentRuntime.Kind,
		Name:               agentRuntime.Name,
		UID:                agentRuntime.UID,
		Controller:         ptr(true),
		BlockOwnerDeletion: ptr(true),
	}
	scaledObject.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	// Set defaults
	minReplicas := int64(0) // KEDA supports scale-to-zero
	if autoscaling.MinReplicas != nil {
		minReplicas = int64(*autoscaling.MinReplicas)
	}

	maxReplicas := int64(10)
	if autoscaling.MaxReplicas != nil {
		maxReplicas = int64(*autoscaling.MaxReplicas)
	}

	pollingInterval := int64(30)
	cooldownPeriod := int64(300)
	if autoscaling.KEDA != nil {
		if autoscaling.KEDA.PollingInterval != nil {
			pollingInterval = int64(*autoscaling.KEDA.PollingInterval)
		}
		if autoscaling.KEDA.CooldownPeriod != nil {
			cooldownPeriod = int64(*autoscaling.KEDA.CooldownPeriod)
		}
	}

	// Build triggers
	triggers := r.buildKEDATriggers(agentRuntime)

	// Set spec
	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": agentRuntime.Name,
		},
		"pollingInterval": pollingInterval,
		"cooldownPeriod":  cooldownPeriod,
		"minReplicaCount": minReplicas,
		"maxReplicaCount": maxReplicas,
		"triggers":        triggers,
	}

	if err := unstructured.SetNestedField(scaledObject.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set ScaledObject spec: %w", err)
	}

	// Check if ScaledObject exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	err := r.Get(ctx, types.NamespacedName{Name: agentRuntime.Name, Namespace: agentRuntime.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		// Create ScaledObject
		if err := r.Create(ctx, scaledObject); err != nil {
			return fmt.Errorf("failed to create ScaledObject: %w", err)
		}
		log.Info("Created KEDA ScaledObject")
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get ScaledObject: %w", err)
	}

	// Update existing ScaledObject
	existing.Object["spec"] = scaledObject.Object["spec"]
	existing.SetLabels(labels)
	existing.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ScaledObject: %w", err)
	}
	log.Info("Updated KEDA ScaledObject")

	return nil
}

func (r *AgentRuntimeReconciler) buildKEDATriggers(agentRuntime *omniav1alpha1.AgentRuntime) []interface{} {
	autoscaling := agentRuntime.Spec.Runtime.Autoscaling

	// Use custom triggers if specified
	if autoscaling.KEDA != nil && len(autoscaling.KEDA.Triggers) > 0 {
		triggers := make([]interface{}, 0, len(autoscaling.KEDA.Triggers))
		for _, t := range autoscaling.KEDA.Triggers {
			// Convert map[string]string to map[string]interface{} for unstructured
			metadata := make(map[string]interface{}, len(t.Metadata))
			for k, v := range t.Metadata {
				metadata[k] = v
			}
			triggers = append(triggers, map[string]interface{}{
				"type":     t.Type,
				"metadata": metadata,
			})
		}
		return triggers
	}

	// Default: Prometheus trigger for active connections
	// This assumes Prometheus is configured via the Omnia Helm chart with default settings
	// Users with custom Prometheus setups should specify triggers explicitly
	return []interface{}{
		map[string]interface{}{
			"type": "prometheus",
			"metadata": map[string]interface{}{
				"serverAddress": "http://omnia-prometheus-server.omnia-system.svc.cluster.local/prometheus",
				"query":         fmt.Sprintf(`sum(omnia_agent_connections_active{agent="%s",namespace="%s"}) or vector(0)`, agentRuntime.Name, agentRuntime.Namespace),
				"threshold":     "10", // Scale when avg connections per pod > 10
			},
		},
	}
}

func (r *AgentRuntimeReconciler) reconcileHPA(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	autoscaling := agentRuntime.Spec.Runtime.Autoscaling
	if autoscaling == nil || !autoscaling.Enabled {
		// Delete HPA if it exists
		if err := r.Delete(ctx, hpa); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to delete HPA: %w", err)
		}
		log.Info("Deleted HPA (autoscaling disabled)")
		return nil
	}

	// Create or update HPA
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, hpa, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, hpa, r.Scheme); err != nil {
			return err
		}

		autoscaling := agentRuntime.Spec.Runtime.Autoscaling

		// Set defaults
		minReplicas := int32(1)
		if autoscaling.MinReplicas != nil {
			minReplicas = *autoscaling.MinReplicas
		}

		maxReplicas := int32(10)
		if autoscaling.MaxReplicas != nil {
			maxReplicas = *autoscaling.MaxReplicas
		}

		// Memory is the primary metric (default 70%)
		// Agents are I/O bound, not CPU bound - each connection uses memory
		targetMemory := int32(70)
		if autoscaling.TargetMemoryUtilizationPercentage != nil {
			targetMemory = *autoscaling.TargetMemoryUtilizationPercentage
		}

		// CPU is secondary/safety valve (default 90%)
		targetCPU := int32(90)
		if autoscaling.TargetCPUUtilizationPercentage != nil {
			targetCPU = *autoscaling.TargetCPUUtilizationPercentage
		}

		// Scale-down stabilization (default 5 minutes)
		// Prevents thrashing when connections are bursty
		scaleDownStabilization := int32(300)
		if autoscaling.ScaleDownStabilizationSeconds != nil {
			scaleDownStabilization = *autoscaling.ScaleDownStabilizationSeconds
		}

		labels := map[string]string{
			"app.kubernetes.io/name":         "omnia-agent",
			"app.kubernetes.io/instance":     agentRuntime.Name,
			"app.kubernetes.io/managed-by":   "omnia-operator",
			"omnia.altairalabs.ai/component": "agent",
		}

		hpa.Labels = labels
		hpa.Spec = autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       agentRuntime.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			// Memory is primary metric for agents (I/O bound workloads)
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &targetMemory,
						},
					},
				},
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &targetCPU,
						},
					},
				},
			},
			// Behavior controls scale-up/scale-down rates
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: &scaleDownStabilization,
					Policies: []autoscalingv2.HPAScalingPolicy{
						{
							Type:          autoscalingv2.PercentScalingPolicy,
							Value:         50, // Scale down max 50% of pods at a time
							PeriodSeconds: 60,
						},
					},
				},
				ScaleUp: &autoscalingv2.HPAScalingRules{
					// Scale up faster than scale down (responsive to load)
					StabilizationWindowSeconds: ptr(int32(0)),
					Policies: []autoscalingv2.HPAScalingPolicy{
						{
							Type:          autoscalingv2.PercentScalingPolicy,
							Value:         100, // Can double pods
							PeriodSeconds: 15,
						},
						{
							Type:          autoscalingv2.PodsScalingPolicy,
							Value:         4, // Or add up to 4 pods
							PeriodSeconds: 15,
						},
					},
					SelectPolicy: ptrSelectPolicy(autoscalingv2.MaxChangePolicySelect),
				},
			},
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile HPA: %w", err)
	}

	log.Info("HPA reconciled", "result", result)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.AgentRuntime{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Named("agentruntime").
		Complete(r)
}
