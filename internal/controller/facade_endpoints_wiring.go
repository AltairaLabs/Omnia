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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// gatewayAPIAvailable reports whether the HTTPRoute kind is served by the
// cluster's API server. Used at SetupWithManager time to gate the
// HTTPRoute/Gateway watches and the facade-endpoint reconcile path: when the
// Gateway API CRDs are not installed, listing them returns a NoMatch error and
// the controller would otherwise crash-loop, so external facade endpoints are
// disabled until the operator is restarted with the CRDs present.
func gatewayAPIAvailable(mapper apimeta.RESTMapper) bool {
	_, err := mapper.RESTMapping(
		schema.GroupKind{Group: gatewayv1.GroupVersion.Group, Kind: "HTTPRoute"},
		gatewayv1.GroupVersion.Version,
	)
	return err == nil
}

// reconcileFacadeEndpoints lists HTTPRoutes in the agent's namespace, builds the
// external endpoints via BuildFacadeEndpoints, and sets status.facade. It is a
// no-op that clears status.facade when the Gateway API is unavailable or no
// route matches. Cluster reads are confined to this method; the matcher itself
// is pure. Transient list errors leave the previous status untouched so a flake
// doesn't blank a working endpoint set.
func (r *AgentRuntimeReconciler) reconcileFacadeEndpoints(ctx context.Context, agent *omniav1alpha1.AgentRuntime) {
	log := logf.FromContext(ctx)
	if !r.gatewayAPIPresent {
		agent.Status.Facade = nil
		return
	}
	var routes gatewayv1.HTTPRouteList
	if err := r.List(ctx, &routes, client.InNamespace(agent.Namespace)); err != nil {
		if apimeta.IsNoMatchError(err) {
			agent.Status.Facade = nil
			return
		}
		log.V(1).Info("facade endpoints list skipped",
			"reason", "transient list error",
			"agent", agent.Name,
			"namespace", agent.Namespace)
		return
	}
	eps := BuildFacadeEndpoints(agent, routes.Items, r.gatewayResolver(ctx, agent.Namespace))
	if len(eps) == 0 {
		agent.Status.Facade = nil
		return
	}
	agent.Status.Facade = &omniav1alpha1.FacadeStatus{Endpoints: eps}
}

// gatewayResolver returns a GatewayResolver backed by the controller client,
// resolving each parentRef's Gateway (defaulting the namespace to the route's).
func (r *AgentRuntimeReconciler) gatewayResolver(ctx context.Context, routeNamespace string) GatewayResolver {
	return func(parent gatewayv1.ParentReference, ns string) (*gatewayv1.Gateway, bool) {
		gwNS := ns
		if gwNS == "" {
			gwNS = routeNamespace
		}
		if parent.Namespace != nil {
			gwNS = string(*parent.Namespace)
		}
		var g gatewayv1.Gateway
		if err := r.Get(ctx, client.ObjectKey{Name: string(parent.Name), Namespace: gwNS}, &g); err != nil {
			return nil, false
		}
		return &g, true
	}
}

// findAgentRuntimesForHTTPRoute maps an HTTPRoute change to the AgentRuntimes in
// the same namespace that the route's backendRefs name, so their facade status
// is recomputed when routing changes.
func (r *AgentRuntimeReconciler) findAgentRuntimesForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	var agents omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agents, client.InNamespace(route.Namespace)); err != nil {
		return nil
	}
	backendNames := map[string]struct{}{}
	for ri := range route.Spec.Rules {
		for _, b := range route.Spec.Rules[ri].BackendRefs {
			backendNames[string(b.Name)] = struct{}{}
		}
	}
	var reqs []reconcile.Request
	for i := range agents.Items {
		if _, hit := backendNames[agents.Items[i].Name]; hit {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&agents.Items[i])})
		}
	}
	return reqs
}

// findAgentRuntimesForGateway enqueues every AgentRuntime when a Gateway
// changes. A Gateway's scheme (TLS) or hostname can change in ways that affect
// any route attached to it, so the simplest correct behaviour is to recompute
// all AgentRuntimes; this is bounded by the agent count and acceptable.
func (r *AgentRuntimeReconciler) findAgentRuntimesForGateway(ctx context.Context, _ client.Object) []reconcile.Request {
	var agents omniav1alpha1.AgentRuntimeList
	if err := r.List(ctx, &agents); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(agents.Items))
	for i := range agents.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&agents.Items[i])})
	}
	return reqs
}
