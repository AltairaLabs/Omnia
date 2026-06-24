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
	"testing"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestBuildFacadeEnvVars_SetsPODIP verifies that buildFacadeEnvVars always
// injects POD_IP via the Downward API so the facade can build its pod-address
// hint for the blip-resume route store.
func TestBuildFacadeEnvVars_SetsPODIP(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	envs := r.buildFacadeEnvVars(ar)

	got := findEnvVar(envs, "POD_IP")
	if got == nil {
		t.Fatalf("expected POD_IP env var on facade container from Downward API")
	}
	if got.ValueFrom == nil || got.ValueFrom.FieldRef == nil {
		t.Fatalf("POD_IP must use Downward API FieldRef, got ValueFrom=%+v", got.ValueFrom)
	}
	if got.ValueFrom.FieldRef.FieldPath != "status.podIP" {
		t.Errorf("POD_IP FieldPath = %q, want %q", got.ValueFrom.FieldRef.FieldPath, "status.podIP")
	}
}

// TestBuildFacadeEnvVars_SetsRouteRedisURLFromContextStoreRef verifies that
// when spec.context is configured with a Redis store and a storeRef secret,
// buildFacadeEnvVars injects OMNIA_ROUTE_REDIS_URL from the same secret so
// the facade's blip-resume route store can connect to Redis.
func TestBuildFacadeEnvVars_SetsRouteRedisURLFromContextStoreRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Context: &omniav1alpha1.ContextConfig{
				Type: omniav1alpha1.ContextStoreTypeRedis,
				StoreRef: &corev1.LocalObjectReference{
					Name: testRedisSecretName,
				},
			},
		},
	}
	envs := r.buildFacadeEnvVars(ar)

	got := findEnvVar(envs, "OMNIA_ROUTE_REDIS_URL")
	if got == nil {
		t.Fatalf("expected OMNIA_ROUTE_REDIS_URL env var when context.type=redis and storeRef is set")
	}
	if got.ValueFrom == nil || got.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("OMNIA_ROUTE_REDIS_URL must use SecretKeyRef, got ValueFrom=%+v", got.ValueFrom)
	}
	if got.ValueFrom.SecretKeyRef.Name != testRedisSecretName {
		t.Errorf("SecretKeyRef.Name = %q, want %q", got.ValueFrom.SecretKeyRef.Name, testRedisSecretName)
	}
	if got.ValueFrom.SecretKeyRef.Key != testRedisSecretKey {
		t.Errorf("SecretKeyRef.Key = %q, want %q", got.ValueFrom.SecretKeyRef.Key, testRedisSecretKey)
	}
}

// TestBuildFacadeEnvVars_OmitsRouteRedisURLWhenNoContext verifies that when
// spec.context is nil, OMNIA_ROUTE_REDIS_URL is not injected. The facade will
// use the noop route store — correct for text-only agents without a context
// Redis secret.
func TestBuildFacadeEnvVars_OmitsRouteRedisURLWhenNoContext(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	envs := r.buildFacadeEnvVars(ar)

	if got := findEnvVar(envs, "OMNIA_ROUTE_REDIS_URL"); got != nil {
		t.Errorf("expected no OMNIA_ROUTE_REDIS_URL when session is nil, got %+v", got)
	}
}

// TestBuildFacadeEnvVars_OmitsRouteRedisURLWhenMemoryStore verifies that a
// memory-backed context store does not inject OMNIA_ROUTE_REDIS_URL. There is
// no Redis to connect to in that configuration.
func TestBuildFacadeEnvVars_OmitsRouteRedisURLWhenMemoryStore(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Context: &omniav1alpha1.ContextConfig{
				Type: omniav1alpha1.ContextStoreTypeMemory,
			},
		},
	}
	envs := r.buildFacadeEnvVars(ar)

	if got := findEnvVar(envs, "OMNIA_ROUTE_REDIS_URL"); got != nil {
		t.Errorf("expected no OMNIA_ROUTE_REDIS_URL when context.type=memory, got %+v", got)
	}
}
