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
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/podoverrides"
)

const (
	servicePort = 8080
	healthPort  = 8081
)

// Label key constants for service groups.
const (
	labelComponent    = "app.kubernetes.io/component"
	labelServiceGroup = "omnia.altairalabs.ai/service-group"
	// labelWorkspace is already defined in workspace_controller.go
)

// ServiceBuilder builds Deployment and Service objects for per-workspace
// session-api and memory-api instances.
type ServiceBuilder struct {
	SessionImage           string
	SessionImagePullPolicy corev1.PullPolicy
	MemoryImage            string
	MemoryImagePullPolicy  corev1.PullPolicy

	// MemoryRedisURL is the operator-wide Redis target threaded into
	// every per-workspace memory-api Deployment as --redis-url. Empty
	// disables both the read-through cache and the event publisher.
	// One Redis is shared across workspaces; per-workspace caching is
	// already namespaced by the scope-hash inside CachedStore.
	//
	// Accepts the literal placeholder "$(REDIS_URL)" — when the chart
	// passes this, ServiceBuilder also mounts the corresponding Secret
	// as REDIS_URL env on every memory-api pod so Kubernetes env
	// expansion fills it at startup. The Secret reference is supplied
	// via MemoryRedisURLSecret.
	MemoryRedisURL string

	// MemoryRedisURLSecret references the Kubernetes Secret holding
	// the actual Redis URL when MemoryRedisURL is the placeholder
	// "$(REDIS_URL)". When set, ServiceBuilder mounts it as a REDIS_URL
	// env on every per-workspace memory-api pod. Empty means
	// MemoryRedisURL is a literal value and no env mount is needed.
	MemoryRedisURLSecret SecretKeyRef

	// MemoryCacheTTL is forwarded to memory-api as --cache-ttl. Empty
	// or "0" disables the cache even when MemoryRedisURL is set, so
	// operators can stand up Redis (for the event publisher) without
	// the cache, or vice-versa.
	MemoryCacheTTL string
}

// SecretKeyRef points at a single key within a Kubernetes Secret. Used
// for mounting Redis URL secrets without coupling ServiceBuilder to
// corev1.SecretKeySelector everywhere.
type SecretKeyRef struct {
	Name string
	Key  string
}

// BuildSessionDeployment builds a Deployment for the session-api service group.
func (sb *ServiceBuilder) BuildSessionDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("session-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("session-api", workspaceName, sg.Name)
	args := []string{
		fmt.Sprintf("--workspace=%s", workspaceName),
		fmt.Sprintf("--service-group=%s", sg.Name),
	}
	var overrides *omniav1alpha1.PodOverrides
	if sg.Session != nil {
		overrides = sg.Session.PodOverrides
	}
	return buildServiceDeployment(name, namespace, sb.SessionImage, sb.SessionImagePullPolicy, args, labels, overrides)
}

// Default worker intervals for the operator-managed memory-api. Each
// memory-api flag for a worker defaults to empty (= disabled) on the binary
// side, so an operator that never passes them silently runs no workers. We
// pass safe defaults here so a fresh memory-api Deployment runs every worker
// out of the box; advanced operators can swap MemoryPolicy in later for
// per-workspace tuning.
//
// These defaults were chosen so behaviour matches the spec promises (the
// "compaction worker", "tombstone GC worker", and "re-embed backfill worker"
// described in agentic-memory-design.md and memory-retention-and-pruning-
// proposal.md) without requiring a MemoryPolicy CRD for basic operation.
//
// Re-embed runs frequently because it's cheap when there's nothing to do;
// compaction and tombstone GC walk the table so they run daily.
const (
	defaultMemoryCompactionInterval = "24h"
	defaultMemoryTombstoneInterval  = "24h"
	defaultMemoryReembedInterval    = "60m"
)

// BuildMemoryDeployment builds a Deployment for the memory-api service group.
//
// The pod template carries:
//   - POD_NAMESPACE (downward API) so memory-api's embedding-provider
//     lookup defaults to the workspace namespace where the Provider CRD
//     and its Secret live, instead of falling back to omnia-system.
//   - A configHash annotation derived from sg.Memory so a workspace
//     change (e.g. switching memory.providerRef) rolls the pod —
//     memory-api reads its config once at startup, so without this the
//     change is invisible until something else triggers a roll.
//
// Args bridge the Workspace's MemoryServiceConfig to memory-api flags. Until
// this PR the operator only passed --workspace + --service-group, which left
// every worker disabled and the embedding provider unconfigured even when
// MemoryServiceConfig.ProviderRef was set — a wiring gap that hid behind
// "the workers are implemented" status reports while none of them ran. See
// issue #1038.
func (sb *ServiceBuilder) BuildMemoryDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("memory-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("memory-api", workspaceName, sg.Name)
	// Resolve Redis: per-workspace override wins over operator-wide.
	// Either form may be empty (memory-api then runs without Redis,
	// cache + event publisher disabled) — that's a valid config.
	redisURL, redisSecret := resolveMemoryRedis(sg, sb.MemoryRedisURL, sb.MemoryRedisURLSecret)
	args := buildMemoryAPIArgs(workspaceName, sg, redisURL, sb.MemoryCacheTTL)
	var overrides *omniav1alpha1.PodOverrides
	if sg.Memory != nil {
		overrides = sg.Memory.PodOverrides
	}
	dep := buildServiceDeployment(name, namespace, sb.MemoryImage, sb.MemoryImagePullPolicy, args, labels, overrides)
	addPodNamespaceEnv(dep)
	addMemoryRedisURLEnv(dep, redisSecret)
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[annotationConfigHash] = memoryConfigHash(sg)
	return dep
}

// resolveMemoryRedis returns the Redis URL string and (optional)
// Secret reference for a per-workspace memory-api Deployment.
// Per-workspace `sg.Memory.Redis` overrides the operator-wide default.
//
// Output forms (matches the chart-side resolution):
//
//   - existingSecret → ("$(REDIS_URL)" placeholder, secret ref); the
//     ServiceBuilder mounts REDIS_URL env from the Secret on the pod
//     so Kubernetes env expansion fills the placeholder at startup.
//   - url            → (literal URL, empty secret ref).
//   - host           → (synthesised plaintext URL, empty secret ref).
//   - empty          → ("", empty); operator default is used as the
//     fallback (still honoured via the URL it carries).
func resolveMemoryRedis(sg omniav1alpha1.WorkspaceServiceGroup, defaultURL string, defaultSecret SecretKeyRef) (string, SecretKeyRef) {
	if sg.Memory == nil || sg.Memory.Redis == nil {
		return defaultURL, defaultSecret
	}
	r := sg.Memory.Redis
	if r.ExistingSecret != nil && r.ExistingSecret.Name != "" && r.ExistingSecret.Key != "" {
		return "$(REDIS_URL)", SecretKeyRef{Name: r.ExistingSecret.Name, Key: r.ExistingSecret.Key}
	}
	if r.URL != "" {
		return r.URL, SecretKeyRef{}
	}
	if r.Host != "" {
		port := int32(6379)
		if r.Port != 0 {
			port = r.Port
		}
		userPart := ""
		if r.User != "" {
			userPart = r.User + "@"
		}
		return fmt.Sprintf("redis://%s%s:%d/%d", userPart, r.Host, port, r.DB), SecretKeyRef{}
	}
	// Workspace's Redis block is set but with no input form populated —
	// CEL validation should have rejected this at admission. Fall back
	// to the operator default rather than break the Deployment.
	return defaultURL, defaultSecret
}

// addMemoryRedisURLEnv mounts REDIS_URL from a Secret when the operator
// was configured with the placeholder + Secret reference. Memory-api's
// --redis-url flag arrives at the binary as the literal string
// "$(REDIS_URL)"; Kubernetes env expansion replaces it at container
// startup. No-op when ref is empty (either no Redis configured, or the
// URL was passed as a literal in chart values).
func addMemoryRedisURLEnv(dep *appsv1.Deployment, ref SecretKeyRef) {
	if ref.Name == "" || ref.Key == "" {
		return
	}
	containers := dep.Spec.Template.Spec.Containers
	for i := range containers {
		containers[i].Env = append(containers[i].Env, corev1.EnvVar{
			Name: "REDIS_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
					Key:                  ref.Key,
				},
			},
		})
	}
}

// buildMemoryAPIArgs assembles the CLI args passed to memory-api. The operator
// is the canonical source of these because each one is a wiring boundary
// crossing — the binary's flag default is "off", so anything the operator
// doesn't pass silently doesn't run.
//
// redisURL and cacheTTL come from the operator (chart-driven, not per-
// workspace) because Redis is a shared cluster service. The URL may be
// the literal placeholder "$(REDIS_URL)" — Kubernetes env expansion at
// the memory-api pod fills it from the REDIS_URL Secret env that
// addMemoryRedisURLEnv mounts.
func buildMemoryAPIArgs(workspaceName string, sg omniav1alpha1.WorkspaceServiceGroup, redisURL, cacheTTL string) []string {
	args := []string{
		fmt.Sprintf("--workspace=%s", workspaceName),
		fmt.Sprintf("--service-group=%s", sg.Name),
		fmt.Sprintf("--compaction-interval=%s", defaultMemoryCompactionInterval),
		fmt.Sprintf("--tombstone-interval=%s", defaultMemoryTombstoneInterval),
		fmt.Sprintf("--reembed-interval=%s", defaultMemoryReembedInterval),
	}
	if sg.Memory != nil && sg.Memory.ProviderRef != nil && sg.Memory.ProviderRef.Name != "" {
		args = append(args, fmt.Sprintf("--embedding-provider=%s", sg.Memory.ProviderRef.Name))
	}
	if redisURL != "" {
		args = append(args, fmt.Sprintf("--redis-url=%s", redisURL))
	}
	if cacheTTL != "" {
		args = append(args, fmt.Sprintf("--cache-ttl=%s", cacheTTL))
	}
	return args
}

// addPodNamespaceEnv injects POD_NAMESPACE (sourced from the downward
// API) into every container in the pod template. memory-api / session-
// api use it to scope cross-namespace lookups (Provider CRD, Secret).
func addPodNamespaceEnv(dep *appsv1.Deployment) {
	containers := dep.Spec.Template.Spec.Containers
	for i := range containers {
		containers[i].Env = append(containers[i].Env, corev1.EnvVar{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		})
	}
}

// memoryConfigHash hashes the runtime-relevant fields of a memory
// service group (database SecretRef name + embedding provider Ref name
// + per-workspace Redis target) into a short stable string. Hashing
// only the bits the memory-api actually reads at startup keeps
// cosmetic edits — labels, pod overrides, comments — from rolling
// the pod.
func memoryConfigHash(sg omniav1alpha1.WorkspaceServiceGroup) string {
	var dbSecret, providerRef, redisDescriptor string
	if sg.Memory != nil {
		if sg.Memory.Database.SecretRef.Name != "" {
			dbSecret = sg.Memory.Database.SecretRef.Name
		}
		if sg.Memory.ProviderRef != nil {
			providerRef = sg.Memory.ProviderRef.Name
		}
		redisDescriptor = redisHashDescriptor(sg.Memory.Redis)
	}
	sum := sha256.Sum256([]byte(dbSecret + "|" + providerRef + "|" + redisDescriptor))
	return hex.EncodeToString(sum[:8])
}

// redisHashDescriptor produces a short stable string covering the
// runtime-relevant fields of a RedisConfig. Used by memoryConfigHash
// so flipping a workspace's Redis target rolls its memory-api pod.
func redisHashDescriptor(r *omniav1alpha1.RedisConfig) string {
	if r == nil {
		return ""
	}
	if r.ExistingSecret != nil {
		return fmt.Sprintf("secret:%s/%s", r.ExistingSecret.Name, r.ExistingSecret.Key)
	}
	if r.URL != "" {
		return "url:" + r.URL
	}
	if r.Host != "" {
		return fmt.Sprintf("host:%s:%d/%d:%s", r.Host, r.Port, r.DB, r.User)
	}
	return ""
}

// BuildService builds a ClusterIP Service for the given component.
func BuildService(name, namespace, component, workspaceName, groupName string) *corev1.Service {
	labels := serviceLabels(component, workspaceName, groupName)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       servicePort,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// serviceAccountName returns the ServiceAccount name for a per-workspace service deployment.
func serviceAccountName(deploymentName string) string {
	return deploymentName
}

// ServiceURL returns the in-cluster HTTP URL for the given service.
func ServiceURL(serviceName, namespace string) string {
	return fmt.Sprintf("http://%s.%s:%d", serviceName, namespace, servicePort)
}

// buildServiceDeployment constructs a Deployment with restricted security context,
// standard health probes, and the given image and args. If overrides is non-nil,
// pod-level and container-level fields from PodOverrides are merged in.
func buildServiceDeployment(
	name, namespace, image string,
	pullPolicy corev1.PullPolicy,
	args []string,
	labels map[string]string,
	overrides *omniav1alpha1.PodOverrides,
) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName(name),
					Containers: []corev1.Container{
						{
							Name:            name,
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Args:            args,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: servicePort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "health",
									ContainerPort: healthPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe:   httpProbe(healthzPath, healthPort),
							ReadinessProbe:  httpProbe("/readyz", healthPort),
							SecurityContext: restrictedSecurityContext(),
						},
					},
				},
			},
		},
	}

	if overrides != nil {
		podoverrides.ApplyPod(&dep.Spec.Template.Spec, &dep.Spec.Template.ObjectMeta, overrides)
		for i := range dep.Spec.Template.Spec.Containers {
			podoverrides.ApplyContainer(&dep.Spec.Template.Spec.Containers[i], overrides)
		}
	}

	return dep
}

// serviceLabels returns the standard label set for a service group component.
func serviceLabels(component, workspaceName, groupName string) map[string]string {
	return map[string]string{
		labelComponent:    component,
		labelAppManagedBy: labelValueOmniaOperator,
		labelWorkspace:    workspaceName,
		labelServiceGroup: groupName,
	}
}

// httpProbe returns an HTTP GET probe for the given path and port.
func httpProbe(path string, port int) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(int32(port)),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}
}

// restrictedSecurityContext returns a PodSecurity restricted-compliant SecurityContext.
func restrictedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
