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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapFetcherConfig contains configuration for the ConfigMap fetcher.
type ConfigMapFetcherConfig struct {
	// Name is the ConfigMap name.
	Name string

	// Namespace is the ConfigMap namespace.
	Namespace string

	// Options contains common fetcher options.
	Options Options
}

// ConfigMapFetcher implements the Fetcher interface for Kubernetes ConfigMaps.
type ConfigMapFetcher struct {
	config ConfigMapFetcherConfig
	client client.Client
}

// NewConfigMapFetcher creates a new ConfigMap fetcher with the given configuration.
func NewConfigMapFetcher(config ConfigMapFetcherConfig, k8sClient client.Client) *ConfigMapFetcher {
	if config.Options.Timeout == 0 {
		config.Options = DefaultOptions()
	}
	return &ConfigMapFetcher{
		config: config,
		client: k8sClient,
	}
}

// Type returns the source type.
func (f *ConfigMapFetcher) Type() string {
	return "configmap"
}

// LatestRevision returns the resourceVersion of the ConfigMap.
func (f *ConfigMapFetcher) LatestRevision(ctx context.Context) (string, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      f.config.Name,
		Namespace: f.config.Namespace,
	}

	if err := f.client.Get(ctx, key, cm); err != nil {
		return "", fmt.Errorf("failed to get ConfigMap %s/%s: %w", f.config.Namespace, f.config.Name, err)
	}

	return cm.ResourceVersion, nil
}

// Fetch creates a tarball from the ConfigMap data.
func (f *ConfigMapFetcher) Fetch(ctx context.Context, revision string) (*Artifact, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      f.config.Name,
		Namespace: f.config.Namespace,
	}

	if err := f.client.Get(ctx, key, cm); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", f.config.Namespace, f.config.Name, err)
	}

	// Verify revision matches if specified
	if revision != "" && cm.ResourceVersion != revision {
		return nil, fmt.Errorf("ConfigMap revision mismatch: expected %s, got %s", revision, cm.ResourceVersion)
	}

	// Create tarball from ConfigMap data
	tarballPath, checksum, size, err := f.createTarball(cm)
	if err != nil {
		return nil, fmt.Errorf("failed to create tarball: %w", err)
	}

	// Determine last modified time
	lastModified := time.Now()
	if cm.CreationTimestamp.After(time.Time{}) {
		lastModified = cm.CreationTimestamp.Time
	}

	return &Artifact{
		Path:         tarballPath,
		Revision:     cm.ResourceVersion,
		Checksum:     checksum,
		Size:         size,
		LastModified: lastModified,
	}, nil
}

// createTarball creates a gzipped tarball from ConfigMap data.
func (f *ConfigMapFetcher) createTarball(cm *corev1.ConfigMap) (string, string, int64, error) {
	// Create temporary file for the tarball
	tmpFile, err := os.CreateTemp(f.config.Options.WorkDir, "configmap-*.tar.gz")
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	hash := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hash)

	gzipWriter := gzip.NewWriter(multiWriter)
	tarWriter := tar.NewWriter(gzipWriter)

	// Track if we've closed everything successfully
	var closed bool
	defer func() {
		if !closed {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			_ = tmpFile.Close()
		}
	}()

	// Get sorted keys for deterministic output
	keys := getSortedConfigMapKeys(cm)

	// Add each file to the tarball
	if err := writeConfigMapEntries(tarWriter, cm, keys); err != nil {
		return "", "", 0, err
	}

	// Close writers to flush
	if err := closeWriters(tarWriter, gzipWriter, tmpFile); err != nil {
		return "", "", 0, err
	}
	closed = true

	// Get file size
	stat, err := os.Stat(tmpFile.Name())
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to stat tarball: %w", err)
	}

	checksum := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return tmpFile.Name(), checksum, stat.Size(), nil
}

// getSortedConfigMapKeys returns sorted keys from both Data and BinaryData, with Data taking precedence.
func getSortedConfigMapKeys(cm *corev1.ConfigMap) []string {
	keys := make([]string, 0, len(cm.Data)+len(cm.BinaryData))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	for k := range cm.BinaryData {
		// Only add if not already in Data (Data takes precedence)
		if _, exists := cm.Data[k]; !exists {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// writeConfigMapEntries writes ConfigMap entries to a tar archive.
func writeConfigMapEntries(tarWriter *tar.Writer, cm *corev1.ConfigMap, keys []string) error {
	for _, key := range keys {
		content := getConfigMapContent(cm, key)
		if err := writeTarEntry(tarWriter, key, content, cm.CreationTimestamp.Time); err != nil {
			return err
		}
	}
	return nil
}

// getConfigMapContent retrieves content for a key from ConfigMap Data or BinaryData.
func getConfigMapContent(cm *corev1.ConfigMap, key string) []byte {
	if data, ok := cm.Data[key]; ok {
		return []byte(data)
	}
	if binData, ok := cm.BinaryData[key]; ok {
		return binData
	}
	return nil
}

// writeTarEntry writes a single entry to the tar archive.
func writeTarEntry(tarWriter *tar.Writer, name string, content []byte, modTime time.Time) error {
	header := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: modTime,
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for %s: %w", name, err)
	}

	if _, err := tarWriter.Write(content); err != nil {
		return fmt.Errorf("failed to write tar content for %s: %w", name, err)
	}

	return nil
}

// closeWriters closes all writers in order.
func closeWriters(tarWriter *tar.Writer, gzipWriter *gzip.Writer, tmpFile *os.File) error {
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	return nil
}

// Ensure ConfigMapFetcher implements Fetcher interface.
var _ Fetcher = (*ConfigMapFetcher)(nil)
