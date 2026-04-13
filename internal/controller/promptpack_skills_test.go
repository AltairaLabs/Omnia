/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// makeSyncedSkillTree writes a SkillSource-shaped tree under
// root/default/default/sourceContentPath. Both workspace and namespace
// are hardcoded — tests use the same namespace consistently for clarity.
func makeSyncedSkillTree(t *testing.T, root, sourceContentPath string, skills map[string]string) {
	t.Helper()
	dst := filepath.Join(root, "default", "default", sourceContentPath)
	for name, desc := range skills {
		dir := filepath.Join(dst, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		body := "---\nname: " + name + "\ndescription: " + desc + "\nallowed-tools:\n  - tool-" + name + "\n---\nbody"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644))
	}
}

func TestResolvePromptPackSkills_Empty(t *testing.T) {
	s := skillTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
	}

	res := ResolvePromptPackSkills(context.Background(), c, pack, t.TempDir())
	assert.Empty(t, res.Manifest.Skills)
	assert.Empty(t, res.LookupErrors)
	assert.NotEmpty(t, res.Manifest.Version)
}

func TestResolvePromptPackSkills_OneSourceAllSkills(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/anthropic", map[string]string{
		"alpha": "first",
		"beta":  "second",
	})

	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "anthropic", Namespace: "default"},
		Spec:       corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeConfigMap, Interval: "1h"},
		Status: corev1alpha1.SkillSourceStatus{
			Artifact: &corev1alpha1.Artifact{ContentPath: "skills/anthropic"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "anthropic"}},
		},
	}

	res := ResolvePromptPackSkills(context.Background(), c, pack, root)
	require.Len(t, res.Manifest.Skills, 2)
	assert.Empty(t, res.LookupErrors)
	assert.Empty(t, res.CollisionErrors)

	mountPaths := []string{res.Manifest.Skills[0].MountAs, res.Manifest.Skills[1].MountAs}
	assert.Contains(t, mountPaths, "anthropic/alpha")
	assert.Contains(t, mountPaths, "anthropic/beta")

	assert.Contains(t, res.AllowedToolsBySkill["alpha"], "tool-alpha")
}

func TestResolvePromptPackSkills_IncludeFilter(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/anthropic", map[string]string{
		"keep": "kept",
		"drop": "dropped",
	})

	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "anthropic", Namespace: "default"},
		Status: corev1alpha1.SkillSourceStatus{
			Artifact: &corev1alpha1.Artifact{ContentPath: "skills/anthropic"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "anthropic", Include: []string{"keep"}}},
		},
	}
	res := ResolvePromptPackSkills(context.Background(), c, pack, root)
	require.Len(t, res.Manifest.Skills, 1)
	assert.Equal(t, "keep", res.Manifest.Skills[0].Name)
}

func TestResolvePromptPackSkills_MountAsRename(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/internal", map[string]string{
		"refund-processing": "refunds",
	})
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "internal", Namespace: "default"},
		Status: corev1alpha1.SkillSourceStatus{
			Artifact: &corev1alpha1.Artifact{ContentPath: "skills/internal"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "internal", MountAs: "billing"}},
		},
	}
	res := ResolvePromptPackSkills(context.Background(), c, pack, root)
	require.Len(t, res.Manifest.Skills, 1)
	assert.Equal(t, "billing/refund-processing", res.Manifest.Skills[0].MountAs)
}

func TestResolvePromptPackSkills_CollisionAcrossSources(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/a", map[string]string{"shared": "x"})
	makeSyncedSkillTree(t, root, "skills/b", map[string]string{"shared": "y"})

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1alpha1.SkillSource{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Status:     corev1alpha1.SkillSourceStatus{Artifact: &corev1alpha1.Artifact{ContentPath: "skills/a"}},
		},
		&corev1alpha1.SkillSource{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Status:     corev1alpha1.SkillSourceStatus{Artifact: &corev1alpha1.Artifact{ContentPath: "skills/b"}},
		},
	).Build()

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "a"}, {Source: "b"}},
		},
	}
	res := ResolvePromptPackSkills(context.Background(), c, pack, root)
	require.Len(t, res.Manifest.Skills, 2)
	require.NotEmpty(t, res.CollisionErrors)
	assert.Contains(t, res.CollisionErrors[0].Error(), "shared")
}

func TestResolvePromptPackSkills_MissingSource(t *testing.T) {
	s := skillTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "nope"}},
		},
	}
	res := ResolvePromptPackSkills(context.Background(), c, pack, t.TempDir())
	assert.Empty(t, res.Manifest.Skills)
	require.Len(t, res.LookupErrors, 1)
	assert.Contains(t, res.LookupErrors[0].Error(), "not found")
}

func TestResolvePromptPackSkills_NoSyncedArtifact(t *testing.T) {
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
		// No Status.Artifact yet.
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "pending"}},
		},
	}
	res := ResolvePromptPackSkills(context.Background(), c, pack, t.TempDir())
	require.Len(t, res.LookupErrors, 1)
	assert.Contains(t, res.LookupErrors[0].Error(), "no synced artifact")
}

func TestResolvePromptPackSkills_StableVersion(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/a", map[string]string{"x": "y"})
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Status:     corev1alpha1.SkillSourceStatus{Artifact: &corev1alpha1.Artifact{ContentPath: "skills/a"}},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "a"}},
		},
	}
	first := ResolvePromptPackSkills(context.Background(), c, pack, root)
	second := ResolvePromptPackSkills(context.Background(), c, pack, root)
	assert.Equal(t, first.Manifest.Version, second.Manifest.Version)
}

func TestValidateSkillTools(t *testing.T) {
	pack := map[string]struct{}{"tool-a": {}, "tool-b": {}}
	allowed := map[string][]string{
		"alpha": {"tool-a"},
		"beta":  {"tool-b", "ghost"},
	}
	bad := ValidateSkillTools(allowed, pack)
	assert.Equal(t, []string{"beta:ghost"}, bad)
}

func TestValidateSkillTools_AllOK(t *testing.T) {
	pack := map[string]struct{}{"x": {}, "y": {}}
	allowed := map[string][]string{"s": {"x", "y"}}
	bad := ValidateSkillTools(allowed, pack)
	assert.Empty(t, bad)
}

func TestWriteSkillManifest_Atomic(t *testing.T) {
	root := t.TempDir()
	m := &SkillManifest{
		Version: "v1",
		Skills:  []SkillManifestEntry{{MountAs: "g/s", ContentPath: "p", Name: "n"}},
	}
	require.NoError(t, WriteSkillManifest(root, "pack", m))

	target := filepath.Join(root, "manifests", "pack.json")
	data, err := os.ReadFile(target)
	require.NoError(t, err)

	var got SkillManifest
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "v1", got.Version)
	assert.Len(t, got.Skills, 1)
}

func TestHashManifest_StableAcrossOrder(t *testing.T) {
	a := &SkillManifest{
		Skills: []SkillManifestEntry{
			{MountAs: "g/x", ContentPath: "p1", Name: "x"},
			{MountAs: "g/y", ContentPath: "p2", Name: "y"},
		},
	}
	// Same content, intentionally same order — should hash identically.
	b := &SkillManifest{
		Skills: []SkillManifestEntry{
			{MountAs: "g/x", ContentPath: "p1", Name: "x"},
			{MountAs: "g/y", ContentPath: "p2", Name: "y"},
		},
	}
	assert.Equal(t, hashManifest(a), hashManifest(b))
}

func TestReconcileSkills_NoSkills(t *testing.T) {
	s := skillTestScheme(t)
	r := &PromptPackReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s, WorkspaceContentPath: t.TempDir()}
	pack := &corev1alpha1.PromptPack{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}

	r.reconcileSkills(context.Background(), pack, "{}")

	cond := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillsResolved)
	require.NotNil(t, cond)
	assert.Equal(t, "NoSkills", cond.Reason)
}

func TestReconcileSkills_NoWorkspaceContentPath(t *testing.T) {
	s := skillTestScheme(t)
	r := &PromptPackReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "anything"}},
		},
	}

	r.reconcileSkills(context.Background(), pack, "{}")

	// No-op: only the NoSkills condition is emitted.
	cond := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillsResolved)
	require.NotNil(t, cond)
	assert.Equal(t, "NoSkills", cond.Reason)
}

func TestReconcileSkills_LookupFailed(t *testing.T) {
	s := skillTestScheme(t)
	r := &PromptPackReconciler{
		Client:               fake.NewClientBuilder().WithScheme(s).Build(),
		Scheme:               s,
		WorkspaceContentPath: t.TempDir(),
	}
	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "missing"}},
		},
	}

	r.reconcileSkills(context.Background(), pack, "{}")

	resolved := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillsResolved)
	require.NotNil(t, resolved)
	assert.Equal(t, metav1.ConditionFalse, resolved.Status)
	assert.Contains(t, resolved.Message, "not found")
}

func TestReconcileSkills_HappyPathWithToolValidation(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/internal", map[string]string{
		"refunds": "Refund handling",
	})
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "internal", Namespace: "default"},
		Status:     corev1alpha1.SkillSourceStatus{Artifact: &corev1alpha1.Artifact{ContentPath: "skills/internal"}},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()
	r := &PromptPackReconciler{Client: c, Scheme: s, WorkspaceContentPath: root}

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "support", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "internal"}},
		},
	}
	// Pack declares the tool the skill needs (tool-refunds — see makeSyncedSkillTree).
	packJSON := `{"tools": {"tool-refunds": {}}}`
	r.reconcileSkills(context.Background(), pack, packJSON)

	resolved := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillsResolved)
	require.NotNil(t, resolved)
	assert.Equal(t, metav1.ConditionTrue, resolved.Status)

	valid := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillsValid)
	require.NotNil(t, valid)
	assert.Equal(t, metav1.ConditionTrue, valid.Status)

	tools := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillToolsResolved)
	require.NotNil(t, tools)
	assert.Equal(t, metav1.ConditionTrue, tools.Status)

	// Manifest should land on disk.
	_, err := os.Stat(filepath.Join(root, "default", "default", "manifests", "support.json"))
	require.NoError(t, err)
}

func TestReconcileSkills_ToolMismatch(t *testing.T) {
	s := skillTestScheme(t)
	root := t.TempDir()
	makeSyncedSkillTree(t, root, "skills/x", map[string]string{
		"alpha": "needs tool-alpha",
	})
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
		Status:     corev1alpha1.SkillSourceStatus{Artifact: &corev1alpha1.Artifact{ContentPath: "skills/x"}},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()
	r := &PromptPackReconciler{Client: c, Scheme: s, WorkspaceContentPath: root}

	pack := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "support", Namespace: "default"},
		Spec: corev1alpha1.PromptPackSpec{
			Skills: []corev1alpha1.SkillRef{{Source: "x"}},
		},
	}
	// Pack DOES NOT declare tool-alpha, so the SkillToolsResolved condition
	// should land False.
	r.reconcileSkills(context.Background(), pack, `{"tools": {}}`)

	tools := findCondition(pack.Status.Conditions, corev1alpha1.PromptPackConditionSkillToolsResolved)
	require.NotNil(t, tools)
	assert.Equal(t, metav1.ConditionFalse, tools.Status)
	assert.Contains(t, tools.Message, "alpha:tool-alpha")
}

// findCondition is a tiny helper so tests don't depend on apimachinery's meta package.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func TestExtractPackTools(t *testing.T) {
	got := ExtractPackTools(`{"tools": {"a": {"x": 1}, "b": {}}}`)
	_, hasA := got["a"]
	_, hasB := got["b"]
	assert.True(t, hasA)
	assert.True(t, hasB)
	assert.Len(t, got, 2)
}

func TestExtractPackTools_NoToolsField(t *testing.T) {
	got := ExtractPackTools(`{"version": "1.0.0"}`)
	assert.Empty(t, got)
}

func TestExtractPackTools_Malformed(t *testing.T) {
	got := ExtractPackTools(`not json`)
	assert.Nil(t, got)
}

func TestHashManifest_ChangesWithConfig(t *testing.T) {
	base := &SkillManifest{Skills: []SkillManifestEntry{{MountAs: "g/x", ContentPath: "p", Name: "x"}}}
	withCfg := &SkillManifest{
		Skills: base.Skills,
		Config: &SkillManifestConfig{MaxActive: 5, Selector: "tag"},
	}
	assert.NotEqual(t, hashManifest(base), hashManifest(withCfg))
}
