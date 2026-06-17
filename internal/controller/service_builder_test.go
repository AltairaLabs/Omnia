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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Test constants extracted to satisfy goconst.
const (
	testOperatorRedisURL  = "redis://operator-default:6379/0"
	testRedisURLEnvName   = "REDIS_URL"
	testRedisSvcName      = "redis"
	testRedisSvcNamespace = "data"
)

// assertPrometheusAnnotations asserts the annotation-driven metrics
// scrape contract: the pod template must carry prometheus.io/scrape,
// prometheus.io/port=9090, and prometheus.io/path=/metrics, plus its
// config-hash annotation must survive (don't regress the rollout signal).
func assertPrometheusAnnotations(t *testing.T, anno map[string]string) {
	t.Helper()
	require.NotNil(t, anno)
	assert.Equal(t, "true", anno["prometheus.io/scrape"])
	assert.Equal(t, "9090", anno["prometheus.io/port"])
	assert.Equal(t, "/metrics", anno["prometheus.io/path"])
	assert.NotEmpty(t, anno[annotationConfigHash],
		"config-hash annotation must survive alongside the prometheus.io/* annotations")
}

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
	require.Len(t, container.Ports, 3)
	portNames := map[string]int32{}
	for _, p := range container.Ports {
		portNames[p.Name] = p.ContainerPort
	}
	assert.Equal(t, int32(servicePort), portNames["http"])
	assert.Equal(t, int32(healthPort), portNames["health"])
	assert.Equal(t, int32(metricsPort), portNames[metricsPortName])

	// Prometheus scrape annotations — the chart's annotation-driven
	// scrape job relabels __address__ from prometheus.io/port; without
	// these the target falls back to :8080 (no /metrics) and stays DOWN.
	assertPrometheusAnnotations(t, dep.Spec.Template.Annotations)

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

// TestBuildSessionDeployment_PerWorkspaceRedisHostSynthesisesURL
// proves the decomposed form: workspace sets host (+ optional port/db/
// user), operator synthesises a URL. No auth — decomposed is cleartext-
// only by design (auth flows via existingSecret).
func TestBuildSessionDeployment_PerWorkspaceRedisHostSynthesisesURL(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")
	sg.Session = &omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
		Redis: &omniav1alpha1.RedisConfig{
			Host: "tenant-session-redis.example.com",
			Port: 6390,
			DB:   3,
			User: "sessions",
		},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=redis://sessions@tenant-session-redis.example.com:6390/3")
}

// TestBuildSessionDeployment_PerWorkspaceRedisEmptyFallsThrough proves
// that a workspace whose Session.Redis block is set but with all
// three input forms empty (CEL should reject this at admission, but
// the unit's defensive fallback returns the operator default to keep
// the Deployment renderable rather than emitting a broken --redis-url).
func TestBuildSessionDeployment_PerWorkspaceRedisEmptyFallsThrough(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.SessionRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("prod")
	sg.Session = &omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
		Redis:    &omniav1alpha1.RedisConfig{}, // all forms empty
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args,
		"--redis-url="+testOperatorRedisURL)
}

// TestBuildSessionDeployment_RollsOnRedisChange proves the
// sessionConfigHash annotation flips when the Redis target changes,
// so the session-api pod rolls and picks up the new --redis-url.
func TestBuildSessionDeployment_RollsOnRedisChange(t *testing.T) {
	sb := newTestServiceBuilder()
	baseSession := omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
	}

	sgA := newTestServiceGroup("prod")
	sessA := baseSession
	sessA.Redis = &omniav1alpha1.RedisConfig{URL: "redis://a.example.com:6379/0"}
	sgA.Session = &sessA
	depA := sb.BuildSessionDeployment("acme", "acme-ns", sgA)

	sgB := newTestServiceGroup("prod")
	sessB := baseSession
	sessB.Redis = &omniav1alpha1.RedisConfig{URL: "redis://b.example.com:6379/0"}
	sgB.Session = &sessB
	depB := sb.BuildSessionDeployment("acme", "acme-ns", sgB)

	annoA := depA.Spec.Template.Annotations[annotationConfigHash]
	annoB := depB.Spec.Template.Annotations[annotationConfigHash]
	assert.NotEqual(t, annoA, annoB,
		"per-workspace session Redis URL change must alter the configHash so the pod rolls")
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
	// Embedding spend has no session, so memory-api emits it to the co-located
	// per-group session-api's provider_usage table. Without this the spend is
	// invisible (the bug #1301 fixes).
	assert.Contains(t, container.Args, "--session-api-url=http://session-acme-prod.acme-ns:8080")

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

	// Metrics port — memory-api serves /metrics on :9090, which must be a
	// declared container port for the annotation-driven scrape to land.
	var hasMetricsPort bool
	for _, p := range container.Ports {
		if p.Name == metricsPortName {
			hasMetricsPort = true
			assert.Equal(t, int32(metricsPort), p.ContainerPort)
		}
	}
	assert.True(t, hasMetricsPort, "memory-api container missing :9090 metrics port")

	// Prometheus scrape annotations + surviving config-hash annotation.
	assertPrometheusAnnotations(t, dep.Spec.Template.Annotations)
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

// TestBuildMemoryDeployment_ConsolidationIntervalThreaded proves the
// operator-level consolidation interval flows through to every
// per-workspace memory-api pod as --consolidation-interval. That flag is
// a wiring boundary: memory-api leaves the LLM-driven consolidation worker
// OFF when the flag is absent, so a configured interval that the operator
// drops silently disables the worker (the bug this guards against — the
// Tiltfile sets workspaceServices.memoryApi.consolidation.interval=30s,
// which must reach the pod).
func TestBuildMemoryDeployment_ConsolidationIntervalThreaded(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryConsolidationInterval = "30s"
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--consolidation-interval=30s")
}

// TestBuildMemoryDeployment_ConsolidationIntervalAbsentByDefault proves
// the operator does NOT pass --consolidation-interval when the interval is
// empty. Memory-api then leaves the consolidation worker disabled, which is
// the correct default. Passing an empty flag (--consolidation-interval=)
// would make memory-api fail to parse the duration at startup.
func TestBuildMemoryDeployment_ConsolidationIntervalAbsentByDefault(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	for _, a := range container.Args {
		if strings.HasPrefix(a, "--consolidation-interval=") {
			t.Errorf("expected no --consolidation-interval flag when MemoryConsolidationInterval is empty, got %q", a)
		}
	}
}

// TestBuildMemoryDeployment_ProjectionIntervalThreaded proves the
// operator-level projection interval flows through to every per-workspace
// memory-api pod as --projection-interval. Same wiring boundary as the
// consolidation interval: memory-api leaves the Memory Galaxy pre-render
// worker OFF when the flag is absent, so a configured interval the operator
// drops silently disables the worker (the Tiltfile sets
// workspaceServices.memoryApi.projection.interval=30s, which must reach the pod).
func TestBuildMemoryDeployment_ProjectionIntervalThreaded(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryProjectionInterval = "30s"
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--projection-interval=30s")
}

// TestBuildMemoryDeployment_ProjectionIntervalAbsentByDefault proves the
// operator does NOT pass --projection-interval when the interval is empty.
// Memory-api then leaves the pre-render worker disabled (the correct default);
// an empty flag (--projection-interval=) would fail duration parsing at startup.
func TestBuildMemoryDeployment_ProjectionIntervalAbsentByDefault(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	for _, a := range container.Args {
		if strings.HasPrefix(a, "--projection-interval=") {
			t.Errorf("expected no --projection-interval flag when MemoryProjectionInterval is empty, got %q", a)
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

// TestResolveRedis_ServiceRefSynthesisesURL proves the serviceRef form
// resolves to redis://<name>.<ns>:<port> with namespace/port defaults.
func TestResolveRedis_ServiceRefSynthesisesURL(t *testing.T) {
	// Explicit namespace + port.
	url, secret := redisConfigToURL(&omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: testRedisSvcName, Namespace: testRedisSvcNamespace, Port: 6390},
	}, "acme-ns")
	assert.Equal(t, "redis://redis.data:6390", url)
	assert.Empty(t, secret.Name)

	// Namespace defaults to the workspace namespace, port defaults to 6379.
	url2, _ := redisConfigToURL(&omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: testRedisSvcName},
	}, "acme-ns")
	assert.Equal(t, "redis://redis.acme-ns:6379", url2)
}

// TestRedisConfigToURL_AllForms exercises every form of redisConfigToURL,
// including the nil guard and the host default-port path, so the resolver
// is fully covered independently of the Deployment builders.
func TestRedisConfigToURL_AllForms(t *testing.T) {
	// nil → empty.
	url, secret := redisConfigToURL(nil, "ns")
	assert.Empty(t, url)
	assert.Empty(t, secret.Name)

	// host with default port (0 → 6379).
	url, secret = redisConfigToURL(&omniav1alpha1.RedisConfig{Host: "h.example.com"}, "ns")
	assert.Equal(t, "redis://h.example.com:6379/0", url)
	assert.Empty(t, secret.Name)

	// url literal.
	url, _ = redisConfigToURL(&omniav1alpha1.RedisConfig{URL: "redis://lit:6379/1"}, "ns")
	assert.Equal(t, "redis://lit:6379/1", url)

	// existingSecret → placeholder + secret ref.
	url, secret = redisConfigToURL(&omniav1alpha1.RedisConfig{
		ExistingSecret: &omniav1alpha1.RedisSecretRef{Name: "s", Key: "k"},
	}, "ns")
	assert.Equal(t, "$(REDIS_URL)", url)
	assert.Equal(t, "s", secret.Name)
	assert.Equal(t, "k", secret.Key)

	// empty (no form populated) → empty.
	url, _ = redisConfigToURL(&omniav1alpha1.RedisConfig{}, "ns")
	assert.Empty(t, url)
}

// TestRedisHashDescriptor_AllForms exercises every descriptor branch,
// including the serviceRef form and the no-form fallthrough.
func TestRedisHashDescriptor_AllForms(t *testing.T) {
	assert.Empty(t, redisHashDescriptor(nil))
	assert.Empty(t, redisHashDescriptor(&omniav1alpha1.RedisConfig{}))
	assert.Equal(t, "url:redis://x", redisHashDescriptor(&omniav1alpha1.RedisConfig{URL: "redis://x"}))
	assert.Equal(t, "secret:s/k", redisHashDescriptor(&omniav1alpha1.RedisConfig{
		ExistingSecret: &omniav1alpha1.RedisSecretRef{Name: "s", Key: "k"},
	}))
	assert.Equal(t, "host:h:0/0:", redisHashDescriptor(&omniav1alpha1.RedisConfig{Host: "h"}))
	assert.Equal(t, "serviceRef:data/redis:6390", redisHashDescriptor(&omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: testRedisSvcName, Namespace: testRedisSvcNamespace, Port: 6390},
	}))
}

// TestBuildSessionDeployment_GroupRedisServiceRef proves a group-level
// redis.serviceRef (no per-component override) wires session-api.
func TestBuildSessionDeployment_GroupRedisServiceRef(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")
	sg.Redis = &omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: testRedisSvcName, Namespace: testRedisSvcNamespace},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args,
		"--redis-url=redis://redis.data:6379")
}

// TestBuildMemoryDeployment_GroupRedisServiceRef proves the same for memory-api.
func TestBuildMemoryDeployment_GroupRedisServiceRef(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")
	sg.Redis = &omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: testRedisSvcName},
	}

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Contains(t, dep.Spec.Template.Spec.Containers[0].Args,
		"--redis-url=redis://redis.acme-ns:6379")
}

// TestBuildSessionDeployment_PerComponentOverridesGroup proves precedence:
// session.redis wins over the group-level redis.
func TestBuildSessionDeployment_PerComponentOverridesGroup(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")
	sg.Redis = &omniav1alpha1.RedisConfig{URL: "redis://group.example.com:6379/0"}
	sg.Session = &omniav1alpha1.SessionServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "session-db"}},
		Redis:    &omniav1alpha1.RedisConfig{URL: "redis://component.example.com:6379/1"},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=redis://component.example.com:6379/1")
	assert.NotContains(t, args, "--redis-url=redis://group.example.com:6379/0")
}

// TestBuildMemoryDeployment_GroupRedisOverridesOperatorDefault proves the
// group-level redis beats the operator-wide default when no per-component
// override is set.
func TestBuildMemoryDeployment_GroupRedisOverridesOperatorDefault(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.MemoryRedisURL = testOperatorRedisURL
	sg := newTestServiceGroup("prod")
	sg.Redis = &omniav1alpha1.RedisConfig{URL: "redis://group.example.com:6379/0"}

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)
	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--redis-url=redis://group.example.com:6379/0")
	assert.NotContains(t, args, "--redis-url="+testOperatorRedisURL)
}

// TestBuildSessionDeployment_GroupRedisExistingSecret proves the group-level
// existingSecret form emits the $(REDIS_URL) placeholder + REDIS_URL env.
func TestBuildSessionDeployment_GroupRedisExistingSecret(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")
	sg.Redis = &omniav1alpha1.RedisConfig{
		ExistingSecret: &omniav1alpha1.RedisSecretRef{Name: "group-redis", Key: "url"},
	}

	dep := sb.BuildSessionDeployment("acme", "acme-ns", sg)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Args, "--redis-url=$(REDIS_URL)")

	var env *corev1.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == testRedisURLEnvName {
			env = &container.Env[i]
			break
		}
	}
	require.NotNil(t, env, "expected REDIS_URL env from group-level Secret")
	require.NotNil(t, env.ValueFrom.SecretKeyRef)
	assert.Equal(t, "group-redis", env.ValueFrom.SecretKeyRef.Name)
}

// TestBuildSessionDeployment_RollsOnGroupRedisChange proves the configHash
// flips when the group-level redis target changes, so the pod rolls.
func TestBuildSessionDeployment_RollsOnGroupRedisChange(t *testing.T) {
	sb := newTestServiceBuilder()

	sgA := newTestServiceGroup("prod")
	sgA.Redis = &omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: "redis-a"},
	}
	depA := sb.BuildSessionDeployment("acme", "acme-ns", sgA)

	sgB := newTestServiceGroup("prod")
	sgB.Redis = &omniav1alpha1.RedisConfig{
		ServiceRef: &omniav1alpha1.RedisServiceRef{Name: "redis-b"},
	}
	depB := sb.BuildSessionDeployment("acme", "acme-ns", sgB)

	annoA := depA.Spec.Template.Annotations[annotationConfigHash]
	annoB := depB.Spec.Template.Annotations[annotationConfigHash]
	assert.NotEqual(t, annoA, annoB,
		"group-level Redis change must alter the configHash so the pod rolls")
}

func TestBuildMemoryDeployment_StampsEnterpriseEnv(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enterprise bool
		want       string
	}{
		{"enabled", true, "true"},
		{"disabled", false, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sb := &ServiceBuilder{MemoryImage: "memory:test", Enterprise: tc.enterprise}
			dep := sb.BuildMemoryDeployment("ws", "ns", omniav1alpha1.WorkspaceServiceGroup{Name: "default"})
			got := deploymentEnvValue(t, dep, "ENTERPRISE_ENABLED")
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildSessionDeployment_StampsEnterpriseEnv(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enterprise bool
		want       string
	}{
		{"enabled", true, "true"},
		{"disabled", false, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sb := &ServiceBuilder{SessionImage: "session:test", Enterprise: tc.enterprise}
			dep := sb.BuildSessionDeployment("ws", "ns", omniav1alpha1.WorkspaceServiceGroup{Name: "default"})
			assert.Equal(t, tc.want, deploymentEnvValue(t, dep, "ENTERPRISE_ENABLED"))
		})
	}
}

// deploymentEnvValue returns the literal Value of the named env on the first container.
func deploymentEnvValue(t *testing.T, dep *appsv1.Deployment, name string) string {
	t.Helper()
	for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	t.Fatalf("env %q not found", name)
	return ""
}
