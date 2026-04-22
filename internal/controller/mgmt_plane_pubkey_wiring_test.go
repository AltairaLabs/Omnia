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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// These tests assert that every facade pod produced by the AgentRuntime
// deployment builder carries the plumbing PR 1b needs to load the
// mgmt-plane validator: a ConfigMap-backed volume (optional), a mount on
// the facade container, and the env var pointing at the file. Without
// all three wired together, cmd/agent silently skips validator
// construction and the dashboard debug view can't authenticate — exactly
// the "exists but not wired" failure mode CLAUDE.md flags in its
// "Wiring tests" section.

func findVolume(pod []corev1.Volume, name string) *corev1.Volume {
	for i := range pod {
		if pod[i].Name == name {
			return &pod[i]
		}
	}
	return nil
}

func findMount(mounts []corev1.VolumeMount, name string) *corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	return nil
}

func findEnv(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func TestBuildVolumes_IncludesMgmtPlanePubkey(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "workspace-a"

	volumes := r.buildVolumes(ar, newTestPromptPack(), nil)

	v := findVolume(volumes, mgmtPlanePubkeyVolumeName)
	if v == nil {
		t.Fatalf("expected volume %q on pod, got %+v", mgmtPlanePubkeyVolumeName, volumes)
	}
	if v.ConfigMap == nil {
		t.Fatal("volume must be ConfigMap-backed")
	}
	if v.ConfigMap.Name != MgmtPlanePubkeyConfigMapName {
		t.Errorf("ConfigMap name = %q, want %q", v.ConfigMap.Name, MgmtPlanePubkeyConfigMapName)
	}
	if v.ConfigMap.Optional == nil || !*v.ConfigMap.Optional {
		t.Error("ConfigMap must be optional — missing mirror shouldn't crashloop the pod")
	}
}

func TestBuildFacadeVolumeMounts_IncludesMgmtPlanePubkey(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	mounts := r.buildFacadeVolumeMounts(newTestPromptPack())

	m := findMount(mounts, mgmtPlanePubkeyVolumeName)
	if m == nil {
		t.Fatalf("expected mount %q on facade container, got %+v", mgmtPlanePubkeyVolumeName, mounts)
	}
	if m.MountPath != MgmtPlanePubkeyMountDir {
		t.Errorf("mount path = %q, want %q", m.MountPath, MgmtPlanePubkeyMountDir)
	}
	if !m.ReadOnly {
		t.Error("mount must be read-only (public key material)")
	}
}

func TestBuildFacadeEnvVars_SetsMgmtPlanePubkeyPath(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"

	envs := r.buildFacadeEnvVars(ar)

	e := findEnv(envs, EnvMgmtPlanePubkeyPath)
	if e == nil {
		t.Fatalf("expected env var %q on facade container", EnvMgmtPlanePubkeyPath)
	}
	wantVal := fmt.Sprintf("%s/%s", MgmtPlanePubkeyMountDir, MgmtPlanePubkeyDataKey)
	if e.Value != wantVal {
		t.Errorf("env value = %q, want %q", e.Value, wantVal)
	}
}
