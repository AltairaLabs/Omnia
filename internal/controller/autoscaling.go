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

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

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
		Group:   kedaAPIGroup,
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
		Group:   kedaAPIGroup,
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(agentRuntime.Name)
	scaledObject.SetNamespace(agentRuntime.Namespace)

	labels := map[string]string{
		labelAppName:      labelValueOmniaAgent,
		labelAppInstance:  agentRuntime.Name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    "agent",
	}
	scaledObject.SetLabels(labels)

	// Set owner reference for garbage collection
	ownerRef := metav1.OwnerReference{
		APIVersion:         agentRuntime.APIVersion,
		Kind:               agentRuntime.Kind,
		Name:               agentRuntime.Name,
		UID:                agentRuntime.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
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
		Group:   kedaAPIGroup,
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
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
			labelOmniaComp:    "agent",
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
					StabilizationWindowSeconds: ptr.To(int32(0)),
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
					SelectPolicy: ptr.To(autoscalingv2.MaxChangePolicySelect),
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
