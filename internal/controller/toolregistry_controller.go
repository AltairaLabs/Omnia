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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ToolRegistry condition types
const (
	ToolRegistryConditionTypeToolsDiscovered = "ToolsDiscovered"
	ToolRegistryConditionTypeServicesFound   = "ServicesFound"
)

// Service annotation keys for tool metadata
const (
	AnnotationToolName        = "omnia.altairalabs.ai/tool-name"
	AnnotationToolDescription = "omnia.altairalabs.ai/tool-description"
	AnnotationToolType        = "omnia.altairalabs.ai/tool-type"
	AnnotationToolPath        = "omnia.altairalabs.ai/tool-path"
)

// ToolRegistryReconciler reconciles a ToolRegistry object
type ToolRegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ToolRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ToolRegistry", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ToolRegistry instance
	toolRegistry := &omniav1alpha1.ToolRegistry{}
	if err := r.Get(ctx, req.NamespacedName, toolRegistry); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ToolRegistry resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ToolRegistry")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if toolRegistry.Status.Phase == "" {
		toolRegistry.Status.Phase = omniav1alpha1.ToolRegistryPhasePending
	}

	// Discover tools
	discoveredTools := r.discoverTools(ctx, toolRegistry)

	// Update status with discovered tools
	toolRegistry.Status.DiscoveredTools = discoveredTools
	toolRegistry.Status.DiscoveredToolsCount = int32(len(discoveredTools))
	now := metav1.Now()
	toolRegistry.Status.LastDiscoveryTime = &now

	// Determine phase based on tool availability
	toolRegistry.Status.Phase = r.determinePhase(discoveredTools)

	// Set conditions
	r.setCondition(toolRegistry, ToolRegistryConditionTypeToolsDiscovered, metav1.ConditionTrue,
		"ToolsDiscovered", fmt.Sprintf("Discovered %d tool(s)", len(discoveredTools)))

	if err := r.Status().Update(ctx, toolRegistry); err != nil {
		log.Error(err, "Failed to update ToolRegistry status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// discoverTools processes all tool definitions and discovers their endpoints.
func (r *ToolRegistryReconciler) discoverTools(ctx context.Context, toolRegistry *omniav1alpha1.ToolRegistry) []omniav1alpha1.DiscoveredTool {
	discoveredTools := make([]omniav1alpha1.DiscoveredTool, 0, len(toolRegistry.Spec.Tools))

	for _, tool := range toolRegistry.Spec.Tools {
		discovered, err := r.discoverTool(ctx, toolRegistry, &tool)
		if err != nil {
			// Log but continue with other tools
			logf.FromContext(ctx).Error(err, "Failed to discover tool", "tool", tool.Name)
			now := metav1.Now()
			discoveredTools = append(discoveredTools, omniav1alpha1.DiscoveredTool{
				Name:        tool.Name,
				Endpoint:    "",
				Status:      omniav1alpha1.ToolStatusUnavailable,
				LastChecked: &now,
			})
			continue
		}
		discoveredTools = append(discoveredTools, *discovered)
	}

	return discoveredTools
}

// discoverTool discovers a single tool's endpoint.
func (r *ToolRegistryReconciler) discoverTool(ctx context.Context, toolRegistry *omniav1alpha1.ToolRegistry, tool *omniav1alpha1.ToolDefinition) (*omniav1alpha1.DiscoveredTool, error) {
	now := metav1.Now()

	// If URL is specified directly, use it
	if tool.Endpoint.URL != nil && *tool.Endpoint.URL != "" {
		return &omniav1alpha1.DiscoveredTool{
			Name:        tool.Name,
			Endpoint:    *tool.Endpoint.URL,
			Status:      omniav1alpha1.ToolStatusAvailable,
			LastChecked: &now,
		}, nil
	}

	// If selector is specified, discover via Services
	if tool.Endpoint.Selector != nil {
		return r.discoverToolViaSelector(ctx, toolRegistry, tool)
	}

	return nil, fmt.Errorf("tool %q has no endpoint URL or selector", tool.Name)
}

// discoverToolViaSelector discovers a tool endpoint by finding matching Services.
func (r *ToolRegistryReconciler) discoverToolViaSelector(ctx context.Context, toolRegistry *omniav1alpha1.ToolRegistry, tool *omniav1alpha1.ToolDefinition) (*omniav1alpha1.DiscoveredTool, error) {
	now := metav1.Now()
	selector := tool.Endpoint.Selector

	// Determine namespace to search
	namespace := toolRegistry.Namespace
	if selector.Namespace != nil && *selector.Namespace != "" {
		namespace = *selector.Namespace
	}

	// Build label selector
	labelSelector := labels.SelectorFromSet(selector.MatchLabels)

	// List matching services
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	if len(serviceList.Items) == 0 {
		return &omniav1alpha1.DiscoveredTool{
			Name:        tool.Name,
			Endpoint:    "",
			Status:      omniav1alpha1.ToolStatusUnavailable,
			LastChecked: &now,
		}, nil
	}

	// Use the first matching service
	svc := &serviceList.Items[0]
	endpoint := r.buildServiceEndpoint(svc, selector.Port, tool.Type)

	return &omniav1alpha1.DiscoveredTool{
		Name:        tool.Name,
		Endpoint:    endpoint,
		Status:      omniav1alpha1.ToolStatusAvailable,
		LastChecked: &now,
	}, nil
}

// buildServiceEndpoint constructs an endpoint URL from a Service.
func (r *ToolRegistryReconciler) buildServiceEndpoint(svc *corev1.Service, portSpec *string, toolType omniav1alpha1.ToolType) string {
	// Determine the port
	var port int32
	if len(svc.Spec.Ports) > 0 {
		port = svc.Spec.Ports[0].Port
		// If a specific port is requested, find it
		if portSpec != nil && *portSpec != "" {
			for _, p := range svc.Spec.Ports {
				if p.Name == *portSpec {
					port = p.Port
					break
				}
			}
		}
	}

	// Determine protocol based on tool type
	protocol := "http"
	if toolType == omniav1alpha1.ToolTypeGRPC {
		protocol = "grpc"
	}

	// Check for path annotation
	path := ""
	if p, ok := svc.Annotations[AnnotationToolPath]; ok {
		path = p
	}

	// Build the endpoint URL
	return fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d%s",
		protocol, svc.Name, svc.Namespace, port, path)
}

// determinePhase determines the registry phase based on discovered tools.
func (r *ToolRegistryReconciler) determinePhase(discoveredTools []omniav1alpha1.DiscoveredTool) omniav1alpha1.ToolRegistryPhase {
	if len(discoveredTools) == 0 {
		return omniav1alpha1.ToolRegistryPhaseFailed
	}

	availableCount := 0
	for _, tool := range discoveredTools {
		if tool.Status == omniav1alpha1.ToolStatusAvailable {
			availableCount++
		}
	}

	if availableCount == len(discoveredTools) {
		return omniav1alpha1.ToolRegistryPhaseReady
	}
	if availableCount > 0 {
		return omniav1alpha1.ToolRegistryPhaseDegraded
	}
	return omniav1alpha1.ToolRegistryPhaseFailed
}

// setCondition sets a condition on the ToolRegistry status.
func (r *ToolRegistryReconciler) setCondition(
	toolRegistry *omniav1alpha1.ToolRegistry,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&toolRegistry.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: toolRegistry.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// findToolRegistriesForService maps a Service to ToolRegistries that might reference it.
func (r *ToolRegistryReconciler) findToolRegistriesForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc := obj.(*corev1.Service)
	log := logf.FromContext(ctx)

	// List all ToolRegistries
	toolRegistryList := &omniav1alpha1.ToolRegistryList{}
	if err := r.List(ctx, toolRegistryList); err != nil {
		log.Error(err, "Failed to list ToolRegistries for Service mapping")
		return nil
	}

	var requests []reconcile.Request
	for _, tr := range toolRegistryList.Items {
		// Check if any tool in this registry uses a selector that might match this service
		for _, tool := range tr.Spec.Tools {
			if tool.Endpoint.Selector != nil && r.selectorMatchesService(tool.Endpoint.Selector, svc, tr.Namespace) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      tr.Name,
						Namespace: tr.Namespace,
					},
				})
				break // Only add once per registry
			}
		}
	}

	return requests
}

// selectorMatchesService checks if a tool selector matches a service.
func (r *ToolRegistryReconciler) selectorMatchesService(selector *omniav1alpha1.ToolSelector, svc *corev1.Service, registryNamespace string) bool {
	// Check namespace
	targetNamespace := registryNamespace
	if selector.Namespace != nil && *selector.Namespace != "" {
		targetNamespace = *selector.Namespace
	}
	if svc.Namespace != targetNamespace {
		return false
	}

	// Check labels
	for key, value := range selector.MatchLabels {
		if svc.Labels[key] != value {
			return false
		}
	}

	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *ToolRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ToolRegistry{}).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findToolRegistriesForService),
		).
		Named("toolregistry").
		Complete(r)
}
