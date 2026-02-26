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
	ToolRegistryConditionTypeHandlersValid   = "HandlersValid"
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

	// Validate handlers and discover tools
	discoveredTools, validationErrors := r.processHandlers(ctx, toolRegistry)

	// Update status with discovered tools
	toolRegistry.Status.DiscoveredTools = discoveredTools
	toolRegistry.Status.DiscoveredToolsCount = int32(len(discoveredTools))
	now := metav1.Now()
	toolRegistry.Status.LastDiscoveryTime = &now

	// Determine phase based on tool availability and validation
	if len(validationErrors) > 0 {
		toolRegistry.Status.Phase = omniav1alpha1.ToolRegistryPhaseFailed
		SetCondition(&toolRegistry.Status.Conditions, toolRegistry.Generation, ToolRegistryConditionTypeHandlersValid, metav1.ConditionFalse,
			"ValidationFailed", fmt.Sprintf("Handler validation errors: %v", validationErrors))
	} else {
		toolRegistry.Status.Phase = r.determinePhase(discoveredTools)
		SetCondition(&toolRegistry.Status.Conditions, toolRegistry.Generation, ToolRegistryConditionTypeHandlersValid, metav1.ConditionTrue,
			"HandlersValid", "All handlers validated successfully")
	}

	// Set discovery condition
	SetCondition(&toolRegistry.Status.Conditions, toolRegistry.Generation, ToolRegistryConditionTypeToolsDiscovered, metav1.ConditionTrue,
		"ToolsDiscovered", fmt.Sprintf("Discovered %d tool(s) from %d handler(s)",
			len(discoveredTools), len(toolRegistry.Spec.Handlers)))

	if err := r.Status().Update(ctx, toolRegistry); err != nil {
		log.Error(err, "Failed to update ToolRegistry status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// processHandlers validates handlers and discovers tools from them.
func (r *ToolRegistryReconciler) processHandlers(ctx context.Context, toolRegistry *omniav1alpha1.ToolRegistry) ([]omniav1alpha1.DiscoveredTool, []string) {
	var discoveredTools []omniav1alpha1.DiscoveredTool
	var validationErrors []string
	log := logf.FromContext(ctx)

	for _, h := range toolRegistry.Spec.Handlers {
		// Validate handler configuration
		if err := r.validateHandler(&h); err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("handler %q: %v", h.Name, err))
			continue
		}

		// Resolve endpoint if using service selector
		endpoint, err := r.resolveEndpoint(ctx, toolRegistry, &h)
		if err != nil {
			log.Error(err, "Failed to resolve endpoint", "handler", h.Name)
			now := metav1.Now()
			errMsg := err.Error()
			discoveredTools = append(discoveredTools, omniav1alpha1.DiscoveredTool{
				Name:        h.Name,
				HandlerName: h.Name,
				Description: fmt.Sprintf("Handler %s (endpoint resolution failed)", h.Type),
				Endpoint:    "",
				Status:      omniav1alpha1.ToolStatusUnavailable,
				LastChecked: &now,
				Error:       &errMsg,
			})
			continue
		}

		// Process based on handler type
		tools := r.discoverToolsFromHandler(&h, endpoint)
		discoveredTools = append(discoveredTools, tools...)
	}

	return discoveredTools, validationErrors
}

// validateHandler validates a handler configuration.
func (r *ToolRegistryReconciler) validateHandler(h *omniav1alpha1.HandlerDefinition) error {
	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		if h.HTTPConfig == nil {
			return fmt.Errorf("httpConfig is required for http handlers")
		}
		if h.Tool == nil {
			return fmt.Errorf("tool definition is required for http handlers")
		}
	case omniav1alpha1.HandlerTypeGRPC:
		if h.GRPCConfig == nil {
			return fmt.Errorf("grpcConfig is required for grpc handlers")
		}
		if h.Tool == nil {
			return fmt.Errorf("tool definition is required for grpc handlers")
		}
	case omniav1alpha1.HandlerTypeMCP:
		if h.MCPConfig == nil {
			return fmt.Errorf("mcpConfig is required for mcp handlers")
		}
		if h.MCPConfig.Transport == omniav1alpha1.MCPTransportSSE && h.MCPConfig.Endpoint == nil {
			return fmt.Errorf("endpoint is required for mcp handlers with SSE transport")
		}
		if h.MCPConfig.Transport == omniav1alpha1.MCPTransportStdio && h.MCPConfig.Command == nil {
			return fmt.Errorf("command is required for mcp handlers with stdio transport")
		}
	case omniav1alpha1.HandlerTypeOpenAPI:
		if h.OpenAPIConfig == nil {
			return fmt.Errorf("openAPIConfig is required for openapi handlers")
		}
	default:
		return fmt.Errorf("unknown handler type: %s", h.Type)
	}
	return nil
}

// resolveEndpoint resolves the handler endpoint, using service selector if specified.
func (r *ToolRegistryReconciler) resolveEndpoint(ctx context.Context, toolRegistry *omniav1alpha1.ToolRegistry, h *omniav1alpha1.HandlerDefinition) (string, error) {
	// If using service selector, discover via Services
	if h.Selector != nil {
		return r.resolveEndpointViaSelector(ctx, toolRegistry.Namespace, h)
	}

	// Otherwise, use the endpoint from type-specific config
	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		return h.HTTPConfig.Endpoint, nil
	case omniav1alpha1.HandlerTypeGRPC:
		return h.GRPCConfig.Endpoint, nil
	case omniav1alpha1.HandlerTypeMCP:
		if h.MCPConfig.Endpoint != nil {
			return *h.MCPConfig.Endpoint, nil
		}
		// For stdio transport, return command as "endpoint"
		if h.MCPConfig.Command != nil {
			return fmt.Sprintf("stdio://%s", *h.MCPConfig.Command), nil
		}
		return "", fmt.Errorf("no endpoint configured for MCP handler")
	case omniav1alpha1.HandlerTypeOpenAPI:
		return h.OpenAPIConfig.SpecURL, nil
	}

	return "", fmt.Errorf("cannot determine endpoint for handler type %s", h.Type)
}

// resolveEndpointViaSelector discovers an endpoint by finding matching Services.
func (r *ToolRegistryReconciler) resolveEndpointViaSelector(ctx context.Context, registryNamespace string, h *omniav1alpha1.HandlerDefinition) (string, error) {
	selector := h.Selector

	// Determine namespace to search
	namespace := registryNamespace
	if selector.Namespace != nil && *selector.Namespace != "" {
		namespace = *selector.Namespace
	}

	// Build label selector
	labelSelector := labels.SelectorFromSet(selector.MatchLabels)

	// List matching services
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}

	if len(serviceList.Items) == 0 {
		return "", fmt.Errorf("no services found matching selector in namespace %s", namespace)
	}

	// Use the first matching service
	svc := &serviceList.Items[0]
	return r.buildServiceEndpoint(svc, selector.Port, h.Type), nil
}

// buildServiceEndpoint constructs an endpoint URL from a Service.
func (r *ToolRegistryReconciler) buildServiceEndpoint(svc *corev1.Service, portSpec *string, handlerType omniav1alpha1.HandlerType) string {
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

	// Determine protocol based on handler type
	protocol := "http"
	if handlerType == omniav1alpha1.HandlerTypeGRPC {
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

// discoverToolsFromHandler creates discovered tool entries for a handler.
func (r *ToolRegistryReconciler) discoverToolsFromHandler(h *omniav1alpha1.HandlerDefinition, endpoint string) []omniav1alpha1.DiscoveredTool {
	now := metav1.Now()

	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP, omniav1alpha1.HandlerTypeGRPC:
		// For HTTP/gRPC, the tool definition is explicit in the handler
		if h.Tool == nil {
			return nil
		}
		tool := omniav1alpha1.DiscoveredTool{
			Name:        h.Tool.Name,
			HandlerName: h.Name,
			Description: h.Tool.Description,
			InputSchema: &h.Tool.InputSchema,
			Endpoint:    endpoint,
			Status:      omniav1alpha1.ToolStatusAvailable,
			LastChecked: &now,
		}
		if h.Tool.OutputSchema != nil {
			tool.OutputSchema = h.Tool.OutputSchema
		}
		return []omniav1alpha1.DiscoveredTool{tool}

	case omniav1alpha1.HandlerTypeMCP, omniav1alpha1.HandlerTypeOpenAPI:
		// For self-describing handlers, we create a placeholder
		// Actual tools will be discovered at runtime
		return []omniav1alpha1.DiscoveredTool{
			{
				Name:        h.Name,
				HandlerName: h.Name,
				Description: fmt.Sprintf("Self-describing %s handler (tools discovered at runtime)", h.Type),
				Endpoint:    endpoint,
				Status:      omniav1alpha1.ToolStatusAvailable, // Endpoint is reachable
				LastChecked: &now,
			},
		}
	}

	return nil
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
		// Check if any handler in this registry uses a selector that might match this service
		for _, h := range tr.Spec.Handlers {
			if h.Selector != nil && r.selectorMatchesService(h.Selector, svc, tr.Namespace) {
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

// selectorMatchesService checks if a service selector matches a service.
func (r *ToolRegistryReconciler) selectorMatchesService(selector *omniav1alpha1.ServiceSelector, svc *corev1.Service, registryNamespace string) bool {
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
