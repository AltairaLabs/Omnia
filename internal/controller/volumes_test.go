/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func minimalAgentRuntime() *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "tenant-a"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "support-pack"},
		},
	}
}

func minimalPromptPack() *omniav1alpha1.PromptPack {
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "support-pack", Namespace: "tenant-a"},
		Spec: omniav1alpha1.PromptPackSpec{
			Source: omniav1alpha1.PromptPackSource{
				Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{
					Name: "support-pack-cm",
				},
			},
			Version: "1.0.0",
		},
	}
}

// packWithSkills returns a minimal PromptPack that declares one SkillRef,
// so the workspace-content volume machinery activates.
func packWithSkills() *omniav1alpha1.PromptPack {
	p := minimalPromptPack()
	p.Spec.Skills = []omniav1alpha1.SkillRef{{Source: "my-skills"}}
	return p
}

func TestBuildVolumes_NoWorkspaceContent(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	vols := r.buildVolumes(minimalAgentRuntime(), packWithSkills(), nil)
	for _, v := range vols {
		assert.NotEqual(t, workspaceContentVolumeName, v.Name,
			"workspace-content volume must not appear when WorkspaceContentPath is empty")
	}
}

func TestBuildVolumes_PackHasNoSkills(t *testing.T) {
	// Operator configured, but the pack doesn't declare skills — mount must
	// not happen, otherwise pods peg to a PVC the cluster doesn't provision.
	r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
	vols := r.buildVolumes(minimalAgentRuntime(), minimalPromptPack(), nil)
	for _, v := range vols {
		assert.NotEqual(t, workspaceContentVolumeName, v.Name,
			"workspace-content volume must not appear when the pack declares no skills")
	}
}

func TestBuildVolumes_WithWorkspaceContentAndSkills(t *testing.T) {
	r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
	vols := r.buildVolumes(minimalAgentRuntime(), packWithSkills(), nil)
	var found *corev1.Volume
	for i := range vols {
		if vols[i].Name == workspaceContentVolumeName {
			found = &vols[i]
			break
		}
	}
	require.NotNil(t, found, "workspace-content volume must be present")
	require.NotNil(t, found.PersistentVolumeClaim)
	assert.Equal(t, "workspace-tenant-a-content", found.PersistentVolumeClaim.ClaimName)
	assert.True(t, found.PersistentVolumeClaim.ReadOnly)
}

func TestBuildRuntimeVolumeMounts_WithWorkspaceContentAndSkills(t *testing.T) {
	r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
	mounts := r.buildRuntimeVolumeMounts(minimalAgentRuntime(), packWithSkills(), nil)
	var found *corev1.VolumeMount
	for i := range mounts {
		if mounts[i].Name == workspaceContentVolumeName {
			found = &mounts[i]
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, workspaceContentMountPath, found.MountPath)
	assert.True(t, found.ReadOnly)
}

func TestBuildRuntimeVolumeMounts_NoWorkspaceContent(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	mounts := r.buildRuntimeVolumeMounts(minimalAgentRuntime(), packWithSkills(), nil)
	for _, m := range mounts {
		assert.NotEqual(t, workspaceContentVolumeName, m.Name)
	}
}

func TestSkillsEnabled(t *testing.T) {
	t.Run("unconfigured operator", func(t *testing.T) {
		r := &AgentRuntimeReconciler{}
		assert.False(t, r.skillsEnabled(packWithSkills()))
	})
	t.Run("nil pack", func(t *testing.T) {
		r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
		assert.False(t, r.skillsEnabled(nil))
	})
	t.Run("pack without skills", func(t *testing.T) {
		r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
		assert.False(t, r.skillsEnabled(minimalPromptPack()))
	})
	t.Run("both configured", func(t *testing.T) {
		r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
		assert.True(t, r.skillsEnabled(packWithSkills()))
	})
}

func TestSkillManifestPath(t *testing.T) {
	t.Run("empty when WorkspaceContentPath is unset", func(t *testing.T) {
		r := &AgentRuntimeReconciler{}
		assert.Equal(t, "", r.skillManifestPath("support-pack"))
	})

	t.Run("returns mount-relative path when set", func(t *testing.T) {
		r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
		got := r.skillManifestPath("support-pack")
		assert.Equal(t, "/workspace-content/manifests/support-pack.json", got)
	})
}
