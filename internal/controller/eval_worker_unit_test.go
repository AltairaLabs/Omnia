/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	testRedisSecretKey  = "url"
	testRedisSecretName = "redis-secret"
	testEvalAgentName   = "agent"
	testStaleGroup      = "gone"
	testWISA            = "omnia-runtime-wi"
)

func newEvalWorkerTestReconciler() *AgentRuntimeReconciler {
	scheme := evalWorkerTestScheme()
	return &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
}

// evalWorkerTestScheme registers every type the eval-worker reconcile path
// creates or lists: AgentRuntime/Workspace (omnia), Deployment (apps),
// ServiceAccount (core), and Role/RoleBinding/ClusterRoleBinding (rbac).
func evalWorkerTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	return scheme
}

func TestBuildEvalWorkerDeployment_PodOverrides(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Evals: &omniav1alpha1.EvalConfig{
				Enabled: true,
				PodOverrides: &omniav1alpha1.PodOverrides{
					ServiceAccountName: "eval-sa",
					NodeSelector:       map[string]string{"workload": "batch"},
					ExtraVolumes:       []corev1.Volume{{Name: "kv"}},
					ExtraVolumeMounts:  []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
					ExtraEnv:           []corev1.EnvVar{{Name: "JUDGE_API_KEY_FILE", Value: "/mnt/kv/key"}},
				},
			},
		},
	}

	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", defaultSvcGroupName, agent.Spec.Evals.PodOverrides)
	spec := dep.Spec.Template.Spec

	require.Equal(t, "eval-sa", spec.ServiceAccountName)
	require.Equal(t, "batch", spec.NodeSelector["workload"])
	require.NotEmpty(t, spec.Volumes)
	require.Equal(t, "kv", spec.Volumes[0].Name)

	c := spec.Containers[0]
	hasEnv := false
	for _, e := range c.Env {
		if e.Name == "JUDGE_API_KEY_FILE" {
			hasEnv = true
		}
	}
	require.True(t, hasEnv, "extraEnv must be applied on eval-worker container")
	require.NotEmpty(t, c.VolumeMounts)
	require.Equal(t, "kv", c.VolumeMounts[0].Name)
}

func TestBuildEvalWorkerDeployment_ImagePullPolicy(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	r.EvalWorkerImagePullPolicy = corev1.PullIfNotPresent

	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", defaultSvcGroupName, nil)
	require.Equal(t, corev1.PullIfNotPresent,
		dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
}

func TestBuildEvalWorkerDeployment_NoOverrides(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", defaultSvcGroupName, nil)
	require.Equal(t, "arena-eval-worker-default", dep.Spec.Template.Spec.ServiceAccountName,
		"no overrides, default to the eval-worker SA")
}

func TestBuildEvalWorkerDeployment_SetsServiceAccount(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", defaultSvcGroupName, nil)
	require.Equal(t, "arena-eval-worker-default", dep.Spec.Template.Spec.ServiceAccountName)
}

func TestEvalWorkerName_PerGroup(t *testing.T) {
	require.Equal(t, "arena-eval-worker-default", evalWorkerName(defaultSvcGroupName))
	require.Equal(t, "arena-eval-worker-prod", evalWorkerName("prod"))
}

func TestBuildEvalWorkerDeployment_PrometheusScrape(t *testing.T) {
	r := newEvalWorkerTestReconciler()
	dep := r.buildEvalWorkerDeployment(context.Background(), "ns", defaultSvcGroupName, nil)

	tmpl := dep.Spec.Template

	// The pod carries the app.kubernetes.io/component label the omnia-eval-worker
	// scrape job keeps on, plus the prometheus.io scrape annotations.
	require.Equal(t, "eval-worker", tmpl.Labels["app.kubernetes.io/component"])
	require.Equal(t, "true", tmpl.Annotations["prometheus.io/scrape"])
	require.Equal(t, "9090", tmpl.Annotations["prometheus.io/port"])
	require.Equal(t, "/metrics", tmpl.Annotations["prometheus.io/path"])

	// The component label must NOT be on the immutable selector.
	_, inSelector := dep.Spec.Selector.MatchLabels["app.kubernetes.io/component"]
	require.False(t, inSelector, "component label must not be added to the selector")

	// The container exposes the metrics port the annotation points at.
	ports := tmpl.Spec.Containers[0].Ports
	require.Len(t, ports, 1)
	require.Equal(t, int32(9090), ports[0].ContainerPort)
}

func keysOf(m map[string]*omniav1alpha1.PodOverrides) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestServiceGroupsNeedingEvalWorker(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	langChain := &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeLangChain}
	agentA := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: defaultSvcGroupName,
			Framework:    langChain,
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	agentB := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "prod",
			Framework:    langChain,
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	agentC := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "pk",
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypePromptKit},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(agentA, agentB, agentC).Build(),
		Scheme: scheme,
	}

	needed, err := r.serviceGroupsNeedingEvalWorker(context.Background(), "ns")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{defaultSvcGroupName, "prod"}, keysOf(needed))
}

func TestServiceGroupsNeedingEvalWorker_PromptKitGroupOptIn(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	// Two PromptKit agents in different service groups. Only "pk-optin"'s
	// WorkspaceServiceGroup opts into the eval-worker; "pk-default"'s does not.
	optIn := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "optin", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "pk-optin",
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypePromptKit},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	noOptIn := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "default-pk", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: "pk-default",
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypePromptKit},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{Name: "pk-optin", EvalWorker: &omniav1alpha1.ServiceGroupEvalWorker{Enabled: true}},
				{Name: "pk-default"},
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(optIn, noOptIn, ws).Build(),
		Scheme: scheme,
	}

	needed, err := r.serviceGroupsNeedingEvalWorker(context.Background(), "ns")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"pk-optin"}, keysOf(needed))
}

func TestEvalWorkerPodOverrides_Precedence(t *testing.T) {
	groupPO := &omniav1alpha1.PodOverrides{ServiceAccountName: "group-sa"}
	agentPO := &omniav1alpha1.PodOverrides{ServiceAccountName: "agent-sa"}
	rt := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{Evals: &omniav1alpha1.EvalConfig{PodOverrides: agentPO}},
	}

	// Group-level podOverrides wins when set.
	sgWithPO := omniav1alpha1.WorkspaceServiceGroup{
		EvalWorker: &omniav1alpha1.ServiceGroupEvalWorker{PodOverrides: groupPO},
	}
	require.Equal(t, groupPO, evalWorkerPodOverrides(sgWithPO, true, rt))

	// Falls back to the agent's evals.podOverrides when the group sets none.
	sgNoPO := omniav1alpha1.WorkspaceServiceGroup{
		EvalWorker: &omniav1alpha1.ServiceGroupEvalWorker{Enabled: true},
	}
	require.Equal(t, agentPO, evalWorkerPodOverrides(sgNoPO, true, rt))

	// Falls back to the agent's evals.podOverrides when the group is not found.
	require.Equal(t, agentPO, evalWorkerPodOverrides(omniav1alpha1.WorkspaceServiceGroup{}, false, rt))
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func findEnv(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for _, e := range env {
		if e.Name == name {
			return e, true
		}
	}
	return corev1.EnvVar{}, false
}

func TestEvalWorkerEnv_GroupRedisLiteral(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name:  defaultSvcGroupName,
					Redis: &omniav1alpha1.RedisConfig{URL: "redis://group.example.com:6379/0"},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: defaultSvcGroupName, SessionURL: "http://session-ws-default.ns:8080", Ready: true},
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ws).Build(),
		Scheme:          scheme,
		RedisURL:        "redis://operator-default:6379/0",
		SessionRedisURL: "redis://operator-session:6379/0",
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", defaultSvcGroupName)
	require.Equal(t, "redis://group.example.com:6379/0", envValue(env, "REDIS_URL"))
}

func TestEvalWorkerEnv_GroupRedisExistingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: defaultSvcGroupName,
					Redis: &omniav1alpha1.RedisConfig{
						ExistingSecret: &omniav1alpha1.RedisSecretRef{Name: "grp-redis", Key: testRedisSecretKey},
					},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: defaultSvcGroupName, SessionURL: "http://session-ws-default.ns:8080", Ready: true},
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ws).Build(),
		Scheme: scheme,
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", defaultSvcGroupName)
	redisEnv, ok := findEnv(env, "REDIS_URL")
	require.True(t, ok, "REDIS_URL env must be set")
	require.Empty(t, redisEnv.Value, "secret-sourced REDIS_URL must not set Value")
	require.NotNil(t, redisEnv.ValueFrom)
	require.NotNil(t, redisEnv.ValueFrom.SecretKeyRef)
	require.Equal(t, "grp-redis", redisEnv.ValueFrom.SecretKeyRef.Name)
	require.Equal(t, testRedisSecretKey, redisEnv.ValueFrom.SecretKeyRef.Key)
}

func TestEvalWorkerEnv_FallbackToSessionRedisDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	// No Workspace objects: findServiceGroup returns false, so the eval-worker
	// must fall back to the operator default. SessionRedisURL takes precedence
	// over the legacy RedisURL.
	r := &AgentRuntimeReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme:          scheme,
		RedisURL:        "redis://operator-default:6379/0",
		SessionRedisURL: "redis://operator-session:6379/0",
	}

	env := r.buildEvalWorkerEnvVars(context.Background(), "ns", defaultSvcGroupName)
	require.Equal(t, "redis://operator-session:6379/0", envValue(env, "REDIS_URL"))
}

func TestReconcileEvalWorker_WiringEndToEnd(t *testing.T) {
	scheme := evalWorkerTestScheme()

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name:  defaultSvcGroupName,
					Redis: &omniav1alpha1.RedisConfig{URL: "redis://wiring.example.com:6379/0"},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: defaultSvcGroupName, SessionURL: "http://session-ws-default.ns:8080", Ready: true},
			},
		},
	}

	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: testEvalAgentName, Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: defaultSvcGroupName,
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeLangChain},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ws, agent).Build(),
		Scheme: scheme,
		// Both operator defaults are set to distinct values so the assertion that
		// the GROUP redis wins proves end-to-end resolution, not a default leak.
		RedisURL:        "redis://operator-default:6379/0",
		SessionRedisURL: "redis://operator-session:6379/0",
	}

	require.NoError(t, r.reconcileEvalWorker(context.Background(), agent))

	dep := &appsv1.Deployment{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: evalWorkerName(defaultSvcGroupName), Namespace: "ns"}, dep))

	env := dep.Spec.Template.Spec.Containers[0].Env
	require.Equal(t, "http://session-ws-default.ns:8080", envValue(env, "SESSION_API_URL"),
		"eval-worker must be wired to its group's session-api URL")
	require.Equal(t, "redis://wiring.example.com:6379/0", envValue(env, "REDIS_URL"),
		"eval-worker must consume the GROUP redis, not the operator default")
	require.Equal(t, defaultSvcGroupName, envValue(env, "OMNIA_SERVICE_GROUP"))
	require.Equal(t, "ns", envValue(env, "NAMESPACE"))
}

func TestReconcileEvalWorker_PerGroup_CreatesAndCleansUp(t *testing.T) {
	scheme := evalWorkerTestScheme()

	agentDefault := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ServiceGroup: defaultSvcGroupName,
			Framework:    &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeLangChain},
			Evals:        &omniav1alpha1.EvalConfig{Enabled: true},
		},
	}
	staleDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evalWorkerName(testStaleGroup),
			Namespace: "ns",
			Labels: map[string]string{
				labelAppName:      labelValueEvalWorker,
				labelAppManagedBy: labelValueOmniaOperator,
				labelServiceGroup: testStaleGroup,
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(agentDefault, staleDep).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.reconcileEvalWorker(context.Background(), agentDefault))

	// The needed worker exists.
	got := &appsv1.Deployment{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: evalWorkerName(defaultSvcGroupName), Namespace: "ns"}, got))

	// The stale worker was cleaned up.
	err := r.Get(context.Background(),
		types.NamespacedName{Name: evalWorkerName(testStaleGroup), Namespace: "ns"}, &appsv1.Deployment{})
	require.True(t, apierrors.IsNotFound(err))
}

const testWorkspaceReaderClusterRole = "agent-workspace-reader"

// evalWorkerNsWorkspace owns the "ns" namespace these tests deploy into. The
// eval-worker binds a reader scoped to its own workspace, so a Workspace is a
// precondition for the binding to exist at all (#1875).
func evalWorkerNsWorkspace() *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "eval-ws", UID: "eval-ws-uid"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Eval",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ns"},
		},
	}
}

func TestEnsureEvalWorkerRBAC_CreatesObjects(t *testing.T) {
	scheme := evalWorkerTestScheme()
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(evalWorkerNsWorkspace()).Build(),
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: testWorkspaceReaderClusterRole,
	}

	require.NoError(t, r.ensureEvalWorkerRBAC(context.Background(), "ns", defaultSvcGroupName, nil))

	name := evalWorkerName(defaultSvcGroupName)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, sa))

	role := &rbacv1.Role{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, role))
	require.True(t, roleGrantsGet(role, "configmaps"), "Role must grant get on configmaps")

	rb := &rbacv1.RoleBinding{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, rb))
	require.Equal(t, name, rb.RoleRef.Name)
	require.Len(t, rb.Subjects, 1)
	require.Equal(t, "ServiceAccount", rb.Subjects[0].Kind)
	require.Equal(t, name, rb.Subjects[0].Name)

	crb := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "ns-" + name + "-workspace-reader"}, crb))
	// Scoped to the eval-worker's own workspace, not the cluster-wide reader (#1875).
	require.Equal(t, WorkspaceReaderClusterRoleName("eval-ws"), crb.RoleRef.Name)
	require.NotEqual(t, testWorkspaceReaderClusterRole, crb.RoleRef.Name)
	require.Equal(t, "ns", crb.Labels[labelWorkspaceReaderFor])
}

func TestEnsureEvalWorkerRBAC_OverriddenServiceAccount(t *testing.T) {
	scheme := evalWorkerTestScheme()
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(evalWorkerNsWorkspace()).Build(),
		Scheme:                          scheme,
		AgentWorkspaceReaderClusterRole: testWorkspaceReaderClusterRole,
	}

	// Point the worker at an external (e.g. workload-identity) SA.
	po := &omniav1alpha1.PodOverrides{ServiceAccountName: testWISA}
	require.NoError(t, r.ensureEvalWorkerRBAC(context.Background(), "ns", defaultSvcGroupName, po))

	name := evalWorkerName(defaultSvcGroupName)

	// The operator must NOT create the externally-owned SA.
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "ns"}, &corev1.ServiceAccount{})
	require.True(t, apierrors.IsNotFound(err), "default SA must not be created when overridden")

	// Role + RoleBinding keep the default name, but the subject is the override.
	rb := &rbacv1.RoleBinding{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "ns"}, rb))
	require.Equal(t, name, rb.RoleRef.Name)
	require.Equal(t, testWISA, rb.Subjects[0].Name)

	crb := &rbacv1.ClusterRoleBinding{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "ns-" + name + "-workspace-reader"}, crb))
	require.Equal(t, testWISA, crb.Subjects[0].Name)
}

func TestEnsureEvalWorkerRBAC_SkipsCRBWhenNoClusterRole(t *testing.T) {
	scheme := evalWorkerTestScheme()
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
		// AgentWorkspaceReaderClusterRole intentionally empty.
	}

	require.NoError(t, r.ensureEvalWorkerRBAC(context.Background(), "ns", defaultSvcGroupName, nil))

	name := evalWorkerName(defaultSvcGroupName)

	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, &corev1.ServiceAccount{}))
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, &rbacv1.Role{}))
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: "ns"}, &rbacv1.RoleBinding{}))

	err := r.Get(context.Background(),
		types.NamespacedName{Name: "ns-" + name + "-workspace-reader"}, &rbacv1.ClusterRoleBinding{})
	require.True(t, apierrors.IsNotFound(err), "no ClusterRoleBinding when ClusterRole is unset")
}

func TestCleanupEvalWorkers_DeletesStaleRBAC(t *testing.T) {
	scheme := evalWorkerTestScheme()
	goneName := evalWorkerName(testStaleGroup)
	goneLabels := map[string]string{
		labelAppName:      labelValueEvalWorker,
		labelAppManagedBy: labelValueOmniaOperator,
		labelServiceGroup: testStaleGroup,
	}
	crbLabels := map[string]string{
		labelAppName:            labelValueEvalWorker,
		labelAppManagedBy:       labelValueOmniaOperator,
		labelServiceGroup:       testStaleGroup,
		labelWorkspaceReaderFor: "ns",
	}

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: goneName, Namespace: "ns", Labels: goneLabels}}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: goneName, Namespace: "ns", Labels: goneLabels}}
	rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: goneName, Namespace: "ns", Labels: goneLabels}}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-" + goneName + "-workspace-reader", Labels: crbLabels},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(sa, role, rb, crb).Build(),
		Scheme: scheme,
	}

	needed := map[string]*omniav1alpha1.PodOverrides{defaultSvcGroupName: nil}
	require.NoError(t, r.cleanupEvalWorkers(context.Background(), "ns", needed))

	require.True(t, apierrors.IsNotFound(r.Get(context.Background(),
		types.NamespacedName{Name: goneName, Namespace: "ns"}, &corev1.ServiceAccount{})))
	require.True(t, apierrors.IsNotFound(r.Get(context.Background(),
		types.NamespacedName{Name: goneName, Namespace: "ns"}, &rbacv1.Role{})))
	require.True(t, apierrors.IsNotFound(r.Get(context.Background(),
		types.NamespacedName{Name: goneName, Namespace: "ns"}, &rbacv1.RoleBinding{})))
	require.True(t, apierrors.IsNotFound(r.Get(context.Background(),
		types.NamespacedName{Name: "ns-" + goneName + "-workspace-reader"}, &rbacv1.ClusterRoleBinding{})))
}

// TestCleanupEvalWorkers_PreservesOtherNamespaceCRB verifies that cleanup of
// namespace nsA's stale eval-worker ClusterRoleBinding does not delete an
// otherwise-identically-named binding belonging to a different namespace nsB.
// The workspace-reader-for label scoping must keep the two apart even though
// both namespaces have a service group named "default".
func TestCleanupEvalWorkers_PreservesOtherNamespaceCRB(t *testing.T) {
	scheme := evalWorkerTestScheme()

	// nsB's live "default" CRB — must survive cleanup of nsA.
	nsBName := evalWorkerName(defaultSvcGroupName)
	nsBCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nsB-" + nsBName + "-workspace-reader",
			Labels: map[string]string{
				labelAppName:            labelValueEvalWorker,
				labelAppManagedBy:       labelValueOmniaOperator,
				labelServiceGroup:       defaultSvcGroupName,
				labelWorkspaceReaderFor: "nsB",
			},
		},
	}

	// nsA's stale testStaleGroup CRB — must be deleted.
	nsAGoneName := evalWorkerName(testStaleGroup)
	nsACRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nsA-" + nsAGoneName + "-workspace-reader",
			Labels: map[string]string{
				labelAppName:            labelValueEvalWorker,
				labelAppManagedBy:       labelValueOmniaOperator,
				labelServiceGroup:       testStaleGroup,
				labelWorkspaceReaderFor: "nsA",
			},
		},
	}

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(nsBCRB, nsACRB).Build(),
		Scheme: scheme,
	}

	needed := map[string]*omniav1alpha1.PodOverrides{defaultSvcGroupName: nil}
	require.NoError(t, r.cleanupEvalWorkers(context.Background(), "nsA", needed))

	// nsA's stale CRB is gone.
	require.True(t, apierrors.IsNotFound(r.Get(context.Background(),
		types.NamespacedName{Name: "nsA-" + nsAGoneName + "-workspace-reader"},
		&rbacv1.ClusterRoleBinding{})),
		"nsA's stale CRB must be deleted")

	// nsB's CRB must be untouched — the workspace-reader-for=nsA filter must
	// not select nsB's binding.
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "nsB-" + nsBName + "-workspace-reader"},
		&rbacv1.ClusterRoleBinding{}),
		"nsB's CRB must survive cleanup of nsA")
}

func roleGrantsGet(role *rbacv1.Role, resource string) bool {
	for _, rule := range role.Rules {
		hasResource := false
		for _, res := range rule.Resources {
			if res == resource {
				hasResource = true
			}
		}
		if !hasResource {
			continue
		}
		for _, verb := range rule.Verbs {
			if verb == verbGet {
				return true
			}
		}
	}
	return false
}
