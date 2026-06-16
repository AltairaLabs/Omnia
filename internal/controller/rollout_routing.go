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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Traffic routing modes (CRD enum values + resolved-mode strings).
// ConditionTypeTrafficRouting is declared with the other condition types in
// constants.go (Task 2) — reference that single source; do not redeclare here.
const (
	TrafficModeMesh            = "mesh"
	TrafficModeReplicaWeighted = "replicaWeighted"
	TrafficModeExternal        = "external"

	// istioNetworkingGroup is the bare API group for Istio networking CRDs
	// (distinct from istioNetworkingAPIVersion which carries the version).
	istioNetworkingGroup = "networking.istio.io"

	// labelIstioUseWaypoint enrolls a Service in an Istio ambient waypoint. The
	// operator stamps it on the agent Service in mesh mode so the operator-owned
	// VirtualService's L7 routing takes effect (ztunnel alone is L4-only).
	labelIstioUseWaypoint = "istio.io/use-waypoint"
)

// meshWaypointFor returns the ambient waypoint name to enroll the agent Service
// in, or "" when the agent does not resolve to mesh routing or no waypoint is
// configured. Without this enrollment the operator-owned VirtualService is
// silently bypassed (ambient ztunnel is L4-only), so the weighted stable/
// candidate split — and the x-omnia-variant header injection it carries — never
// applies.
func (r *AgentRuntimeReconciler) meshWaypointFor(ctx context.Context, ar *omniav1alpha1.AgentRuntime) string {
	return meshWaypointForResolved(ar, r.meshAvailable(ctx))
}

// meshWaypointForResolved is the pure resolution (testable without a client):
// the configured waypoint name when the agent resolves to mesh routing, else "".
func meshWaypointForResolved(ar *omniav1alpha1.AgentRuntime, meshAvailable bool) string {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.TrafficRouting == nil || ar.Spec.Rollout.TrafficRouting.Mesh == nil {
		return ""
	}
	waypoint := ar.Spec.Rollout.TrafficRouting.Mesh.Waypoint
	if waypoint == "" {
		return ""
	}
	if mode, _ := resolveTrafficModeFor(ar.Spec.Rollout.TrafficRouting, meshAvailable); mode != TrafficModeMesh {
		return ""
	}
	return waypoint
}

// resolveTrafficModeFor is the pure resolution rule (testable without a client).
// Returns the resolved mode and whether it degraded from an explicit request.
func resolveTrafficModeFor(cfg *omniav1alpha1.TrafficRoutingConfig, meshAvailable bool) (string, bool) {
	if cfg == nil {
		if meshAvailable {
			return TrafficModeMesh, false
		}
		return TrafficModeReplicaWeighted, false
	}
	switch cfg.Mode {
	case TrafficModeMesh:
		if !meshAvailable {
			return TrafficModeReplicaWeighted, true // degraded
		}
		return TrafficModeMesh, false
	case TrafficModeReplicaWeighted, TrafficModeExternal:
		return cfg.Mode, false
	default: // unset
		if cfg.Istio != nil {
			return TrafficModeExternal, false // legacy reference form
		}
		if meshAvailable {
			return TrafficModeMesh, false
		}
		return TrafficModeReplicaWeighted, false
	}
}

// meshAvailable reports whether mode=mesh can actually work: the chart flag is
// on AND the Istio routing CRDs are served by the API server.
func (r *AgentRuntimeReconciler) meshAvailable(_ context.Context) bool {
	if !r.MeshEnabled {
		return false
	}
	mapper := r.RESTMapper()
	_, err := mapper.RESTMapping(schema.GroupKind{Group: istioNetworkingGroup, Kind: istioVirtualServiceKind})
	return err == nil
}

// isReplicaWeightedActive is the pure predicate: an active rollout whose
// resolved traffic mode is replica-weighted. When true, reconcileReplicaWeighting
// owns the stable + candidate .spec.replicas (it splits the canonical total
// across the two Deployments), so the deployment builders must NOT re-stamp the
// canonical total each reconcile or they collapse the split back to 1:1.
func isReplicaWeightedActive(ar *omniav1alpha1.AgentRuntime, meshAvailable bool) bool {
	if !isRolloutActive(ar) || ar.Spec.Rollout.TrafficRouting == nil {
		return false
	}
	mode, _ := resolveTrafficModeFor(ar.Spec.Rollout.TrafficRouting, meshAvailable)
	return mode == TrafficModeReplicaWeighted
}

// replicaWeightingActive is the client-aware wrapper around isReplicaWeightedActive.
// It uses the side-effect-free mode resolver so callers in the deployment-build
// path don't emit duplicate degrade observations.
func (r *AgentRuntimeReconciler) replicaWeightingActive(ctx context.Context, ar *omniav1alpha1.AgentRuntime) bool {
	return isReplicaWeightedActive(ar, r.meshAvailable(ctx))
}

// resolveTrafficMode resolves the effective mode and, when it degraded from an
// explicit request, emits the structured-log + condition + Event observability
// trio (spec §7).
func (r *AgentRuntimeReconciler) resolveTrafficMode(ctx context.Context, ar *omniav1alpha1.AgentRuntime) string {
	cfg := ar.Spec.Rollout.TrafficRouting
	mode, degraded := resolveTrafficModeFor(cfg, r.meshAvailable(ctx))
	if degraded {
		r.observeTrafficDegrade(ctx, ar, cfg.Mode, mode, "mesh_unavailable",
			"requested mesh routing but the Istio mesh is unavailable; using replica-weighted")
	}
	return mode
}

// observeTrafficDegrade emits log + condition + Event for a routing
// degrade/approximation. reason is a snake_case key; message is human text.
func (r *AgentRuntimeReconciler) observeTrafficDegrade(
	ctx context.Context, ar *omniav1alpha1.AgentRuntime,
	requestedMode, resolvedMode, reason, message string,
) {
	logf.FromContext(ctx).Info("rollout traffic routing degraded",
		"agentRuntime", ar.Name,
		"requestedMode", requestedMode,
		"resolvedMode", resolvedMode,
		"reason", reason)
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeTrafficRouting, metav1.ConditionFalse, "Degraded", message)
	if r.Recorder != nil {
		r.Recorder.Event(ar, corev1.EventTypeWarning, "TrafficRoutingDegraded", message)
	}
}

// setTrafficStatus records the resolved mode + enforcement on status and the
// delivered weight on the metric. enforced=false also drives a (non-degrade)
// informational condition noting the weight is approximate.
func (r *AgentRuntimeReconciler) setTrafficStatus(ar *omniav1alpha1.AgentRuntime, mode string, deliveredWeight int32, enforced bool) {
	if ar.Status.Rollout == nil {
		ar.Status.Rollout = &omniav1alpha1.RolloutStatus{}
	}
	ar.Status.Rollout.TrafficRoutingMode = mode
	e := enforced
	ar.Status.Rollout.TrafficWeightEnforced = &e
	if r.RolloutMetrics != nil {
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(float64(100 - deliveredWeight))
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(float64(deliveredWeight))
	}
}

// hasIstioConfig reports whether the legacy reference-form Istio config is set.
func hasIstioConfig(ar *omniav1alpha1.AgentRuntime) bool {
	return ar.Spec.Rollout != nil &&
		ar.Spec.Rollout.TrafficRouting != nil &&
		ar.Spec.Rollout.TrafficRouting.Istio != nil
}

// applyTrafficRouting resolves the mode and delivers desiredWeight via the
// chosen mechanism, then records delivered weight + enforcement on status/metric.
// Skips application while paused or in an analysis window (caller passes those).
func (r *AgentRuntimeReconciler) applyTrafficRouting(ctx context.Context, ar *omniav1alpha1.AgentRuntime, desiredWeight int32) error {
	mode := r.resolveTrafficMode(ctx, ar)
	switch mode {
	case TrafficModeMesh:
		if err := r.reconcileMeshRouting(ctx, ar, desiredWeight); err != nil {
			return err
		}
		r.setTrafficStatus(ar, mode, desiredWeight, true)
	case TrafficModeExternal:
		if hasIstioConfig(ar) {
			if err := r.patchVirtualServiceWeights(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio, desiredWeight); err != nil {
				// A genuinely-missing referenced VirtualService (CRDs present,
				// object NotFound) is a degrade, not a hard failure (spec §7/§11):
				// emit the log+condition+Event trio and continue. Other errors
				// (read/update conflicts, missing routes) still propagate.
				if apierrors.IsNotFound(err) {
					r.observeTrafficDegrade(ctx, ar, mode, mode, "ReferencedObjectsMissing",
						"referenced VirtualService not found; traffic weights not applied")
					r.setTrafficStatus(ar, mode, desiredWeight, false)
					return nil
				}
				return err
			}
		}
		r.setTrafficStatus(ar, mode, desiredWeight, true)
	default: // replicaWeighted
		delivered, err := r.reconcileReplicaWeighting(ctx, ar, desiredWeight)
		if err != nil {
			return err
		}
		r.setTrafficStatus(ar, mode, delivered, false)
	}
	return nil
}

// resetTrafficRoutingForMode resets to 100% stable across whichever mode is
// active (used on promote / rollback).
func (r *AgentRuntimeReconciler) resetTrafficRoutingForMode(ctx context.Context, ar *omniav1alpha1.AgentRuntime) error {
	mode := r.resolveTrafficMode(ctx, ar)
	switch mode {
	case TrafficModeMesh:
		return r.reconcileMeshRouting(ctx, ar, 0)
	case TrafficModeExternal:
		if hasIstioConfig(ar) {
			return r.resetTrafficRouting(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio)
		}
	default:
		_, err := r.reconcileReplicaWeighting(ctx, ar, 0)
		return err
	}
	return nil
}
