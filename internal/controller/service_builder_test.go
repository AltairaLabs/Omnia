/*
Copyright 2026 Altaira Labs.

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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Test constants extracted to satisfy goconst.
const (
	testOperatorRedisURL = "redis://operator-default:6379/0"
	testRedisURLEnvName  = "REDIS_URL"
)

func newTestServiceGroup(name string) omniav1alpha1.WorkspaceServiceGroup {
	return omniav1alpha1.WorkspaceServiceGroup{
		Name: name,
		Mode: omniav1alpha1.ServiceModeManaged,
	}
}

func newTestServiceBuilder() ServiceBuilder {
	return ServiceBuilder{
		SessionImage:           "ghcr.io/altairalabs/omnia-session-api:test",
		SessionImagePullPolicy: corev1.PullIfNotPresent,
		MemoryImage:            "ghcr.io/altairalabs/omnia-memory-api:test",
		MemoryImagePullPolicy:  corev1.PullIfNotPresent,
	}
}

func TestBuildSessionDeployment(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("my-workspace", "my-ns", sg)

	require.NotNil(t, dep)
	assert.Equal(t, "session-my-workspace-default", dep.Name)
	assert.Equal(t, "my-ns", dep.Namespace)

	// Labels
	labels := dep.Labels
	assert.Equal(t, "session-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "my-workspace", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", labels["omnia.altairalabs.ai/service-group"])

	// Replicas
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	// Container
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, sb.SessionImage, container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)

	// Args
	assert.Contains(t, container.Args, "--workspace=my-workspace")
	assert.Contains(t, container.Args, "--service-group=default")

	// Ports
	require.Len(t, container.Ports, 2)
	portNames := map[string]int32{}
	for _, p := range container.Ports {
		portNames[p.Name] = p.ContainerPort
	}
	assert.Equal(t, int32(servicePort), portNames["http"])
	assert.Equal(t, int32(healthPort), portNames["health"])

	// Probes
	require.NotNil(t, container.LivenessProbe)
	require.NotNil(t, container.LivenessProbe.HTTPGet)
	assert.Equal(t, "/healthz", container.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, int32(healthPort), container.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, container.ReadinessProbe)
	require.NotNil(t, container.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/readyz", container.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, int32(healthPort), container.ReadinessProbe.HTTPGet.Port.IntVal)

	// Security context
	sc := container.SecurityContext
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.Capabilities)
	assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))
	require.NotNil(t, sc.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
}

// TestBuildSessionDeployment_NoRedisFlagByDefault proves session-api
// pods are built without --redis-url when no Redis is configured —
// session-api gracefully degrades to warm-tier-only when REDIS_URL is
// empty, but emitting the flag with empty value would surface
// confusing "parse redis URL" errors at startup.
func TestBuildSessionDeployment_NoRedisFlagByDefault(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	for _, a := range dep.Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(a, "--redis-url=") {
			t.Errorf("expected no --redis-url flag, got %q", a)
		}
	}
}

// TestBuildSessionDeployment_OperatorRedisURLLiteral proves the
// operator-wide --session-redis-url flag flows through to session-api
// pods. Without this, configuring SessionRedisURL on ServiceBuilder
// would be cosmetic.
func TestBuildSessionDeployment_OperatorRedisURLLiteral(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.SessionRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args,
		"--redis-url=redis://operator-default:6379/0")
}

// TestBuildSessionDeployment_PerWorkspaceRedisURLOverridesOperator
// proves per-workspace session.redis.url shadows the operator-wide
// SessionRedisURL. Mirrors the memory-api per-workspace test from #1062.
func TestBuildSessionDeployment_PerWorkspaceRedisURLOverridesOperator(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.SessionRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("prod")
	sg.Session = &omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			URL: "rediss://customer-acme-sessions.example.com:6379/2",
		},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=rediss://customer-acme-sessions.example.com:6379/2")
	assert.NotContains(t, args, "--redis-url=redis://operator-default:6379/0")
}

// TestBuildSessionDeployment_PerWorkspaceRedisExistingSecret proves
// the secret form on a per-workspace session override: $(REDIS_URL)
// flag placeholder + REDIS_URL env from the workspace's Secret.
func TestBuildSessionDeployment_PerWorkspaceRedisExistingSecret(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.SessionRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("prod")
	sg.Session = &omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			ExistingSecret: &omniav1alpha1.RedisSecretRef{
				Name: "acme-session-redis",
				Key:  "url",
			},
		},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--redis-url=$(REDIS_URL)")

	var env *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == testRedisURLEnvName {
			env = &container.Env[i]
			break
		}
	}
	require.NotNil(t, env, "expected REDIS_URL env from per-workspace Secret")
	require.NotNil(t, env.ValueFrom)
	require.NotNil(t, env.ValueFrom.SecretKeyRef)
	assert.Equal(t, "acme-session-redis", env.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "url", env.ValueFrom.SecretKeyRef.Key)
}

func TestBuildMemoryDeployment(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)

	require.NotNil(t, dep)
	assert.Equal(t, "memory-acme-prod", dep.Name)
	assert.Equal(t, "acme-ns", dep.Namespace)

	// Labels
	labels := dep.Labels
	assert.Equal(t, "memory-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "acme", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "prod", labels["omnia.altairalabs.ai/service-group"])

	// Container
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, sb.MemoryImage, container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)

	// Args. Each --*-interval flag is a wiring boundary: memory-api defaults
	// every worker to "off" when its flag is empty, so anything the operator
	// doesn't pass silently doesn't run. The compaction / tombstone / reembed
	// workers were dead in production for weeks until issue #1038 surfaced
	// the gap. These asserts fail loudly if anyone removes the bridge.
	assert.Contains(t, container.Args, "--workspace=acme")
	assert.Contains(t, container.Args, "--service-group=prod")
	assert.Contains(t, container.Args, "--compaction-interval=24h")
	assert.Contains(t, container.Args, "--tombstone-interval=24h")
	assert.Contains(t, container.Args, "--reembed-interval=60m")

	// POD_NAMESPACE downward API — required so the embedding-provider
	// lookup defaults to the workspace namespace (where the Provider
	// CRD lives) instead of falling back to omnia-system.
	var podNS *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == "POD_NAMESPACE" {
			podNS = &container.Env[i]
			break
		}
	}
	require.NotNil(t, podNS, "POD_NAMESPACE env var missing on memory-api container")
	require.NotNil(t, podNS.ValueFrom, "POD_NAMESPACE must come from downward API, not a literal")
	require.NotNil(t, podNS.ValueFrom.FieldRef)
	assert.Equal(t, "metadata.namespace", podNS.ValueFrom.FieldRef.FieldPath)
}

// TestBuildMemoryDeployment_AnnotatesConfigHash proves a providerRef
// change on the workspace's memory service group flows through to the
// Deployment's pod template annotations — the only signal Kubernetes
// uses to roll a Deployment whose container image and args haven't
// changed. Without this, switching memory.providerRef leaves the
// running pod with a stale config until something else rolls it.
func TestBuildMemoryDeployment_AnnotatesConfigHash(t *testing.T) {
	sb := newTestServiceBuilder()

	baseMem := omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{
			SecretRef: corev1.LocalObjectReference{Name: "memory-db"},
		},
	}

	sgNoProvider := newTestServiceGroup("prod")
	memNoProvider := baseMem
	sgNoProvider.Memory = &memNoProvider
	depA := sb.BuildMemoryDeployment("acme", "acme-ns", sgNoProvider)

	sgWithProvider := newTestServiceGroup("prod")
	memWithProvider := baseMem
	memWithProvider.ProviderRef = &corev1.LocalObjectReference{Name: "gemini-embeddings"}
	sgWithProvider.Memory = &memWithProvider
	depB := sb.BuildMemoryDeployment("acme", "acme-ns", sgWithProvider)

	annoA := depA.Spec.Template.Annotations[annotationConfigHash]
	annoB := depB.Spec.Template.Annotations[annotationConfigHash]
	require.NotEmpty(t, annoA, "configHash annotation missing on memory-api pod template")
	require.NotEmpty(t, annoB)
	assert.NotEqual(t, annoA, annoB,
		"providerRef change must alter the configHash so the pod rolls")
}

// TestBuildMemoryDeployment_RedisFlagsAbsentByDefault proves the
// operator does NOT pass --redis-url / --cache-ttl when the
// ServiceBuilder is constructed without Redis config. Memory-api
// then runs with the cache disabled and no event publisher, which
// is the correct OSS / non-Redis dev install behaviour. Passing
// empty strings as flags would degrade into "--redis-url=" and
// trigger goredis.ParseURL to fail on every startup.
func TestBuildMemoryDeployment_RedisFlagsAbsentByDefault(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	for _, a := range container.Args {
		if strings.HasPrefix(a, "--redis-url=") {
			t.Errorf("expected no --redis-url flag when MemoryRedisURL is empty, got %q", a)
		}
		if strings.HasPrefix(a, "--cache-ttl=") {
			t.Errorf("expected no --cache-ttl flag when MemoryCacheTTL is empty, got %q", a)
		}
	}
	for _, e := range container.Env {
		if e.Name == testRedisURLEnvName {
			t.Errorf("expected no REDIS_URL env when no Secret reference is configured, got %+v", e)
		}
	}
}

// TestBuildMemoryDeployment_RedisURLLiteralThreaded proves that an
// operator-level literal Redis URL flows through to every per-workspace
// memory-api pod as --redis-url, with no REDIS_URL Secret env (because
// no Secret reference was configured). The URL is consumed verbatim by
// goredis.ParseURL at memory-api startup.
func TestBuildMemoryDeployment_RedisURLLiteralThreaded(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryRedisURL = "redis://omnia-redis-master.omnia-system.svc.cluster.local:6379/0"
	sb.MemoryCacheTTL = "10m"
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--redis-url=redis://omnia-redis-master.omnia-system.svc.cluster.local:6379/0")
	assert.Contains(t, container.Args, "--cache-ttl=10m")
	for _, e := range container.Env {
		if e.Name == testRedisURLEnvName {
			t.Errorf("expected no REDIS_URL env for literal URL, got %+v", e)
		}
	}
}

// TestBuildMemoryDeployment_RedisURLFromSecretMountsEnv proves the
// secret-backed URL path: when ServiceBuilder is configured with the
// $(REDIS_URL) placeholder + a Secret reference, every per-workspace
// memory-api pod gets:
//   - the placeholder URL on its --redis-url flag, and
//   - a REDIS_URL env sourced from valueFrom.secretKeyRef.
//
// Kubernetes env expansion at pod startup substitutes the placeholder
// with the Secret's value before goredis.ParseURL sees it. Without the
// env mount, expansion no-ops and goredis.ParseURL fails with
// "redis: invalid URL scheme: $(REDIS_URL)".
func TestBuildMemoryDeployment_RedisURLFromSecretMountsEnv(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryRedisURL = "$(REDIS_URL)"
	sb.MemoryRedisURLSecret = SecretKeyRef{Name: "redis-url-secret", Key: "url"}
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--redis-url=$(REDIS_URL)")

	var redisURLEnv *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == testRedisURLEnvName {
			redisURLEnv = &container.Env[i]
			break
		}
	}
	require.NotNil(t, redisURLEnv, "expected REDIS_URL env to be mounted from Secret")
	require.NotNil(t, redisURLEnv.ValueFrom)
	require.NotNil(t, redisURLEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, "redis-url-secret", redisURLEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "url", redisURLEnv.ValueFrom.SecretKeyRef.Key)
}

// TestBuildMemoryDeployment_PerWorkspaceRedisURLOverridesOperator proves
// that setting Workspace.spec.services[].memory.redis.url shadows the
// operator-wide --memory-redis-url. Without this, a multi-tenant SaaS
// install couldn't pin one workspace's memory cache to a customer-
// specific Redis (data residency, blast-radius isolation).
func TestBuildMemoryDeployment_PerWorkspaceRedisURLOverridesOperator(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("prod")
	sg.Memory = &omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "memory-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			URL: "rediss://customer-acme.cache.example.com:6379/2",
		},
	}

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=rediss://customer-acme.cache.example.com:6379/2")
	assert.NotContains(t, args, "--redis-url=redis://operator-default:6379/0")
}

// TestBuildMemoryDeployment_PerWorkspaceRedisExistingSecret proves the
// secret form on a per-workspace override: the operator emits the
// $(REDIS_URL) placeholder flag plus mounts REDIS_URL env from the
// workspace's Secret (not the operator-default Secret). Production-
// typical: the workspace's Secret contains a tenant-specific URL with
// a tenant-specific password.
func TestBuildMemoryDeployment_PerWorkspaceRedisExistingSecret(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryRedisURL = testOperatorRedisURL
	sb.MemoryRedisURLSecret = SecretKeyRef{Name: "operator-secret", Key: "url"}
	sg := newTestServiceGroup("prod")
	sg.Memory = &omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "memory-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			ExistingSecret: &omniav1alpha1.RedisSecretRef{
				Name: "acme-redis-creds",
				Key:  "url",
			},
		},
	}

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--redis-url=$(REDIS_URL)")

	var env *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == testRedisURLEnvName {
			env = &container.Env[i]
			break
		}
	}
	require.NotNil(t, env, "expected REDIS_URL env from per-workspace Secret")
	require.NotNil(t, env.ValueFrom)
	require.NotNil(t, env.ValueFrom.SecretKeyRef)
	assert.Equal(t, "acme-redis-creds", env.ValueFrom.SecretKeyRef.Name,
		"per-workspace Secret must override operator-wide Secret")
	assert.Equal(t, "url", env.ValueFrom.SecretKeyRef.Key)
}

// TestBuildMemoryDeployment_PerWorkspaceRedisHostSynthesisesURL proves
// the decomposed form: workspace sets host (+ optional port/db/user),
// operator synthesises a URL. No auth — the decomposed form is
// cleartext-only by design (auth flows via existingSecret).
func TestBuildMemoryDeployment_PerWorkspaceRedisHostSynthesisesURL(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")
	sg.Memory = &omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "memory-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			Host: "tenant-redis.example.com",
			Port: 6390,
			DB:   3,
		},
	}

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=redis://tenant-redis.example.com:6390/3")
}

// TestBuildMemoryDeployment_PerWorkspaceRedisRollsOnChange proves the
// configHash annotation flips when the Redis target changes, so the
// memory-api pod rolls and picks up the new --redis-url. Without this,
// switching a workspace's Redis would leave the running pod connected
// to the old target until something else triggered a roll.
func TestBuildMemoryDeployment_PerWorkspaceRedisRollsOnChange(t *testing.T) {
	sb := newTestServiceBuilder()
	baseMem := omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "memory-db"}},
	}

	sgA := newTestServiceGroup("prod")
	memA := baseMem
	memA.Redis = &omniav1alpha1.RedisConfig{URL: "redis://a.example.com:6379/0"}
	sgA.Memory = &memA
	depA := sb.BuildMemoryDeployment("acme", "acme-ns", sgA)

	sgB := newTestServiceGroup("prod")
	memB := baseMem
	memB.Redis = &omniav1alpha1.RedisConfig{URL: "redis://b.example.com:6379/0"}
	sgB.Memory = &memB
	depB := sb.BuildMemoryDeployment("acme", "acme-ns", sgB)

	annoA := depA.Spec.Template.Annotations[annotationConfigHash]
	annoB := depB.Spec.Template.Annotations[annotationConfigHash]
	assert.NotEqual(t, annoA, annoB,
		"per-workspace Redis URL change must alter the configHash so the pod rolls")
}

// TestBuildMemoryDeployment_EmbeddingProviderArg proves that setting
// MemoryServiceConfig.ProviderRef threads through to memory-api as an
// --embedding-provider flag. Without this, the embedding service is nil
// at memory-api startup, the re-embed worker no-ops, and hybrid recall
// has no semantic side to fuse — the exact "hasEmbeddingProvider:false"
// production state from issue #1038.
func TestBuildMemoryDeployment_EmbeddingProviderArg(t *testing.T) {
	sb := newTestServiceBuilder()

	// No ProviderRef: must NOT pass --embedding-provider (memory-api treats
	// empty as "no embedder", which is correct).
	sgNoProvider := newTestServiceGroup("prod")
	depNo := sb.BuildMemoryDeployment("acme", "acme-ns", sgNoProvider)
	for _, a := range depNo.Spec.Template.Spec.Containers[0].Args {
		assert.NotContains(t, a, "--embedding-provider=",
			"unexpected embedding-provider arg when ProviderRef is unset")
	}

	// ProviderRef set: arg flows through verbatim.
	sgWithProvider := newTestServiceGroup("prod")
	sgWithProvider.Memory = &omniav1alpha1.MemoryServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{
			SecretRef: corev1.LocalObjectReference{Name: "memory-db"},
		},
		ProviderRef: &corev1.LocalObjectReference{Name: "gemini-embeddings"},
	}
	depYes := sb.BuildMemoryDeployment("acme", "acme-ns", sgWithProvider)
	assert.Contains(t, depYes.Spec.Template.Spec.Containers[0].Args,
		"--embedding-provider=gemini-embeddings",
		"--embedding-provider=<name> must flow from MemoryServiceConfig.ProviderRef")
}

func TestBuildService(t *testing.T) {
	svc := BuildService("session-my-workspace-default", "my-ns", "session-api", "my-workspace", "default")

	require.NotNil(t, svc)
	assert.Equal(t, "session-my-workspace-default", svc.Name)
	assert.Equal(t, "my-ns", svc.Namespace)

	// Labels
	labels := svc.Labels
	assert.Equal(t, "session-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "my-workspace", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", labels["omnia.altairalabs.ai/service-group"])

	// Spec
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	require.Len(t, svc.Spec.Ports, 1)
	port := svc.Spec.Ports[0]
	assert.Equal(t, int32(servicePort), port.Port)
	assert.Equal(t, "http", port.TargetPort.String())

	// Selector matches deployment labels
	assert.Equal(t, "session-api", svc.Spec.Selector["app.kubernetes.io/component"])
	assert.Equal(t, "my-workspace", svc.Spec.Selector["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", svc.Spec.Selector["omnia.altairalabs.ai/service-group"])
}

func TestServiceURL(t *testing.T) {
	url := ServiceURL("session-my-workspace-default", "my-ns")
	assert.Equal(t, "http://session-my-workspace-default.my-ns:8080", url)
}

func TestBuildServiceDeployment_PodOverrides(t *testing.T) {
	overrides := &omniav1alpha1.PodOverrides{
		ServiceAccountName: "workload-identity-sa",
		Annotations:        map[string]string{"azure.workload.identity/use": "true"},
		ExtraVolumes:       []corev1.Volume{{Name: "kv"}},
		ExtraVolumeMounts:  []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
		ExtraEnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "db-secret"}},
		}},
	}
	dep := buildServiceDeployment(
		"session-ws-default", "ns", "img:v1",
		corev1.PullIfNotPresent,
		[]string{"--x"},
		map[string]string{"a": "b"},
		overrides,
	)
	spec := dep.Spec.Template.Spec

	require.Equal(t, "workload-identity-sa", spec.ServiceAccountName)
	require.Equal(t, "true", dep.Spec.Template.Annotations["azure.workload.identity/use"])
	require.NotEmpty(t, spec.Volumes)
	require.Equal(t, "kv", spec.Volumes[0].Name)

	c := spec.Containers[0]
	require.NotEmpty(t, c.VolumeMounts)
	require.Equal(t, "kv", c.VolumeMounts[0].Name)
	require.NotEmpty(t, c.EnvFrom)
	require.Equal(t, "db-secret", c.EnvFrom[0].SecretRef.Name)
}

func TestBuildServiceDeployment_NoOverrides(t *testing.T) {
	dep := buildServiceDeployment(
		"session-ws-default", "ns", "img:v1",
		corev1.PullIfNotPresent,
		[]string{"--x"},
		map[string]string{"a": "b"},
		nil,
	)
	spec := dep.Spec.Template.Spec
	// default SA is the deployment name
	require.Equal(t, "session-ws-default", spec.ServiceAccountName)
}
