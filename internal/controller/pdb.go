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

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// reconcilePDB creates or updates a PodDisruptionBudget for agent pods.
// PDB is only created when replicas > 1 (single-replica PDB is meaningless).
// When replicas <= 1, any existing PDB is cleaned up.
func (r *AgentRuntimeReconciler) reconcilePDB(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Replicas != nil {
		replicas = *agentRuntime.Spec.Runtime.Replicas
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	// Clean up PDB when replicas <= 1
	if replicas <= 1 {
		if err := r.Delete(ctx, pdb); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete PDB: %w", err)
		}
		return nil
	}

	labels := map[string]string{
		labelAppName:      labelValueOmniaAgent,
		labelAppInstance:  agentRuntime.Name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    "agent",
	}

	minAvailable := intstr.FromInt32(1)

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, pdb, func() error {
		if err := controllerutil.SetControllerReference(agentRuntime, pdb, r.Scheme); err != nil {
			return err
		}

		pdb.Labels = labels
		pdb.Spec = policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile PDB: %w", err)
	}

	log.Info("PDB reconciled", "result", result)
	return nil
}
