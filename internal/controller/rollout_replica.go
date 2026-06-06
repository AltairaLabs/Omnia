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
	"math"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// replicaSplit computes candidate/stable replica counts approximating
// candidateWeight against a fixed total, plus the delivered weight that ratio
// actually yields. total<2 degrades to binary (0/100).
func replicaSplit(total, candidateWeight int32) (candidate, stable, delivered int32) {
	if total < 2 {
		if candidateWeight >= 50 {
			return total, 0, 100
		}
		return 0, total, 0
	}
	candidate = int32(math.Round(float64(total) * float64(candidateWeight) / 100.0))
	if candidate > total {
		candidate = total
	}
	stable = total - candidate
	delivered = int32(math.Round(float64(candidate) / float64(total) * 100.0))
	return candidate, stable, delivered
}

// canonicalReplicaTotal returns the AgentRuntime's desired stable replica count
// — the canonical source of truth used by the deployment builder. This is
// stable across reconciles (the controller mutates the live Deployment's
// .Spec.Replicas during weighting, but never the spec), so deriving `total`
// from it avoids the downward drift that re-reading the live Deployment causes.
// Defaults to 1 to match the deployment builder and CRD default.
func canonicalReplicaTotal(ar *omniav1alpha1.AgentRuntime) int32 {
	if ar.Spec.Runtime != nil && ar.Spec.Runtime.Replicas != nil && *ar.Spec.Runtime.Replicas > 0 {
		return *ar.Spec.Runtime.Replicas
	}
	return 1
}

// reconcileReplicaWeighting scales the stable + candidate Deployments to
// approximate candidateWeight, returns the delivered weight, and logs when the
// delivered weight differs from the request (granularity loss).
func (r *AgentRuntimeReconciler) reconcileReplicaWeighting(ctx context.Context, ar *omniav1alpha1.AgentRuntime, candidateWeight int32) (int32, error) {
	// Derive the canonical total from the AgentRuntime spec (the same source the
	// deployment builder stamps onto the stable Deployment), NOT the live stable
	// Deployment's .Spec.Replicas — the controller itself shrinks that field as
	// it shifts weight, so re-reading it across reconciles would let `total`
	// drift downward (e.g. 4→2→1) and permanently shrink stable.
	total := canonicalReplicaTotal(ar)

	candReplicas, stableReplicas, delivered := replicaSplit(total, candidateWeight)

	candName := candidateDeploymentName(ar.Name)
	if err := r.scaleDeployment(ctx, ar.Namespace, candName, candReplicas); err != nil {
		return 0, err
	}
	if err := r.scaleDeployment(ctx, ar.Namespace, ar.Name, stableReplicas); err != nil {
		return 0, err
	}

	if delivered != candidateWeight {
		logf.FromContext(ctx).Info("rollout weight approximated",
			"agentRuntime", ar.Name,
			"requestedWeight", candidateWeight,
			"deliveredWeight", delivered,
			"totalReplicas", total,
			"reason", "replica_granularity")
	}
	return delivered, nil
}

// scaleDeployment sets replicas on a Deployment (no-op when already at target).
func (r *AgentRuntimeReconciler) scaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // candidate may not exist yet on weight 0
		}
		return fmt.Errorf("get deployment %q: %w", name, err)
	}
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas == replicas {
		return nil
	}
	dep.Spec.Replicas = ptr.To(replicas)
	if err := r.Update(ctx, dep); err != nil {
		return fmt.Errorf("scale deployment %q to %d: %w", name, replicas, err)
	}
	return nil
}
