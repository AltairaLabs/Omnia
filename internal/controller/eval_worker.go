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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// reconcileEvalWorker ensures a per-namespace eval worker Deployment exists when
// any non-PromptKit AgentRuntime in the namespace has evals enabled.
// When the last such agent is deleted or disabled, the worker is cleaned up.
func (r *AgentRuntimeReconciler) reconcileEvalWorker(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	namespace := agentRuntime.Namespace

	needed, err := r.namespaceNeedsEvalWorker(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to check eval worker need: %w", err)
	}

	if needed {
		return r.ensureEvalWorkerDeployment(ctx, namespace)
	}

	log.V(1).Info("no non-PromptKit eval-enabled agents in namespace, cleaning up eval worker",
		"namespace", namespace)
	return r.deleteEvalWorkerDeployment(ctx, namespace)
}

// namespaceNeedsEvalWorker checks if any non-PromptKit AgentRuntime in the
// namespace has evals enabled.
func (r *AgentRuntimeReconciler) namespaceNeedsEvalWorker(
	ctx context.Context,
	namespace string,
) (bool, error) {
	list := &omniav1alpha1.AgentRuntimeList{}
	if err := r.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return false, fmt.Errorf("failed to list AgentRuntimes: %w", err)
	}

	for i := range list.Items {
		rt := &list.Items[i]
		if rt.DeletionTimestamp != nil {
			continue
		}
		if hasEvalsEnabled(&rt.Spec) && !isPromptKit(&rt.Spec) {
			return true, nil
		}
	}

	return false, nil
}

// ensureEvalWorkerDeployment creates or updates the eval worker Deployment.
func (r *AgentRuntimeReconciler) ensureEvalWorkerDeployment(
	ctx context.Context,
	namespace string,
) error {
	log := logf.FromContext(ctx)

	desired := r.buildEvalWorkerDeployment(namespace)

	existing := &appsv1.Deployment{}
	key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
	err := r.Get(ctx, key, existing)

	if apierrors.IsNotFound(err) {
		log.Info("creating eval worker Deployment", "namespace", namespace)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("failed to get eval worker Deployment: %w", err)
	}

	// Update the existing deployment spec
	existing.Labels = desired.Labels
	existing.Spec = desired.Spec
	log.V(1).Info("updating eval worker Deployment", "namespace", namespace)
	return r.Update(ctx, existing)
}

// deleteEvalWorkerDeployment removes the eval worker Deployment if it exists.
func (r *AgentRuntimeReconciler) deleteEvalWorkerDeployment(
	ctx context.Context,
	namespace string,
) error {
	existing := &appsv1.Deployment{}
	key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
	err := r.Get(ctx, key, existing)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get eval worker Deployment for deletion: %w", err)
	}

	// Only delete if it has our managed-by label
	if existing.Labels[labelAppManagedBy] != labelValueOmniaOperator {
		return nil
	}

	return r.Delete(ctx, existing)
}

// buildEvalWorkerDeployment constructs the desired eval worker Deployment.
func (r *AgentRuntimeReconciler) buildEvalWorkerDeployment(namespace string) *appsv1.Deployment {
	labels := map[string]string{
		labelAppName:      labelValueEvalWorker,
		labelAppInstance:  EvalWorkerDeploymentName,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    labelEvalWorkerComp,
	}

	image := r.evalWorkerImage()
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      EvalWorkerDeploymentName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  EvalWorkerContainerName,
							Image: image,
							Env:   r.buildEvalWorkerEnvVars(namespace),
						},
					},
				},
			},
		},
	}
}

// buildEvalWorkerEnvVars creates environment variables for the eval worker container.
func (r *AgentRuntimeReconciler) buildEvalWorkerEnvVars(namespace string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{Name: envNamespace, Value: namespace},
	}

	if r.RedisAddr != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  envRedisAddr,
			Value: r.RedisAddr,
		})
	}

	if r.SessionAPIURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  envSessionAPIURL,
			Value: r.SessionAPIURL,
		})
	}

	return envVars
}

// evalWorkerImage returns the image to use for the eval worker container.
func (r *AgentRuntimeReconciler) evalWorkerImage() string {
	if r.EvalWorkerImage != "" {
		return r.EvalWorkerImage
	}
	return DefaultEvalWorkerImage
}
