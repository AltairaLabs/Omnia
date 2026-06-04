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
	"github.com/altairalabs/omnia/internal/podoverrides"
)

// defaultSvcGroupName is the service-group name used when an AgentRuntime does
// not set spec.serviceGroup.
const defaultSvcGroupName = "default"

// evalWorkerName returns the per-service-group eval-worker Deployment name.
func evalWorkerName(serviceGroup string) string {
	return fmt.Sprintf("%s-%s", labelValueEvalWorker, serviceGroup)
}

// reconcileEvalWorker converges the full set of per-service-group eval worker
// Deployments for the agent's namespace. One arena-eval-worker-<group> exists
// per service group that has a non-PromptKit, eval-enabled AgentRuntime. Workers
// for groups that no longer need one (including the legacy un-suffixed singleton)
// are removed. The reconcile is idempotent regardless of which agent triggered it.
func (r *AgentRuntimeReconciler) reconcileEvalWorker(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	namespace := agentRuntime.Namespace

	needed, err := r.serviceGroupsNeedingEvalWorker(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to check eval worker need: %w", err)
	}

	for group, podOverrides := range needed {
		if err := r.ensureEvalWorkerDeployment(ctx, namespace, group, podOverrides); err != nil {
			return err
		}
	}

	return r.cleanupEvalWorkers(ctx, namespace, needed)
}

// serviceGroupsNeedingEvalWorker returns the set of service groups in the
// namespace that have at least one non-PromptKit, eval-enabled AgentRuntime.
// The value is the representative PodOverrides for the group (the first agent
// seen, may be nil).
func (r *AgentRuntimeReconciler) serviceGroupsNeedingEvalWorker(
	ctx context.Context,
	namespace string,
) (map[string]*omniav1alpha1.PodOverrides, error) {
	list := &omniav1alpha1.AgentRuntimeList{}
	if err := r.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AgentRuntimes: %w", err)
	}

	needed := map[string]*omniav1alpha1.PodOverrides{}
	for i := range list.Items {
		rt := &list.Items[i]
		if rt.DeletionTimestamp != nil {
			continue
		}
		if !hasEvalsEnabled(&rt.Spec) || isPromptKit(&rt.Spec) {
			continue
		}
		group := rt.Spec.ServiceGroup
		if group == "" {
			group = defaultSvcGroupName
		}
		if _, seen := needed[group]; !seen {
			needed[group] = rt.Spec.Evals.PodOverrides
		}
	}

	return needed, nil
}

// ensureEvalWorkerDeployment creates or updates the eval worker Deployment for a
// single service group.
func (r *AgentRuntimeReconciler) ensureEvalWorkerDeployment(
	ctx context.Context,
	namespace, serviceGroup string,
	podOverrides *omniav1alpha1.PodOverrides,
) error {
	log := logf.FromContext(ctx)

	desired := r.buildEvalWorkerDeployment(ctx, namespace, serviceGroup, podOverrides)

	existing := &appsv1.Deployment{}
	key := types.NamespacedName{Name: evalWorkerName(serviceGroup), Namespace: namespace}
	err := r.Get(ctx, key, existing)

	if apierrors.IsNotFound(err) {
		log.Info("creating eval worker Deployment", "namespace", namespace, "serviceGroup", serviceGroup)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("failed to get eval worker Deployment: %w", err)
	}

	// Update the existing deployment spec
	existing.Labels = desired.Labels
	existing.Spec = desired.Spec
	log.V(1).Info("updating eval worker Deployment", "namespace", namespace, "serviceGroup", serviceGroup)
	return r.Update(ctx, existing)
}

// cleanupEvalWorkers deletes operator-managed eval worker Deployments whose
// service group is no longer in the needed set. The legacy un-suffixed singleton
// has an empty service-group label, which is never a key in needed, so it is
// removed here.
func (r *AgentRuntimeReconciler) cleanupEvalWorkers(
	ctx context.Context,
	namespace string,
	needed map[string]*omniav1alpha1.PodOverrides,
) error {
	log := logf.FromContext(ctx)

	list := &appsv1.DeploymentList{}
	if err := r.List(ctx, list,
		client.InNamespace(namespace),
		client.MatchingLabels{
			labelAppName:      labelValueEvalWorker,
			labelAppManagedBy: labelValueOmniaOperator,
		},
	); err != nil {
		return fmt.Errorf("failed to list eval worker Deployments: %w", err)
	}

	for i := range list.Items {
		dep := &list.Items[i]
		group := dep.Labels[labelServiceGroup]
		if _, ok := needed[group]; ok {
			continue
		}
		log.V(1).Info("deleting stale eval worker Deployment",
			"namespace", namespace, "name", dep.Name, "serviceGroup", group)
		if err := r.Delete(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete eval worker Deployment %s: %w", dep.Name, err)
		}
	}

	return nil
}

// buildEvalWorkerDeployment constructs the desired eval worker Deployment for a
// single service group.
func (r *AgentRuntimeReconciler) buildEvalWorkerDeployment(
	ctx context.Context,
	namespace, serviceGroup string,
	podOverrides *omniav1alpha1.PodOverrides,
) *appsv1.Deployment {
	name := evalWorkerName(serviceGroup)
	labels := map[string]string{
		labelAppName:      labelValueEvalWorker,
		labelAppInstance:  name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    labelEvalWorkerComp,
		labelServiceGroup: serviceGroup,
	}

	image := r.evalWorkerImage()
	replicas := int32(1)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
							Name:            EvalWorkerContainerName,
							Image:           image,
							ImagePullPolicy: r.EvalWorkerImagePullPolicy,
							Env:             r.buildEvalWorkerEnvVars(ctx, namespace, serviceGroup),
						},
					},
				},
			},
		},
	}

	if podOverrides != nil {
		podoverrides.ApplyPod(&dep.Spec.Template.Spec, &dep.Spec.Template.ObjectMeta, podOverrides)
		for i := range dep.Spec.Template.Spec.Containers {
			podoverrides.ApplyContainer(&dep.Spec.Template.Spec.Containers[i], podOverrides)
		}
	}

	return dep
}

// buildEvalWorkerEnvVars creates environment variables for the eval worker container.
//
// REDIS_URL is resolved the same way the group's session-api resolves it
// (per-component session.redis > group redis > operator session default), so
// the worker consumes from the same stream session-api publishes eval events
// to. The eval-worker binary reads REDIS_URL directly from its environment, so
// the existingSecret form must be a ValueFrom-only entry (a Kubernetes EnvVar
// cannot set both Value and ValueFrom).
func (r *AgentRuntimeReconciler) buildEvalWorkerEnvVars(ctx context.Context, namespace, serviceGroup string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{Name: envNamespace, Value: namespace},
		{Name: envServiceGroup, Value: serviceGroup},
	}

	redisURL, redisSecret := r.resolveEvalWorkerRedis(ctx, namespace, serviceGroup)
	switch {
	case redisSecret.Name != "" && redisSecret.Key != "":
		envVars = append(envVars, corev1.EnvVar{
			Name: envRedisURL,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: redisSecret.Name},
				Key:                  redisSecret.Key,
			}},
		})
	case redisURL != "":
		envVars = append(envVars, corev1.EnvVar{Name: envRedisURL, Value: redisURL})
	}

	if sessionURL := r.resolveSessionURLForWorkspace(ctx, namespace, serviceGroup); sessionURL != "" {
		envVars = append(envVars, corev1.EnvVar{Name: envSessionAPIURL, Value: sessionURL})
	}

	return envVars
}

// evalWorkerRedisDefault returns the operator-wide default (url, secret) pair
// for an eval-worker's redis: the session redis default when set, otherwise
// the operator-wide RedisURL with no secret.
func (r *AgentRuntimeReconciler) evalWorkerRedisDefault() (string, SecretKeyRef) {
	if r.SessionRedisURL != "" {
		return r.SessionRedisURL, r.SessionRedisURLSecret
	}
	return r.RedisURL, SecretKeyRef{}
}

// resolveEvalWorkerRedis resolves the eval-worker's REDIS_URL with the same
// precedence session-api uses: per-component session.redis > group redis >
// operator session default. When the group's Workspace cannot be found, the
// operator default pair is returned.
func (r *AgentRuntimeReconciler) resolveEvalWorkerRedis(ctx context.Context, namespace, serviceGroup string) (string, SecretKeyRef) {
	defaultURL, defaultSecret := r.evalWorkerRedisDefault()
	if sg, ok := r.findServiceGroup(ctx, namespace, serviceGroup); ok {
		return resolveSessionRedis(sg, namespace, defaultURL, defaultSecret)
	}
	return defaultURL, defaultSecret
}

// findServiceGroup looks up the WorkspaceServiceGroup spec for the given
// namespace + service group from the Workspace CRDs. Returns the zero value and
// false when no matching Workspace/group exists (including on a list error).
func (r *AgentRuntimeReconciler) findServiceGroup(ctx context.Context, namespace, serviceGroup string) (omniav1alpha1.WorkspaceServiceGroup, bool) {
	log := logf.FromContext(ctx)
	var list omniav1alpha1.WorkspaceList
	if err := r.List(ctx, &list); err != nil {
		log.V(1).Info("eval worker redis: workspace list failed, using operator default",
			"namespace", namespace, "serviceGroup", serviceGroup)
		return omniav1alpha1.WorkspaceServiceGroup{}, false
	}
	for i := range list.Items {
		ws := &list.Items[i]
		if ws.Spec.Namespace.Name != namespace {
			continue
		}
		for j := range ws.Spec.Services {
			if ws.Spec.Services[j].Name == serviceGroup {
				return ws.Spec.Services[j], true
			}
		}
	}
	return omniav1alpha1.WorkspaceServiceGroup{}, false
}

// resolveSessionURLForWorkspace looks up the session-api URL for the given namespace
// and service group from the Workspace CRD status. Returns an empty string if not found.
func (r *AgentRuntimeReconciler) resolveSessionURLForWorkspace(ctx context.Context, namespace, serviceGroup string) string {
	var list omniav1alpha1.WorkspaceList
	if err := r.List(ctx, &list); err != nil {
		return ""
	}
	for i := range list.Items {
		ws := &list.Items[i]
		if ws.Spec.Namespace.Name != namespace {
			continue
		}
		for _, sg := range ws.Status.Services {
			if sg.Name == serviceGroup && sg.Ready {
				return sg.SessionURL
			}
		}
	}
	return ""
}

// evalWorkerImage returns the image to use for the eval worker container.
func (r *AgentRuntimeReconciler) evalWorkerImage() string {
	if r.EvalWorkerImage != "" {
		return r.EvalWorkerImage
	}
	return DefaultEvalWorkerImage
}
