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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// promotePollInterval is how often the controller re-checks whether the stable
// Deployment has finished rolling to the promoted config while the candidate
// keeps serving.
const promotePollInterval = 5 * time.Second

// reconcileRolloutPromote drives a zero-downtime promotion as a small state
// machine across reconciles:
//
//  1. enter   — advance spec to the candidate config (so the stable Deployment
//     starts rolling to it in the background) while keeping 100% of traffic on
//     the warm, validated candidate; mark status.promoting.
//  2. wait    — each reconcile, hold traffic on the candidate until the stable
//     Deployment is fully rolled out and healthy on the new config.
//  3. finish  — cut traffic back to stable and delete the candidate.
//
// No request is ever served from a cold/restarting stable pod, because the
// cutover only happens once stable is healthy on the new config.
func (r *AgentRuntimeReconciler) reconcileRolloutPromote(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	if ar.Status.Rollout != nil && ar.Status.Rollout.Promoting {
		return r.advanceOrFinishPromotion(ctx, ar)
	}
	return r.enterPromotion(ctx, ar)
}

// enterPromotion advances spec to the candidate config and routes all traffic
// to the candidate, then marks the rollout as promoting and requeues. The
// stable Deployment is re-rendered to the new config by the normal
// reconcileDeployment path (spec changed) and rolls in the background.
func (r *AgentRuntimeReconciler) enterPromotion(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	promote(ar)
	// Persist the spec advance before the status update (separate API calls).
	if err := r.Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist promotion spec: %w", err)
	}

	if err := r.routeToCandidate(ctx, ar); err != nil {
		return ctrl.Result{}, err
	}
	if r.RolloutMetrics != nil {
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(0)
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(100)
	}

	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:    true,
		Promoting: true,
		Message:   "promoting: waiting for stable to roll to the new config",
	}
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionTrue,
		"Promoting", "promotion in progress: stable rolling to new config, candidate still serving")
	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist promotion status: %w", err)
	}

	r.recordRolloutNormal(ar, eventReasonPromoting, "promotion started: stable rolling to the new config, candidate still serving 100%")
	log.Info("rollout promotion started", "agentRuntime", ar.Name)
	return ctrl.Result{RequeueAfter: promotePollInterval}, nil
}

// advanceOrFinishPromotion checks whether the stable Deployment has finished
// rolling to the promoted config. While it has not, traffic is held on the
// candidate and the reconcile requeues; once it has, promotion finishes.
func (r *AgentRuntimeReconciler) advanceOrFinishPromotion(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	stable := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: ar.Name, Namespace: ar.Namespace}, stable); err != nil {
		return ctrl.Result{}, fmt.Errorf("get stable deployment for promotion: %w", err)
	}

	if !deploymentRolloutComplete(stable) {
		// Keep serving from the warm candidate while stable rolls.
		if err := r.routeToCandidate(ctx, ar); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: promotePollInterval}, nil
	}

	return r.finishPromotion(ctx, ar)
}

// finishPromotion cuts traffic back to the (now healthy, new-config) stable
// Deployment and deletes the candidate. This is the only point at which traffic
// leaves the candidate, so the cutover is to warm stable pods.
func (r *AgentRuntimeReconciler) finishPromotion(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil {
		if err := r.resetTrafficRoutingForMode(ctx, ar); err != nil {
			log.Error(err, "failed to reset traffic routing on promotion finish")
		}
		if ar.Spec.Rollout.TrafficRouting.Istio != nil {
			if err := r.patchDestinationRuleConsistentHash(ctx, ar.Namespace,
				ar.Spec.Rollout.TrafficRouting.Istio, ""); err != nil {
				log.Error(err, "failed to remove consistent hash on promotion finish")
			}
		}
	}

	if err := r.deleteCandidateDeployment(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete candidate after promotion: %w", err)
	}

	if r.RolloutMetrics != nil {
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(100)
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(0)
	}

	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: false, Message: "promoted"}
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionFalse,
		"NoActiveRollout", "rollout promoted successfully")
	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist promotion-finished status: %w", err)
	}

	r.recordRolloutNormal(ar, eventReasonPromoted, "promotion complete: stable healthy on the new config, traffic cut over, candidate removed")
	log.Info("rollout promoted", "agentRuntime", ar.Name)
	return ctrl.Result{}, nil
}

// routeToCandidate sends 100% of traffic to the candidate while stable rolls.
//   - mesh / external: weight the candidate subset to 100%.
//   - replicaWeighted: a no-op. The candidate keeps its replicas and serves;
//     the stable Deployment rolls to its canonical replica count via
//     reconcileDeployment (replica-weighting is no longer active post-promote).
//     Scaling stable to 0 here would stop it rolling, and the Service routes
//     only to ready pods, so traffic stays on the candidate until stable's new
//     pods are ready.
func (r *AgentRuntimeReconciler) routeToCandidate(ctx context.Context, ar *omniav1alpha1.AgentRuntime) error {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.TrafficRouting == nil {
		return nil
	}
	switch r.resolveTrafficMode(ctx, ar) {
	case TrafficModeMesh:
		return r.reconcileMeshRouting(ctx, ar, 100)
	case TrafficModeExternal:
		if hasIstioConfig(ar) {
			return r.patchVirtualServiceWeights(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio, 100)
		}
		return nil
	default: // replicaWeighted — see doc comment.
		return nil
	}
}

// deploymentRolloutComplete reports whether a Deployment has fully rolled out to
// its current pod template: the controller has observed the latest spec, and
// every replica is updated and available with no stale replicas remaining. This
// is the standard "kubectl rollout status" completion check.
func deploymentRolloutComplete(d *appsv1.Deployment) bool {
	if d.Status.ObservedGeneration < d.Generation {
		return false
	}
	want := int32(1)
	if d.Spec.Replicas != nil {
		want = *d.Spec.Replicas
	}
	return d.Status.UpdatedReplicas == want &&
		d.Status.AvailableReplicas == want &&
		d.Status.Replicas == want
}
