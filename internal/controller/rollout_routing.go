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
)

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
	mapper := r.Client.RESTMapper()
	_, err := mapper.RESTMapping(schema.GroupKind{Group: "networking.istio.io", Kind: istioVirtualServiceKind})
	return err == nil
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
