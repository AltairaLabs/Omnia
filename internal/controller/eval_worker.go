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
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

// evalWorkerMetricsPort / evalWorkerMetricsPath are where the eval-worker serves
// its Prometheus metrics (matches the binary's METRICS_ADDR default of :9090).
// The pod is annotated with them so the omnia-eval-worker scrape job targets it.
const (
	evalWorkerMetricsPort = 9090
	evalWorkerMetricsPath = "/metrics"
)

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
		if !hasEvalsEnabled(&rt.Spec) {
			continue
		}
		group := rt.Spec.ServiceGroup
		if group == "" {
			group = defaultSvcGroupName
		}
		sg, sgFound := r.findServiceGroup(ctx, namespace, group)
		// PromptKit agents self-evaluate inline and are excluded from the
		// eval-worker by default. Their group can opt in via the
		// WorkspaceServiceGroup evalWorker.enabled flag to run llm_judge
		// (long-running/external) evals out-of-band — the only path that
		// emits worker-only labels like `variant`.
		if isPromptKit(&rt.Spec) && !groupEvalWorkerEnabled(sg, sgFound) {
			continue
		}
		if _, seen := needed[group]; !seen {
			needed[group] = evalWorkerPodOverrides(sg, sgFound, rt)
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

	if err := r.ensureEvalWorkerRBAC(ctx, namespace, serviceGroup, podOverrides); err != nil {
		return fmt.Errorf("ensure eval worker RBAC %s/%s: %w", namespace, serviceGroup, err)
	}

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

// cleanupEvalWorkers deletes operator-managed eval worker Deployments and their
// RBAC (ServiceAccount, Role, RoleBinding, ClusterRoleBinding) whose service
// group is no longer in the needed set. The legacy un-suffixed singleton has an
// empty service-group label, which is never a key in needed, so it is removed
// here.
func (r *AgentRuntimeReconciler) cleanupEvalWorkers(
	ctx context.Context,
	namespace string,
	needed map[string]*omniav1alpha1.PodOverrides,
) error {
	nsLabels := client.MatchingLabels{
		labelAppName:      labelValueEvalWorker,
		labelAppManagedBy: labelValueOmniaOperator,
	}

	namespaced := []client.ObjectList{
		&appsv1.DeploymentList{},
		&corev1.ServiceAccountList{},
		&rbacv1.RoleList{},
		&rbacv1.RoleBindingList{},
	}
	for _, list := range namespaced {
		if err := r.deleteStaleEvalWorkerObjects(ctx, list, needed,
			client.InNamespace(namespace), nsLabels); err != nil {
			return err
		}
	}

	// ClusterRoleBindings are cluster-scoped; filter by the workspace-reader-for
	// label so we only touch this namespace's bindings (two namespaces can both
	// have a group named "default").
	crbLabels := client.MatchingLabels{
		labelAppName:            labelValueEvalWorker,
		labelAppManagedBy:       labelValueOmniaOperator,
		labelWorkspaceReaderFor: namespace,
	}
	return r.deleteStaleEvalWorkerObjects(ctx, &rbacv1.ClusterRoleBindingList{}, needed, crbLabels)
}

// deleteStaleEvalWorkerObjects lists objects matching the given list options and
// deletes each whose service-group label is not a key in needed. NotFound on
// delete is ignored (concurrent reconciles may have already removed it).
func (r *AgentRuntimeReconciler) deleteStaleEvalWorkerObjects(
	ctx context.Context,
	list client.ObjectList,
	needed map[string]*omniav1alpha1.PodOverrides,
	opts ...client.ListOption,
) error {
	log := logf.FromContext(ctx)

	if err := r.List(ctx, list, opts...); err != nil {
		return fmt.Errorf("failed to list eval worker objects: %w", err)
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		return fmt.Errorf("failed to extract eval worker object list: %w", err)
	}

	for _, item := range items {
		obj, ok := item.(client.Object)
		if !ok {
			continue
		}
		group := obj.GetLabels()[labelServiceGroup]
		if _, keep := needed[group]; keep {
			continue
		}
		log.V(1).Info("deleting stale eval worker object",
			"name", obj.GetName(), "namespace", obj.GetNamespace(), "serviceGroup", group)
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete stale eval worker object %s: %w", obj.GetName(), err)
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

	// The pod template carries the app.kubernetes.io/component label and the
	// prometheus.io scrape annotations the omnia-eval-worker Prometheus job
	// matches on, so omnia_eval_* (faithfulness, …) is scraped from every
	// per-service-group worker across the cluster (multi-workspace: the job
	// discovers pods by label, not by name). The component label is deliberately
	// NOT added to the immutable Deployment selector.
	podLabels := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		podLabels[k] = v
	}
	podLabels[labelAppComponent] = labelEvalWorkerComp
	podAnnotations := map[string]string{
		"prometheus.io/scrape": labelValueTrue,
		"prometheus.io/port":   strconv.Itoa(evalWorkerMetricsPort),
		"prometheus.io/path":   evalWorkerMetricsPath,
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
					Labels:      podLabels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					Containers: []corev1.Container{
						{
							Name:            EvalWorkerContainerName,
							Image:           image,
							ImagePullPolicy: r.EvalWorkerImagePullPolicy,
							Ports: []corev1.ContainerPort{{
								Name:          metricsPortName,
								ContainerPort: evalWorkerMetricsPort,
							}},
							Env: r.buildEvalWorkerEnvVars(ctx, namespace, serviceGroup),
						},
					},
				},
			},
		},
	}

	// Internal service auth (SEC-1/SEC-5): the eval-worker reads eval results
	// from session-api, so it presents an audience-bound projected SA token
	// when enabled. Applied before podOverrides so user extraVolumeMounts win
	// on collision. No-op when disabled.
	r.ServiceAuth.applyCallerToken(&dep.Spec.Template.Spec)

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

	// The eval-worker resolves its own service URLs from the Workspace when no
	// session URL override applies, so it needs the workspace name for the same
	// scoped Get the agent does. It has no AgentRuntime of its own to fall back
	// to, so this injection is the only source (#1875).
	if wsName, _ := r.resolveWorkspaceForNamespace(namespace); wsName != "" {
		envVars = append(envVars, corev1.EnvVar{Name: envWorkspaceName, Value: wsName})
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

// groupEvalWorkerEnabled reports whether a resolved service group opts into the
// eval-worker via its WorkspaceServiceGroup evalWorker.enabled flag. This lets
// PromptKit agents — which self-evaluate inline and are otherwise excluded —
// run their out-of-band (llm_judge) evals when the group operator opts in.
// Returns false when the group was not found.
func groupEvalWorkerEnabled(sg omniav1alpha1.WorkspaceServiceGroup, found bool) bool {
	return found && sg.EvalWorker != nil && sg.EvalWorker.Enabled
}

// evalWorkerPodOverrides returns the PodOverrides for the group's eval-worker:
// the group-level WorkspaceServiceGroup.evalWorker.podOverrides when set (the
// place a worker's cloud workload-identity belongs, since the worker is
// per-group), otherwise the representative agent's spec.evals.podOverrides for
// backward compatibility.
func evalWorkerPodOverrides(sg omniav1alpha1.WorkspaceServiceGroup, found bool, rt *omniav1alpha1.AgentRuntime) *omniav1alpha1.PodOverrides {
	if found && sg.EvalWorker != nil && sg.EvalWorker.PodOverrides != nil {
		return sg.EvalWorker.PodOverrides
	}
	return rt.Spec.Evals.PodOverrides
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
