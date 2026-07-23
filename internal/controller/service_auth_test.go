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
)

const (
	testAudience              = "omnia-session-api"
	testAuthNS                = "acme-ns"
	testServiceAuthNetpolName = "service-auth-acme"    // NetworkPolicy/PeerAuthentication name for workspace "acme"
	testSessionSAName         = "session-acme-default" // session-api ServiceAccount in the "acme" workspace
	testMemorySAName          = "memory-acme-default"  // memory-api ServiceAccount in the "acme" workspace
)

func enabledAuth() ServiceAuthConfig {
	return ServiceAuthConfig{
		Enabled:                true,
		Audience:               testAudience,
		TokenExpirationSeconds: 1800,
		ExtraSubjects:          []string{"system:serviceaccount:omnia-system:omnia-dashboard"},
	}
}

func TestServiceAuth_ExpirationSecondsDefault(t *testing.T) {
	assert.Equal(t, int64(3600), ServiceAuthConfig{}.expirationSeconds())
	assert.Equal(t, int64(3600), ServiceAuthConfig{TokenExpirationSeconds: -5}.expirationSeconds())
	assert.Equal(t, int64(1800), ServiceAuthConfig{TokenExpirationSeconds: 1800}.expirationSeconds())
}

func TestServiceAuth_SubjectFor(t *testing.T) {
	assert.Equal(t, "system:serviceaccount:ns:sa", subjectFor("ns", "sa"))
}

func TestServiceAuth_DedupeNonEmpty(t *testing.T) {
	in := []string{"a", "", "  b ", "a", "c", "b"}
	assert.Equal(t, []string{"a", "b", "c"}, dedupeNonEmpty(in))
}

func TestServiceAuth_AllowedSubjectsFor(t *testing.T) {
	c := enabledAuth()
	got := c.allowedSubjectsFor("acme", "default", testAuthNS)

	// memory-api + eval-worker caller SAs + the dashboard extra, deduped.
	assert.Contains(t, got, "system:serviceaccount:acme-ns:memory-acme-default")
	assert.Contains(t, got, subjectFor(testAuthNS, evalWorkerName("default")))
	assert.Contains(t, got, "system:serviceaccount:omnia-system:omnia-dashboard")
}

func TestServiceAuth_AllowedNamespacesFor(t *testing.T) {
	c := enabledAuth()
	got := c.allowedNamespacesFor(testAuthNS)

	// The workspace namespace is trusted, so facade / memory-api / eval-worker
	// SAs there are authorized without being enumerated as subjects.
	assert.Equal(t, []string{testAuthNS}, got)
}

func TestServiceAuth_ApplyServerEnv_Disabled(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")
	dep := sb.BuildSessionDeployment("acme", testAuthNS, sg)

	for _, c := range dep.Spec.Template.Spec.Containers {
		for _, e := range c.Env {
			assert.NotEqual(t, envSessionAPIAuthEnabled, e.Name,
				"auth env must not be set when disabled")
		}
	}
}

func TestServiceAuth_ApplyServerEnv_Enabled(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.ServiceAuth = enabledAuth()
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("acme", testAuthNS, sg)

	env := envMap(dep.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, "true", env[envSessionAPIAuthEnabled])
	assert.Equal(t, testAudience, env[envSessionAPIAuthAudiences])
	require.Contains(t, env, envSessionAPIAuthAllowedSubjects)
	subs := strings.Split(env[envSessionAPIAuthAllowedSubjects], ",")
	assert.Contains(t, subs, "system:serviceaccount:acme-ns:memory-acme-default")
	assert.Contains(t, subs, "system:serviceaccount:omnia-system:omnia-dashboard")

	// The workspace namespace is set as a trusted namespace so facade SAs pass.
	require.Contains(t, env, envSessionAPIAuthAllowedNamespaces)
	nss := strings.Split(env[envSessionAPIAuthAllowedNamespaces], ",")
	assert.Contains(t, nss, testAuthNS)
}

func TestServiceAuth_ApplyServerEnv_NoAudienceOmitsEnv(t *testing.T) {
	sb := newTestServiceBuilder()
	auth := enabledAuth()
	auth.Audience = ""
	sb.ServiceAuth = auth
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("acme", testAuthNS, sg)
	env := envMap(dep.Spec.Template.Spec.Containers[0].Env)
	assert.NotContains(t, env, envSessionAPIAuthAudiences)
	assert.Equal(t, "true", env[envSessionAPIAuthEnabled])
}

func TestServiceAuth_MemoryCallerToken_Disabled(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")
	dep := sb.BuildMemoryDeployment("acme", testAuthNS, sg)

	assert.False(t, hasVolume(dep.Spec.Template.Spec.Volumes, sessionAuthTokenVolumeName))
}

// TestServiceAuth_MemoryServerEnv_Enabled proves memory-api joins the shared
// SESSION_API_AUTH_* data-plane auth: the operator stamps the same server-auth
// env onto the memory-api deployment it stamps onto session-api and privacy-api.
func TestServiceAuth_MemoryServerEnv_Enabled(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.ServiceAuth = enabledAuth()
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", testAuthNS, sg)
	env := envMap(dep.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, "true", env[envSessionAPIAuthEnabled])
	require.Contains(t, env, envSessionAPIAuthAllowedNamespaces)
	assert.Contains(t, strings.Split(env[envSessionAPIAuthAllowedNamespaces], ","), testAuthNS)
}

func TestServiceAuth_MemoryCallerToken_Enabled(t *testing.T) {
	sb := newTestServiceBuilder()
	sb.ServiceAuth = enabledAuth()
	sg := newTestServiceGroup("default")

	dep := sb.BuildMemoryDeployment("acme", testAuthNS, sg)
	spec := dep.Spec.Template.Spec

	// Projected token volume present with the configured audience + expiry.
	vol := findVolume(t, spec.Volumes, sessionAuthTokenVolumeName)
	require.NotNil(t, vol.Projected)
	require.Len(t, vol.Projected.Sources, 1)
	sat := vol.Projected.Sources[0].ServiceAccountToken
	require.NotNil(t, sat)
	assert.Equal(t, testAudience, sat.Audience)
	assert.Equal(t, sessionAuthTokenFileName, sat.Path)
	require.NotNil(t, sat.ExpirationSeconds)
	assert.Equal(t, int64(1800), *sat.ExpirationSeconds)

	// Every container mounts it + gets SESSION_API_TOKEN_PATH.
	for _, c := range spec.Containers {
		assert.True(t, hasMount(c.VolumeMounts, sessionAuthTokenVolumeName), c.Name)
		assert.Equal(t, sessionAuthTokenPath(), envMap(c.Env)[envSessionAPITokenPath], c.Name)
	}
}

func TestServiceAuth_TokenPath(t *testing.T) {
	assert.Equal(t, "/var/run/secrets/omnia/session-api/token", sessionAuthTokenPath())
}

func TestServiceAuth_ApplyCallerToken_DisabledIsNoop(t *testing.T) {
	spec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}
	ServiceAuthConfig{Enabled: false}.applyCallerToken(spec)
	assert.Empty(t, spec.Volumes)
	assert.Empty(t, spec.Containers[0].VolumeMounts)
	assert.Empty(t, spec.Containers[0].Env)
}

// --- test helpers ---

func envMap(env []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		m[e.Name] = e.Value
	}
	return m
}

func hasVolume(vols []corev1.Volume, name string) bool {
	for _, v := range vols {
		if v.Name == name {
			return true
		}
	}
	return false
}

func findVolume(t *testing.T, vols []corev1.Volume, name string) corev1.Volume {
	t.Helper()
	for _, v := range vols {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("volume %q not found", name)
	return corev1.Volume{}
}

func hasMount(mounts []corev1.VolumeMount, name string) bool {
	for _, m := range mounts {
		if m.Name == name {
			return true
		}
	}
	return false
}

func findMount(t *testing.T, mounts []corev1.VolumeMount, name string) corev1.VolumeMount {
	t.Helper()
	for _, m := range mounts {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("mount %q not found", name)
	return corev1.VolumeMount{}
}
