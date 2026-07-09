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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ToolRegistry condition types
const (
	ToolRegistryConditionTypeToolsDiscovered = "ToolsDiscovered"
	ToolRegistryConditionTypeHandlersValid   = "HandlersValid"
)

// ToolRegistryReconciler reconciles a ToolRegistry object
type ToolRegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

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

	// Optionally probe endpoint reachability (off by default), which can flip a
	// tool to Unavailable and drive the registry to Degraded/Failed.
	r.probeTools(ctx, toolRegistry, discoveredTools)

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

	// When probing is enabled, requeue to re-probe on the configured interval.
	if requeue := probeRequeueAfter(toolRegistry.Spec.Probe); requeue > 0 {
		return ctrl.Result{RequeueAfter: requeue}, nil
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

		// Resolve the handler endpoint from its type-specific config.
		// A validated handler always resolves to an endpoint, so this failure
		// branch is currently unreachable; it is retained as the wiring point
		// for future probe-backed status (marking a reachable-but-unhealthy tool
		// Unavailable → Degraded). See #1791.
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
	// Reject unknown handler types using the authoritative set from the API types.
	if !omniav1alpha1.ValidHandlerTypes[h.Type] {
		return fmt.Errorf("unknown handler type: %s", h.Type)
	}

	// Type-specific config validation.
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
		if h.MCPConfig.Transport == omniav1alpha1.MCPTransportStreamableHTTP && h.MCPConfig.Endpoint == nil {
			return fmt.Errorf("endpoint is required for mcp handlers with streamable-http transport")
		}
		if h.MCPConfig.Transport == omniav1alpha1.MCPTransportStdio && h.MCPConfig.Command == nil {
			return fmt.Errorf("command is required for mcp handlers with stdio transport")
		}
	case omniav1alpha1.HandlerTypeOpenAPI:
		if h.OpenAPIConfig == nil {
			return fmt.Errorf("openAPIConfig is required for openapi handlers")
		}
	case omniav1alpha1.HandlerTypeClient:
		// Client handlers execute in the browser but still declare their tool
		// interface (name/description/inputSchema) like http/grpc handlers.
		if h.Tool == nil {
			return fmt.Errorf("tool definition is required for client handlers")
		}
	}

	// Retry policy validation: invoke the per-transport builders with discarded
	// results so any policy-level error (bad BackoffMultiplier, unknown gRPC
	// status code, MaxBackoff < InitialBackoff, etc.) surfaces as a handler
	// validation failure and feeds the existing HandlersValid condition.
	return validateRetryPolicies(h)
}

// validateRetryPolicies invokes the per-transport retry policy builders on
// the handler's config, discarding results and returning the first validation
// error found.
func validateRetryPolicies(h *omniav1alpha1.HandlerDefinition) error {
	if h.HTTPConfig != nil && h.HTTPConfig.RetryPolicy != nil {
		if _, err := buildHTTPRetryPolicy(h.HTTPConfig.RetryPolicy); err != nil {
			return err
		}
	}
	if h.GRPCConfig != nil && h.GRPCConfig.RetryPolicy != nil {
		if _, err := buildGRPCRetryPolicy(h.GRPCConfig.RetryPolicy); err != nil {
			return err
		}
	}
	if h.MCPConfig != nil && h.MCPConfig.RetryPolicy != nil {
		if _, err := buildMCPRetryPolicy(h.MCPConfig.RetryPolicy); err != nil {
			return err
		}
	}
	if h.OpenAPIConfig != nil && h.OpenAPIConfig.RetryPolicy != nil {
		if _, err := buildHTTPRetryPolicy(h.OpenAPIConfig.RetryPolicy); err != nil {
			return err
		}
	}
	return nil
}

// resolveEndpoint resolves the handler endpoint from its type-specific config.
func (r *ToolRegistryReconciler) resolveEndpoint(_ context.Context, _ *omniav1alpha1.ToolRegistry, h *omniav1alpha1.HandlerDefinition) (string, error) {
	switch h.Type {
	case omniav1alpha1.HandlerTypeHTTP:
		return h.HTTPConfig.Endpoint, nil
	case omniav1alpha1.HandlerTypeGRPC:
		return h.GRPCConfig.Endpoint, nil
	case omniav1alpha1.HandlerTypeMCP:
		if h.MCPConfig.Endpoint != nil {
			return *h.MCPConfig.Endpoint, nil
		}
		if h.MCPConfig.Command != nil {
			return fmt.Sprintf("stdio://%s", *h.MCPConfig.Command), nil
		}
		return "", fmt.Errorf("no endpoint configured for MCP handler")
	case omniav1alpha1.HandlerTypeOpenAPI:
		return h.OpenAPIConfig.SpecURL, nil
	case omniav1alpha1.HandlerTypeClient:
		return "client://browser", nil
	}
	return "", fmt.Errorf("cannot determine endpoint for handler type %s", h.Type)
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

	case omniav1alpha1.HandlerTypeClient:
		// Client-side tools execute in the browser via WebSocket.
		// They have explicit tool definitions like HTTP/gRPC handlers.
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

// SetupWithManager sets up the controller with the Manager.
func (r *ToolRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		// Reconcile on spec changes only. The controller writes its own status
		// (LastDiscoveryTime, and per-tool LastChecked when probing) every pass;
		// without this predicate those status writes would re-enqueue Reconcile
		// immediately, turning interval-paced probing into a continuous loop.
		// Re-probing is instead driven by the RequeueAfter interval.
		For(&omniav1alpha1.ToolRegistry{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("toolregistry").
		Complete(r)
}
