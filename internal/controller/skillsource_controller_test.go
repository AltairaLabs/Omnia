/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/sourcesync"
)

// skillTestScheme returns a Scheme with core v1 + core v1alpha1 types registered.
func skillTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	require.NoError(t, corev1alpha1.AddToScheme(s))
	return s
}

// makeConfigMapFetcherSource wires a SkillSource pointing at a ConfigMap
// containing a tiny bundle of SKILL.md files.
func makeConfigMapFetcherSource(t *testing.T, name, namespace, cmName string) (*corev1alpha1.SkillSource, *corev1.ConfigMap) {
	t.Helper()
	// ConfigMap fetcher wants base64-encoded binary data keyed by path.
	// The path "a__SKILL.md" becomes dir "a" containing SKILL.md after
	// decodeConfigMapKey translates "__" -> "/".
	files := map[string]string{
		"a__SKILL.md": "---\nname: alpha\ndescription: First\n---\nbody",
		"b__SKILL.md": "---\nname: beta\ndescription: Second\n---\nbody",
	}
	data := map[string][]byte{}
	for k, v := range files {
		data[k] = []byte(v)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmName,
			Namespace:       namespace,
			ResourceVersion: "1",
		},
		BinaryData: data,
	}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:     corev1alpha1.SkillSourceTypeConfigMap,
			Interval: "1h",
			ConfigMap: &corev1alpha1.ConfigMapSource{
				Name: cmName,
			},
		},
	}
	return src, cm
}

func TestSkillSourceReconcile_HappyPath_ConfigMap(t *testing.T) {
	s := skillTestScheme(t)
	src, cm := makeConfigMapFetcherSource(t, "test-src", "default", "skills-cm")

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src, cm).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()

	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		Recorder:             record.NewFakeRecorder(10),
		WorkspaceContentPath: t.TempDir(),
		MaxVersionsPerSource: 3,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-src", Namespace: "default"},
	})
	require.NoError(t, err)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "test-src", Namespace: "default"}, got))
	assert.Equal(t, corev1alpha1.SkillSourcePhaseReady, got.Status.Phase)
	assert.Equal(t, int32(2), got.Status.SkillCount)
	require.NotNil(t, got.Status.Artifact)
	assert.NotEmpty(t, got.Status.Artifact.Version)

	available := meta.FindStatusCondition(got.Status.Conditions, SkillSourceConditionSourceAvailable)
	require.NotNil(t, available)
	assert.Equal(t, metav1.ConditionTrue, available.Status)

	valid := meta.FindStatusCondition(got.Status.Conditions, SkillSourceConditionContentValid)
	require.NotNil(t, valid)
	assert.Equal(t, metav1.ConditionTrue, valid.Status)
}

func TestSkillSourceReconcile_Suspended(t *testing.T) {
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "suspended", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "1h",
			Suspend:   true,
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "anything"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(src).Build()
	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		WorkspaceContentPath: t.TempDir(),
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "suspended", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "suspended sources must not requeue")
}

func TestSkillSourceReconcile_NotFound(t *testing.T) {
	s := skillTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	r := &SkillSourceReconciler{Client: c, Scheme: s}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
}

func TestSkillSourceReconcile_InvalidInterval(t *testing.T) {
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "not-a-duration",
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "x"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()
	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		WorkspaceContentPath: t.TempDir(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad", Namespace: "default"},
	})
	// errorStatus returns nil error (only the status update error surfaces).
	require.NoError(t, err)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "bad", Namespace: "default"}, got))
	assert.Equal(t, corev1alpha1.SkillSourcePhaseError, got.Status.Phase)
}

func TestSkillSourceReconcile_ContentStorageUnavailable(t *testing.T) {
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "no-content", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "1h",
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "anything"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()
	// WorkspaceContentPath intentionally left empty — simulates chart value
	// workspaceContent.enabled=false.
	r := &SkillSourceReconciler{Client: c, Scheme: s}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "no-content", Namespace: "default"},
	})
	require.NoError(t, err)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "no-content", Namespace: "default"}, got))
	assert.Equal(t, corev1alpha1.SkillSourcePhaseError, got.Status.Phase)

	cond := meta.FindStatusCondition(got.Status.Conditions, SkillSourceConditionSourceAvailable)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, SkillSourceReasonContentStorageUnavailable, cond.Reason)
	assert.Contains(t, cond.Message, "workspaceContent.enabled=false")
}

func TestSkillSourceReconcile_MissingVariantBlock(t *testing.T) {
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-variant", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:     corev1alpha1.SkillSourceTypeGit,
			Interval: "1h",
			// Git is nil — controller should error.
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()
	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		WorkspaceContentPath: t.TempDir(),
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing-variant", Namespace: "default"},
	})
	require.NoError(t, err)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "missing-variant", Namespace: "default"}, got))
	assert.Equal(t, corev1alpha1.SkillSourcePhaseError, got.Status.Phase)
	cond := meta.FindStatusCondition(got.Status.Conditions, SkillSourceConditionSourceAvailable)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
}

// helper: assertBase64 is a sanity check for the test fixture's binary-data
// keys. Unused but kept to document the encoding convention.
var _ = base64.StdEncoding

func TestSkillSourceReconcile_ConfigMapMissing(t *testing.T) {
	// Source references a ConfigMap that doesn't exist — LatestRevision
	// (or Fetch) errors, status lands in Error phase.
	s := skillTestScheme(t)
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "src", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "1h",
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "does-not-exist"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()
	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		WorkspaceContentPath: t.TempDir(),
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "src", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, time.Minute, res.RequeueAfter)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "src", Namespace: "default"}, got))
	assert.Equal(t, corev1alpha1.SkillSourcePhaseError, got.Status.Phase)
}

func TestSkillSourceReconcile_DuplicateNames(t *testing.T) {
	// Two SKILL.md files with the same frontmatter name.
	s := skillTestScheme(t)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "dupes",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		BinaryData: map[string][]byte{
			"x__SKILL.md": []byte("---\nname: shared\ndescription: one\n---\nbody"),
			"y__SKILL.md": []byte("---\nname: shared\ndescription: two\n---\nbody"),
		},
	}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "1h",
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "dupes"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(src, cm).
		WithStatusSubresource(&corev1alpha1.SkillSource{}).
		Build()
	r := &SkillSourceReconciler{
		Client:               c,
		Scheme:               s,
		Recorder:             record.NewFakeRecorder(10),
		WorkspaceContentPath: t.TempDir(),
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "s", Namespace: "default"},
	})
	require.NoError(t, err)

	got := &corev1alpha1.SkillSource{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: "s", Namespace: "default"}, got))
	valid := meta.FindStatusCondition(got.Status.Conditions, SkillSourceConditionContentValid)
	require.NotNil(t, valid)
	assert.Equal(t, metav1.ConditionFalse, valid.Status)
	assert.Contains(t, valid.Message, "duplicate names: [shared]")
}

func TestFindDuplicateNames(t *testing.T) {
	got := findDuplicateNames([]ResolvedSkill{
		{Name: "a"}, {Name: "b"}, {Name: "a"}, {Name: "c"}, {Name: "b"},
	})
	assert.ElementsMatch(t, []string{"a", "b"}, got)
}

func TestFindDuplicateNames_Empty(t *testing.T) {
	assert.Empty(t, findDuplicateNames(nil))
}

func TestFetcherFor_Git(t *testing.T) {
	s := skillTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	r := &SkillSourceReconciler{Client: c, Scheme: s}

	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeGit,
			Git: &corev1alpha1.GitSource{
				URL: "https://example.com/repo.git",
				Ref: &corev1alpha1.GitReference{Branch: "main"},
			},
		},
	}
	f, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.NoError(t, err)
	assert.NotNil(t, f)
}

func TestFetcherFor_GitMissingSpec(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeGit},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.git")
}

func TestFetcherFor_GitWithSecret(t *testing.T) {
	s := skillTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "git-creds", Namespace: "default"},
		Data: map[string][]byte{
			"username": []byte("u"),
			"password": []byte("p"),
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	r := &SkillSourceReconciler{Client: c, Scheme: s}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeGit,
			Git: &corev1alpha1.GitSource{
				URL:       "https://example.com/repo.git",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "git-creds"},
			},
		},
	}
	f, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.NoError(t, err)
	assert.NotNil(t, f)
}

func TestFetcherFor_GitSecretMissing(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeGit,
			Git: &corev1alpha1.GitSource{
				URL:       "https://example.com/repo.git",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "missing"},
			},
		},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git credentials")
}

func TestFetcherFor_OCI(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeOCI,
			OCI:  &corev1alpha1.OCISource{URL: "oci://ghcr.io/example/skills:v1"},
		},
	}
	f, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.NoError(t, err)
	assert.NotNil(t, f)
}

func TestFetcherFor_OCIMissingSpec(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeOCI},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.oci")
}

func TestFetcherFor_OCIWithSecret(t *testing.T) {
	s := skillTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oci-creds", Namespace: "default"},
		Data: map[string][]byte{
			"username": []byte("u"),
			"password": []byte("p"),
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	r := &SkillSourceReconciler{Client: c, Scheme: s}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeOCI,
			OCI: &corev1alpha1.OCISource{
				URL:       "oci://ghcr.io/example/skills:v1",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "oci-creds"},
				Insecure:  true,
			},
		},
	}
	f, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.NoError(t, err)
	assert.NotNil(t, f)
}

func TestFetcherFor_OCISecretMissing(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "default"},
		Spec: corev1alpha1.SkillSourceSpec{
			Type: corev1alpha1.SkillSourceTypeOCI,
			OCI: &corev1alpha1.OCISource{
				URL:       "oci://ghcr.io/example/skills:v1",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "missing"},
			},
		},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oci credentials")
}

func TestFetcherFor_ConfigMapMissingSpec(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeConfigMap},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.configMap")
}

func TestFetcherFor_UnknownType(t *testing.T) {
	s := skillTestScheme(t)
	r := &SkillSourceReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build(), Scheme: s}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: "bogus"},
	}
	_, err := r.fetcherFor(context.Background(), src, sourcesync.DefaultOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown source type")
}

func TestGetWorkspaceForNamespace(t *testing.T) {
	s := skillTestScheme(t)

	t.Run("nil client", func(t *testing.T) {
		assert.Equal(t, "default", GetWorkspaceForNamespace(context.Background(), nil, "default"))
	})

	t.Run("namespace without label", func(t *testing.T) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "no-label"}}
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
		assert.Equal(t, "no-label", GetWorkspaceForNamespace(context.Background(), c, "no-label"))
	})

	t.Run("namespace with workspace label", func(t *testing.T) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "tenant-a",
				Labels: map[string]string{"omnia.altairalabs.ai/workspace": "workspace-a"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
		assert.Equal(t, "workspace-a", GetWorkspaceForNamespace(context.Background(), c, "tenant-a"))
	})

	t.Run("namespace lookup error falls back to namespace name", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(s).Build()
		assert.Equal(t, "missing", GetWorkspaceForNamespace(context.Background(), c, "missing"))
	})
}
