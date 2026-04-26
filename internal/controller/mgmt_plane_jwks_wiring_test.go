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
	"testing"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// These tests assert that every facade pod produced by the AgentRuntime
// deployment builder is wired with the dashboard's JWKS URL when the
// reconciler is configured. Without it, cmd/agent silently skips the
// mgmt-plane validator construction and the dashboard's "Try this
// agent" debug view 401s — exactly the "exists but not wired" failure
// mode CLAUDE.md flags in its "Wiring tests" section.

func findEnvVar(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func TestBuildFacadeEnvVars_SetsJWKSURLWhenConfigured(t *testing.T) {
	const jwksURL = "http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks"
	r := &AgentRuntimeReconciler{MgmtPlaneJWKSURL: jwksURL}
	ar := &omniav1alpha1.AgentRuntime{}
	envs := r.buildFacadeEnvVars(ar)

	got := findEnvVar(envs, EnvMgmtPlaneJWKSURL)
	if got == nil {
		t.Fatalf("expected env var %q on facade container", EnvMgmtPlaneJWKSURL)
	}
	if got.Value != jwksURL {
		t.Errorf("env value = %q, want %q", got.Value, jwksURL)
	}
}

func TestBuildFacadeEnvVars_OmitsJWKSURLWhenEmpty(t *testing.T) {
	// Headless installs (Arena E2E, dashboard.enabled=false) leave the
	// URL empty. The env var must be omitted entirely so cmd/agent's
	// "env unset" path triggers — setting the var to "" would fall into
	// the JWKS resolver constructor and surface as a fetch error on
	// every JWT verification.
	r := &AgentRuntimeReconciler{MgmtPlaneJWKSURL: ""}
	ar := &omniav1alpha1.AgentRuntime{}
	envs := r.buildFacadeEnvVars(ar)

	if got := findEnvVar(envs, EnvMgmtPlaneJWKSURL); got != nil {
		t.Errorf("expected no %q env var when MgmtPlaneJWKSURL empty, got %+v", EnvMgmtPlaneJWKSURL, got)
	}
}
