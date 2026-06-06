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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	trackLabelKey = "track"
	trackStable   = "stable"
	trackCanary   = "canary"

	// Istio resource spec field keys reused when building unstructured objects.
	fieldName        = "name"
	fieldHost        = "host"
	fieldSubset      = "subset"
	fieldLabels      = "labels"
	fieldRoute       = "route"
	fieldDestination = "destination"
	fieldWeight      = "weight"
)

// meshRoutingName is the shared name for the operator-owned VS + DR of an agent.
func meshRoutingName(agentName string) string { return agentName + "-rollout" }

// agentServiceDNS is the default host the VS matches when mesh.Hosts is empty.
func agentServiceDNS(agentName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", agentName, namespace)
}

// buildOwnedDestinationRule builds an unstructured DestinationRule defining the
// stable/canary subsets keyed on the operator's own track label.
func buildOwnedDestinationRule(agentName, namespace, host string, mesh *omniav1alpha1.MeshTrafficRouting) *unstructured.Unstructured {
	dr := &unstructured.Unstructured{}
	dr.SetAPIVersion(istioNetworkingAPIVersion)
	dr.SetKind(istioDestinationRuleKind)
	dr.SetName(meshRoutingName(agentName))
	dr.SetNamespace(namespace)
	_ = unstructured.SetNestedField(dr.Object, host, "spec", fieldHost)
	_ = unstructured.SetNestedSlice(dr.Object, []interface{}{
		map[string]interface{}{fieldName: mesh.StableSubset, fieldLabels: map[string]interface{}{trackLabelKey: trackStable}},
		map[string]interface{}{fieldName: mesh.CandidateSubset, fieldLabels: map[string]interface{}{trackLabelKey: trackCanary}},
	}, "spec", "subsets")
	return dr
}

// buildOwnedVirtualService builds an unstructured VirtualService weighting the
// stable/canary subsets. candidateWeight is the canary percentage (0..100).
func buildOwnedVirtualService(agentName, namespace string, hosts []string, mesh *omniav1alpha1.MeshTrafficRouting, candidateWeight int32) *unstructured.Unstructured {
	host := agentServiceDNS(agentName, namespace)
	hostIfaces := make([]interface{}, 0, len(hosts))
	for _, h := range hosts {
		hostIfaces = append(hostIfaces, h)
	}
	vs := &unstructured.Unstructured{}
	vs.SetAPIVersion(istioNetworkingAPIVersion)
	vs.SetKind(istioVirtualServiceKind)
	vs.SetName(meshRoutingName(agentName))
	vs.SetNamespace(namespace)
	_ = unstructured.SetNestedSlice(vs.Object, hostIfaces, "spec", "hosts")
	_ = unstructured.SetNestedSlice(vs.Object, []interface{}{
		map[string]interface{}{
			fieldName: "rollout",
			fieldRoute: []interface{}{
				map[string]interface{}{
					fieldDestination: map[string]interface{}{fieldHost: host, fieldSubset: mesh.StableSubset},
					fieldWeight:      int64(100 - candidateWeight),
				},
				map[string]interface{}{
					fieldDestination: map[string]interface{}{fieldHost: host, fieldSubset: mesh.CandidateSubset},
					fieldWeight:      int64(candidateWeight),
				},
			},
		},
	}, "spec", "http")
	return vs
}

// reconcileMeshRouting creates/updates the operator-owned VS + DR and sets the
// weights for candidateWeight. Owner-ref ties them to the AgentRuntime for GC.
func (r *AgentRuntimeReconciler) reconcileMeshRouting(ctx context.Context, ar *omniav1alpha1.AgentRuntime, candidateWeight int32) error {
	mesh := ar.Spec.Rollout.TrafficRouting.Mesh
	if mesh == nil {
		mesh = &omniav1alpha1.MeshTrafficRouting{StableSubset: trackStable, CandidateSubset: trackCanary}
	}
	hosts := mesh.Hosts
	if len(hosts) == 0 {
		hosts = []string{agentServiceDNS(ar.Name, ar.Namespace)}
	}
	dr := buildOwnedDestinationRule(ar.Name, ar.Namespace, agentServiceDNS(ar.Name, ar.Namespace), mesh)
	vs := buildOwnedVirtualService(ar.Name, ar.Namespace, hosts, mesh, candidateWeight)
	for _, obj := range []*unstructured.Unstructured{dr, vs} {
		if err := controllerutil.SetControllerReference(ar, obj, r.Scheme); err != nil {
			return fmt.Errorf("set owner ref on %s %q: %w", obj.GetKind(), obj.GetName(), err)
		}
		if err := r.upsertUnstructured(ctx, obj); err != nil {
			return err
		}
	}
	logf.FromContext(ctx).V(1).Info("reconciled owned mesh routing",
		"agentRuntime", ar.Name, "candidateWeight", candidateWeight)
	return nil
}

// upsertUnstructured creates obj or, if it exists, overwrites its spec (the
// operator owns these objects; user edits are not preserved).
func (r *AgentRuntimeReconciler) upsertUnstructured(ctx context.Context, obj *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(obj.GetAPIVersion())
	existing.SetKind(obj.GetKind())
	err := r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if err != nil {
		if isNoMatchError(err) {
			return nil // CRDs absent — caller should not have routed here, but no-op safely
		}
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get %s %q: %w", obj.GetKind(), obj.GetName(), err)
		}
		if err := r.Create(ctx, obj); err != nil {
			return fmt.Errorf("create %s %q: %w", obj.GetKind(), obj.GetName(), err)
		}
		return nil
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	if err := r.Update(ctx, obj); err != nil {
		return fmt.Errorf("update %s %q: %w", obj.GetKind(), obj.GetName(), err)
	}
	return nil
}
