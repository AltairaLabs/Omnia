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

package promptpack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	return scheme
}

// configmapPack builds a PromptPack whose object name (objName, e.g. the
// deterministic pp-<hash> form) DIFFERS from its logical packName — the core
// regression this resolver must handle. It carries the packName label stamped
// by the operator and the packName/version identity fields.
func configmapPack(objName, packName, version, cmName string) *omniav1alpha1.PromptPack {
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objName,
			Namespace: "ns",
			Labels:    map[string]string{packNameLabel: packName},
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: packName,
			Version:  version,
			Source: omniav1alpha1.PromptPackContentSource{
				Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: cmName},
			},
		},
	}
}

// TestLoad_ResolvesByLabelAndVersion is the core regression: the PromptPack's
// object name (pp-abc123) differs from its logical packName (mypack). A
// resolver that Gets by name would 404; it must List by the packName label
// and select the matching version.
func TestLoad_ResolvesByLabelAndVersion(t *testing.T) {
	pp := configmapPack("pp-abc123", "mypack", "1.2.0", "mypack-cm")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "mypack-cm", Namespace: "ns"},
		Data:       map[string]string{packJSONKey: `{"id":"mypack","version":"1.2.0"}`},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp, cm).Build()

	raw, err := NewResolver(c).Load(context.Background(), "ns", "mypack", "1.2.0")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"mypack","version":"1.2.0"}`, string(raw))
}

func TestLoad_VersionNotFound(t *testing.T) {
	pp := configmapPack("pp-abc123", "mypack", "1.2.0", "mypack-cm")
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()

	_, err := NewResolver(c).Load(context.Background(), "ns", "mypack", "9.9.9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mypack")
	assert.Contains(t, err.Error(), "9.9.9")
}

// TestLoad_EmptyVersionDefaultsToStableMax proves the empty-version case
// (channel default) picks the highest stable semver among multiple versions
// of the same pack, matching the operator's stable-channel default.
func TestLoad_EmptyVersionDefaultsToStableMax(t *testing.T) {
	old := configmapPack("pp-old123", "mypack", "1.2.0", "mypack-cm-old")
	newer := configmapPack("pp-new456", "mypack", "1.3.0", "mypack-cm-new")
	cmOld := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "mypack-cm-old", Namespace: "ns"},
		Data:       map[string]string{packJSONKey: `{"id":"mypack","version":"1.2.0"}`},
	}
	cmNew := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "mypack-cm-new", Namespace: "ns"},
		Data:       map[string]string{packJSONKey: `{"id":"mypack","version":"1.3.0"}`},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(old, newer, cmOld, cmNew).Build()

	raw, err := NewResolver(c).Load(context.Background(), "ns", "mypack", "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"mypack","version":"1.3.0"}`, string(raw))
}

// TestLoad_VPrefixVersionMatches proves a v-prefixed spec.version ("v1.2.0")
// still matches a caller-supplied unprefixed version ("1.2.0"), mirroring the
// operator's strip-"v"-then-strict-semver comparison rule.
func TestLoad_VPrefixVersionMatches(t *testing.T) {
	pp := configmapPack("pp-abc123", "mypack", "v1.2.0", "mypack-cm")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "mypack-cm", Namespace: "ns"},
		Data:       map[string]string{packJSONKey: `{"id":"mypack","version":"v1.2.0"}`},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp, cm).Build()

	raw, err := NewResolver(c).Load(context.Background(), "ns", "mypack", "1.2.0")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"mypack","version":"v1.2.0"}`, string(raw))
}

func TestLoad_PromptPackNotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "missing", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLoad_ConfigMapNotFound(t *testing.T) {
	pp := configmapPack("pp-p", "p", "1.0.0", "p-data")
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ConfigMap p-data")
}

func TestLoad_MissingConfigMapRef(t *testing.T) {
	pp := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pp-p",
			Namespace: "ns",
			Labels:    map[string]string{packNameLabel: "p"},
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: "p",
			Version:  "1.0.0",
			Source:   omniav1alpha1.PromptPackContentSource{Type: omniav1alpha1.PromptPackSourceTypeConfigMap},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configMapRef")
}

func TestLoad_MissingPackJSONKey(t *testing.T) {
	pp := configmapPack("pp-p", "p", "1.0.0", "p-data")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "p-data", Namespace: "ns"},
		Data:       map[string]string{"other": "x"},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp, cm).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLoad_UnsupportedSourceType(t *testing.T) {
	pp := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pp-p",
			Namespace: "ns",
			Labels:    map[string]string{packNameLabel: "p"},
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: "p",
			Version:  "1.0.0",
			Source:   omniav1alpha1.PromptPackContentSource{Type: "git"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source type")
}
