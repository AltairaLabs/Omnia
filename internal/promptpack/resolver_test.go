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

func configmapPack(name, cmName string) *omniav1alpha1.PromptPack {
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec: omniav1alpha1.PromptPackSpec{
			Source: omniav1alpha1.PromptPackSource{
				Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: cmName},
			},
		},
	}
}

// TestLoad_FollowsConfigMapRef is the core regression: the pack name and the
// backing ConfigMap name DIFFER, so a loader that guessed the ConfigMap name
// from the pack name would miss. The resolver must follow configMapRef.
func TestLoad_FollowsConfigMapRef(t *testing.T) {
	pp := configmapPack("rag-hero-pack", "rag-hero-pack-v1")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rag-hero-pack-v1", Namespace: "ns"},
		Data:       map[string]string{packJSONKey: `{"id":"rag-hero-pack","version":"1.0.0"}`},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp, cm).Build()

	raw, err := NewResolver(c).Load(context.Background(), "ns", "rag-hero-pack")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"rag-hero-pack","version":"1.0.0"}`, string(raw))
}

func TestLoad_PromptPackNotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get PromptPack ns/missing")
}

func TestLoad_ConfigMapNotFound(t *testing.T) {
	pp := configmapPack("p", "p-data")
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ConfigMap p-data")
}

func TestLoad_MissingConfigMapRef(t *testing.T) {
	pp := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: omniav1alpha1.PromptPackSpec{
			Source: omniav1alpha1.PromptPackSource{Type: omniav1alpha1.PromptPackSourceTypeConfigMap},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configMapRef")
}

func TestLoad_MissingPackJSONKey(t *testing.T) {
	pp := configmapPack("p", "p-data")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "p-data", Namespace: "ns"},
		Data:       map[string]string{"other": "x"},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp, cm).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLoad_UnsupportedSourceType(t *testing.T) {
	pp := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: omniav1alpha1.PromptPackSpec{
			Source: omniav1alpha1.PromptPackSource{Type: "git"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pp).Build()
	_, err := NewResolver(c).Load(context.Background(), "ns", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source type")
}
