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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// Internal service-to-service auth (SEC-1/SEC-5). When enabled, the
// operator-managed session-api requires every caller to present an
// audience-bound ServiceAccount bearer token, validated server-side via the
// TokenReview API against an allowlist of caller subjects. These constants and
// helpers are shared by the per-workspace session-api Deployment builder
// (ServiceBuilder) and the AgentRuntime / eval-worker caller builders.
const (
	// envSessionAPIAuthEnabled toggles auth on the session-api binary.
	envSessionAPIAuthEnabled = "SESSION_API_AUTH_ENABLED"
	// envSessionAPIAuthAllowedSubjects is the comma-separated exact-match caller
	// allowlist (cross-namespace callers, e.g. the dashboard).
	envSessionAPIAuthAllowedSubjects = "SESSION_API_AUTH_ALLOWED_SUBJECTS"
	// envSessionAPIAuthAllowedNamespaces is the comma-separated trusted-namespace
	// allowlist. Any ServiceAccount in one of these namespaces is accepted; this
	// is how the per-AgentRuntime facade SAs (which come and go independently of
	// the Workspace) are trusted without enumerating each one.
	envSessionAPIAuthAllowedNamespaces = "SESSION_API_AUTH_ALLOWED_NAMESPACES"
	// envSessionAPIAuthAudiences is the comma-separated accepted audiences.
	envSessionAPIAuthAudiences = "SESSION_API_AUTH_AUDIENCES"
	// envSessionAPITokenPath tells a caller where its projected token lives.
	envSessionAPITokenPath = "SESSION_API_TOKEN_PATH"

	// sessionAuthTokenVolumeName is the projected SA-token volume name on caller pods.
	sessionAuthTokenVolumeName = "session-api-token"
	// sessionAuthTokenMountDir is the directory the projected token is mounted at.
	sessionAuthTokenMountDir = "/var/run/secrets/omnia/session-api"
	// sessionAuthTokenFileName is the projected token file name.
	sessionAuthTokenFileName = "token"
)

// sessionAuthTokenPath returns the full caller-side path to the projected token.
func sessionAuthTokenPath() string {
	return sessionAuthTokenMountDir + "/" + sessionAuthTokenFileName
}

// ServiceAuthConfig carries the internal-service-auth settings threaded from
// the operator flags into both ServiceBuilder (session-api server side) and
// AgentRuntimeReconciler (facade / eval-worker caller side).
type ServiceAuthConfig struct {
	// Enabled gates all of the below. When false every helper is a no-op so
	// rendering is identical to the pre-feature behaviour.
	Enabled bool
	// Audience is bound into caller projected tokens and enforced by
	// session-api (--auth-audiences). Empty audience means session-api
	// accepts the cluster default audience.
	Audience string
	// TokenExpirationSeconds is the projected token expiry. Defaults to 3600
	// when zero.
	TokenExpirationSeconds int64
	// IstioMTLS additionally provisions a STRICT PeerAuthentication for
	// session-api/memory-api in each workspace namespace.
	IstioMTLS bool
	// ExtraSubjects are caller subjects added to the session-api allowlist
	// beyond the per-workspace SAs the operator creates (e.g. the dashboard
	// SA, which the chart owns).
	ExtraSubjects []string
}

// expirationSeconds returns the configured token expiry, defaulting to 3600.
func (c ServiceAuthConfig) expirationSeconds() int64 {
	if c.TokenExpirationSeconds <= 0 {
		return 3600
	}
	return c.TokenExpirationSeconds
}

// subjectFor renders the canonical ServiceAccount subject string for a SA.
func subjectFor(namespace, name string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, name)
}

// allowedSubjectsFor computes the session-api EXACT-match subject allowlist for
// a workspace's service group. This carries cross-namespace callers only — the
// chart-supplied extras (e.g. the dashboard SA, which lives in the operator /
// release namespace, not the workspace namespace).
//
// In-workspace callers (the per-AgentRuntime facade SAs, memory-api,
// eval-worker) are NOT listed here: they are authorized by namespace via
// allowedNamespacesFor. The per-AgentRuntime facade SAs in particular cannot be
// enumerated at session-api Deployment build time (facades come and go
// independently of the Workspace), which is exactly why the namespace allow
// exists. The deterministic memory-api / eval-worker subjects are still added
// for defence in depth (they're in the workspace namespace too, so the
// namespace allow would already cover them). See SERVICE.md for the trust model.
func (c ServiceAuthConfig) allowedSubjectsFor(workspaceName, groupName, namespace string) []string {
	subjects := make([]string, 0, 4+len(c.ExtraSubjects))
	// Co-located memory-api caller (emits provider_usage to session-api).
	subjects = append(subjects, subjectFor(namespace, fmt.Sprintf("memory-%s-%s", workspaceName, groupName)))
	// Per-group eval-worker caller.
	subjects = append(subjects, subjectFor(namespace, evalWorkerName(groupName)))
	subjects = append(subjects, c.ExtraSubjects...)
	return dedupeNonEmpty(subjects)
}

// allowedNamespacesFor computes the session-api trusted-namespace allowlist for
// a workspace's service group: the workspace namespace, where the facade,
// memory-api and eval-worker pods run. Any ServiceAccount in this namespace is
// accepted by session-api, so the per-AgentRuntime facade SAs (which are created
// and destroyed as AgentRuntimes scale) pass auth without the operator having to
// enumerate each one in the subject allowlist.
func (c ServiceAuthConfig) allowedNamespacesFor(namespace string) []string {
	return dedupeNonEmpty([]string{namespace})
}

// dedupeNonEmpty returns the input with empty strings and duplicates removed,
// order-preserving, so the allowlist is stable across reconciles.
func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// applySessionAPIServerAuthEnv sets the SESSION_API_AUTH_* env on a session-api
// Deployment so its binary enforces ServiceAccount auth against the allowlist.
// No-op when auth is disabled.
func (c ServiceAuthConfig) applySessionAPIServerAuthEnv(dep *appsv1.Deployment, workspaceName, groupName, namespace string) {
	if !c.Enabled {
		return
	}
	env := []corev1.EnvVar{
		{Name: envSessionAPIAuthEnabled, Value: "true"},
		{Name: envSessionAPIAuthAllowedSubjects, Value: strings.Join(c.allowedSubjectsFor(workspaceName, groupName, namespace), ",")},
		{Name: envSessionAPIAuthAllowedNamespaces, Value: strings.Join(c.allowedNamespacesFor(namespace), ",")},
	}
	if c.Audience != "" {
		env = append(env, corev1.EnvVar{Name: envSessionAPIAuthAudiences, Value: c.Audience})
	}
	containers := dep.Spec.Template.Spec.Containers
	for i := range containers {
		containers[i].Env = append(containers[i].Env, env...)
	}
}

// callerTokenVolume returns the projected SA-token volume a caller pod mounts.
func (c ServiceAuthConfig) callerTokenVolume() corev1.Volume {
	return corev1.Volume{
		Name: sessionAuthTokenVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Path:              sessionAuthTokenFileName,
							Audience:          c.Audience,
							ExpirationSeconds: ptr.To(c.expirationSeconds()),
						},
					},
				},
			},
		},
	}
}

// callerTokenMount returns the read-only volume mount for the projected token.
func (c ServiceAuthConfig) callerTokenMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      sessionAuthTokenVolumeName,
		MountPath: sessionAuthTokenMountDir,
		ReadOnly:  true,
	}
}

// applyCallerToken adds the projected token volume to the pod, mounts it on
// every container, and sets SESSION_API_TOKEN_PATH so the session-api HTTP
// client (and dashboard proxy, memory-api emitter, eval-worker) reads it. The
// session-api token source caches+rotates the file. No-op when disabled.
func (c ServiceAuthConfig) applyCallerToken(spec *corev1.PodSpec) {
	if !c.Enabled {
		return
	}
	spec.Volumes = append(spec.Volumes, c.callerTokenVolume())
	mount := c.callerTokenMount()
	pathEnv := corev1.EnvVar{Name: envSessionAPITokenPath, Value: sessionAuthTokenPath()}
	for i := range spec.Containers {
		spec.Containers[i].VolumeMounts = append(spec.Containers[i].VolumeMounts, mount)
		spec.Containers[i].Env = append(spec.Containers[i].Env, pathEnv)
	}
}
