/*
Copyright 2026.

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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	istioNetworkingAPIVersion = "networking.istio.io/v1"
	istioVirtualServiceKind   = "VirtualService"
)

// patchVirtualServiceWeights patches the HTTP route weights of an Istio
// VirtualService for canary traffic splitting. It uses the unstructured client
// so the operator does not require Istio type imports.
func (r *AgentRuntimeReconciler) patchVirtualServiceWeights(
	ctx context.Context,
	namespace string,
	istioConfig *omniav1alpha1.IstioTrafficRouting,
	candidateWeight int32,
) error {
	log := logf.FromContext(ctx)

	vsName := istioConfig.VirtualService.Name
	vs := &unstructured.Unstructured{}
	vs.SetAPIVersion(istioNetworkingAPIVersion)
	vs.SetKind(istioVirtualServiceKind)

	if err := r.Get(ctx, types.NamespacedName{Name: vsName, Namespace: namespace}, vs); err != nil {
		if isNoMatchError(err) {
			log.V(1).Info("istio CRDs not installed", "reason", "VirtualService kind not registered")
			return nil
		}
		return fmt.Errorf("get VirtualService %q: %w", vsName, err)
	}

	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil {
		return fmt.Errorf("read spec.http from VirtualService %q: %w", vsName, err)
	}
	if !found {
		return fmt.Errorf("VirtualService %q has no spec.http routes", vsName)
	}

	stableSubset := istioConfig.DestinationRule.StableSubset
	candidateSubset := istioConfig.DestinationRule.CandidateSubset
	stableWeight := int64(100 - candidateWeight)
	candWeight := int64(candidateWeight)
	targetRoutes := istioConfig.VirtualService.Routes

	for i, raw := range httpRoutes {
		route, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(route, "name")
		if !isTargetRoute(name, targetRoutes) {
			continue
		}
		patchRouteWeights(route, stableSubset, candidateSubset, stableWeight, candWeight)
		httpRoutes[i] = route
	}

	if err := unstructured.SetNestedSlice(vs.Object, httpRoutes, "spec", "http"); err != nil {
		return fmt.Errorf("set spec.http on VirtualService %q: %w", vsName, err)
	}

	if err := r.Update(ctx, vs); err != nil {
		return fmt.Errorf("update VirtualService %q: %w", vsName, err)
	}

	log.V(1).Info("patched VirtualService weights",
		"virtualService", vsName,
		"stableWeight", stableWeight,
		"candidateWeight", candWeight)
	return nil
}

// resetTrafficRouting resets traffic to 100% stable by setting candidate weight to 0.
func (r *AgentRuntimeReconciler) resetTrafficRouting(
	ctx context.Context,
	namespace string,
	istioConfig *omniav1alpha1.IstioTrafficRouting,
) error {
	return r.patchVirtualServiceWeights(ctx, namespace, istioConfig, 0)
}

// isTargetRoute returns true if the route name is in the target list.
func isTargetRoute(name string, targets []string) bool {
	for _, t := range targets {
		if t == name {
			return true
		}
	}
	return false
}

// patchRouteWeights updates the weight fields on destinations matching the
// stable and candidate subsets within a single HTTP route.
func patchRouteWeights(route map[string]interface{}, stableSubset, candidateSubset string, stableWeight, candidateWeight int64) {
	rawDests, ok := route["route"]
	if !ok {
		return
	}
	dests, ok := rawDests.([]interface{})
	if !ok {
		return
	}
	for j, rawDest := range dests {
		dest, ok := rawDest.(map[string]interface{})
		if !ok {
			continue
		}
		subset, _, _ := unstructured.NestedString(dest, "destination", "subset")
		switch subset {
		case stableSubset:
			dest["weight"] = stableWeight
		case candidateSubset:
			dest["weight"] = candidateWeight
		default:
			continue
		}
		dests[j] = dest
	}
	route["route"] = dests
}
