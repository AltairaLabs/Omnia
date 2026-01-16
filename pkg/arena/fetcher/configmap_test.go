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

package fetcher

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewConfigMapFetcher(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name   string
		config ConfigMapFetcherConfig
	}{
		{
			name: "basic config",
			config: ConfigMapFetcherConfig{
				Name:      "my-config",
				Namespace: "default",
			},
		},
		{
			name: "with custom timeout",
			config: ConfigMapFetcherConfig{
				Name:      "my-config",
				Namespace: "default",
				Options: Options{
					Timeout: 120 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewConfigMapFetcher(tt.config, fakeClient)
			assert.NotNil(t, fetcher)
			assert.Equal(t, "configmap", fetcher.Type())
		})
	}
}

func TestConfigMapFetcher_Type(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test",
		Namespace: "default",
	}, fakeClient)

	assert.Equal(t, "configmap", fetcher.Type())
}

func TestConfigMapFetcher_LatestRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-config",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, "12345", revision)
}

func TestConfigMapFetcher_LatestRevision_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "nonexistent",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ConfigMap")
}

func TestConfigMapFetcher_Fetch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "key: value\n",
			"prompt.txt":  "Hello, world!",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "12345")
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.Equal(t, "12345", artifact.Revision)
	assert.True(t, strings.HasPrefix(artifact.Checksum, "sha256:"))
	assert.Greater(t, artifact.Size, int64(0))

	// Verify tarball contents
	verifyConfigMapTarball(t, artifact.Path, map[string]string{
		"config.yaml": "key: value\n",
		"prompt.txt":  "Hello, world!",
	})
}

func TestConfigMapFetcher_Fetch_WithBinaryData(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
		BinaryData: map[string][]byte{
			"binary.bin": binaryContent,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)

	// Verify binary data is in tarball
	contents := extractTarballContents(t, artifact.Path)
	assert.Equal(t, binaryContent, contents["binary.bin"])
}

func TestConfigMapFetcher_Fetch_RevisionMismatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-config",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "99999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "revision mismatch")
}

func TestConfigMapFetcher_Fetch_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "nonexistent",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ConfigMap")
}

func TestConfigMapFetcher_Fetch_EmptyConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "empty-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		// No Data or BinaryData
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "empty-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	// Should create a valid (empty) tarball
	assert.NotEmpty(t, artifact.Path)
	assert.Equal(t, "12345", artifact.Revision)
}

func TestConfigMapFetcher_Fetch_DataPrecedenceOverBinary(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Same key in both Data and BinaryData - Data should take precedence
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "from-data",
		},
		BinaryData: map[string][]byte{
			"config.yaml": []byte("from-binary"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	// Data should take precedence
	contents := extractTarballContents(t, artifact.Path)
	assert.Equal(t, []byte("from-data"), contents["config.yaml"])
}

// Helper function to verify tarball contents
func verifyConfigMapTarball(t *testing.T, tarballPath string, expected map[string]string) {
	t.Helper()

	contents := extractTarballContents(t, tarballPath)
	for key, expectedValue := range expected {
		actualValue, ok := contents[key]
		assert.True(t, ok, "expected file %s not found in tarball", key)
		assert.Equal(t, []byte(expectedValue), actualValue, "content mismatch for %s", key)
	}
}

// Helper function to extract tarball contents
func extractTarballContents(t *testing.T, tarballPath string) map[string][]byte {
	t.Helper()

	file, err := os.Open(tarballPath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)
	contents := make(map[string][]byte)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		data, err := io.ReadAll(tarReader)
		require.NoError(t, err)
		contents[header.Name] = data
	}

	return contents
}
