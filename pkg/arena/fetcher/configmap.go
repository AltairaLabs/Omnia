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
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// Fetch creates a directory from the ConfigMap data.
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

	// Write ConfigMap data to directory
	dirPath, err := f.writeToDirectory(cm)
	if err != nil {
		return nil, fmt.Errorf("failed to write to directory: %w", err)
	}

	// Calculate checksum of directory contents
	checksum, err := CalculateDirectoryHash(dirPath)
	if err != nil {
		_ = os.RemoveAll(dirPath)
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Calculate total size
	size, err := CalculateDirectorySize(dirPath)
	if err != nil {
		_ = os.RemoveAll(dirPath)
		return nil, fmt.Errorf("failed to calculate size: %w", err)
	}

	// Determine last modified time
	lastModified := time.Now()
	if cm.CreationTimestamp.After(time.Time{}) {
		lastModified = cm.CreationTimestamp.Time
	}

	return &Artifact{
		Path:         dirPath,
		Revision:     cm.ResourceVersion,
		Checksum:     "sha256:" + checksum,
		Size:         size,
		LastModified: lastModified,
	}, nil
}

// writeToDirectory writes ConfigMap data to a temporary directory.
func (f *ConfigMapFetcher) writeToDirectory(cm *corev1.ConfigMap) (string, error) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp(f.config.Options.WorkDir, "configmap-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Get sorted keys for deterministic output
	keys := getSortedConfigMapKeys(cm)

	// Write each file
	for _, key := range keys {
		content := getConfigMapContent(cm, key)
		filePath := filepath.Join(tmpDir, key)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", fmt.Errorf("failed to create directory for %s: %w", key, err)
		}

		// Write file with deterministic modification time
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", fmt.Errorf("failed to write file %s: %w", key, err)
		}

		// Set modification time to ConfigMap creation time for determinism
		modTime := cm.CreationTimestamp.Time
		if modTime.IsZero() {
			modTime = time.Now()
		}
		if err := os.Chtimes(filePath, modTime, modTime); err != nil {
			// Non-fatal: log but continue
			_ = err
		}
	}

	return tmpDir, nil
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

// Ensure ConfigMapFetcher implements Fetcher interface.
var _ Fetcher = (*ConfigMapFetcher)(nil)
