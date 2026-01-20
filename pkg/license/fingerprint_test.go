/*
Copyright 2025.

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

package license

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClusterFingerprint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  k8stypes.UID("kube-system-uid-12345"),
		},
	}

	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: LicenseSecretNamespace,
			UID:  k8stypes.UID("omnia-system-uid-67890"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	ctx := context.Background()

	t.Run("generates valid fingerprint", func(t *testing.T) {
		fingerprint, err := ClusterFingerprint(ctx, client)
		require.NoError(t, err)
		assert.NotEmpty(t, fingerprint)
		// Fingerprint should be 32 hex characters (128-bit hash)
		assert.Len(t, fingerprint, 32)
	})

	t.Run("fingerprint is deterministic", func(t *testing.T) {
		fp1, err := ClusterFingerprint(ctx, client)
		require.NoError(t, err)

		fp2, err := ClusterFingerprint(ctx, client)
		require.NoError(t, err)

		assert.Equal(t, fp1, fp2)
	})

	t.Run("different UIDs produce different fingerprints", func(t *testing.T) {
		fp1, err := ClusterFingerprint(ctx, client)
		require.NoError(t, err)

		// Create client with different omnia-system UID
		differentOmniaNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: LicenseSecretNamespace,
				UID:  k8stypes.UID("different-omnia-uid"),
			},
		}
		client2 := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kubeSystemNS, differentOmniaNS).
			Build()

		fp2, err := ClusterFingerprint(ctx, client2)
		require.NoError(t, err)

		assert.NotEqual(t, fp1, fp2)
	})
}

func TestClusterFingerprint_MissingNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ctx := context.Background()

	t.Run("missing kube-system namespace", func(t *testing.T) {
		omniaSystemNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: LicenseSecretNamespace,
				UID:  k8stypes.UID("omnia-system-uid"),
			},
		}
		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(omniaSystemNS).
			Build()

		_, err := ClusterFingerprint(ctx, client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "kube-system")
	})

	t.Run("missing omnia-system namespace", func(t *testing.T) {
		kubeSystemNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  k8stypes.UID("kube-system-uid"),
			},
		}
		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kubeSystemNS).
			Build()

		_, err := ClusterFingerprint(ctx, client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), LicenseSecretNamespace)
	})
}

func TestValidateFingerprint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  k8stypes.UID("kube-system-uid-12345"),
		},
	}

	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: LicenseSecretNamespace,
			UID:  k8stypes.UID("omnia-system-uid-67890"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	ctx := context.Background()

	t.Run("validates matching fingerprint", func(t *testing.T) {
		// First get the current fingerprint
		expected, err := ClusterFingerprint(ctx, client)
		require.NoError(t, err)

		// Validate it matches
		valid, err := ValidateFingerprint(ctx, client, expected)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("rejects non-matching fingerprint", func(t *testing.T) {
		valid, err := ValidateFingerprint(ctx, client, "wrong-fingerprint")
		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("returns error when namespace missing", func(t *testing.T) {
		emptyClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		_, err := ValidateFingerprint(ctx, emptyClient, "any-fingerprint")
		assert.Error(t, err)
	})
}
