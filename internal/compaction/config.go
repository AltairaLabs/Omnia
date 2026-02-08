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

package compaction

import (
	"fmt"
	"os"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"sigs.k8s.io/yaml"
)

const defaultWarmRetentionDays = 7

// RetentionConfig is the format read from the ConfigMap projected by the
// SessionRetentionPolicy controller. It mirrors ResolvedRetentionConfig
// (internal/controller) without importing controller internals.
type RetentionConfig struct {
	Default      TierConfig            `json:"default"`
	PerWorkspace map[string]TierConfig `json:"perWorkspace,omitempty"`
}

// TierConfig mirrors ResolvedTierConfig from the retention controller.
type TierConfig struct {
	HotCache    *omniav1alpha1.HotCacheConfig    `json:"hotCache,omitempty"`
	WarmStore   *omniav1alpha1.WarmStoreConfig   `json:"warmStore,omitempty"`
	ColdArchive *omniav1alpha1.ColdArchiveConfig `json:"coldArchive,omitempty"`
}

// LoadRetentionConfig reads and parses a retention YAML file from a ConfigMap mount.
func LoadRetentionConfig(path string) (*RetentionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading retention config: %w", err)
	}
	var cfg RetentionConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing retention config: %w", err)
	}
	return &cfg, nil
}

// WarmCutoff returns the warm-store retention cutoff for the given workspace.
// Sessions last updated before this time are eligible for compaction.
func (c *RetentionConfig) WarmCutoff(workspace string, now time.Time) time.Time {
	if ws, ok := c.PerWorkspace[workspace]; ok && ws.WarmStore != nil && ws.WarmStore.RetentionDays > 0 {
		return now.AddDate(0, 0, -int(ws.WarmStore.RetentionDays))
	}
	return c.defaultWarmCutoff(now)
}

// MinWarmCutoff returns the earliest (most aggressive) warm cutoff across
// all workspaces and the default. Used to build the initial batch query.
func (c *RetentionConfig) MinWarmCutoff(now time.Time) time.Time {
	min := c.defaultWarmCutoff(now)
	for _, ws := range c.PerWorkspace {
		if ws.WarmStore != nil && ws.WarmStore.RetentionDays > 0 {
			cutoff := now.AddDate(0, 0, -int(ws.WarmStore.RetentionDays))
			if cutoff.Before(min) {
				min = cutoff
			}
		}
	}
	return min
}

// ColdCutoff returns the time before which cold archive data should be purged.
// Returns zero time if cold archive retention is not configured.
func (c *RetentionConfig) ColdCutoff(now time.Time) time.Time {
	ca := c.Default.ColdArchive
	if ca == nil || !ca.Enabled || ca.RetentionDays == nil || *ca.RetentionDays <= 0 {
		return time.Time{}
	}
	return now.AddDate(0, 0, -int(*ca.RetentionDays))
}

// ColdArchiveEnabled returns true if the default cold archive is enabled.
func (c *RetentionConfig) ColdArchiveEnabled() bool {
	return c.Default.ColdArchive != nil && c.Default.ColdArchive.Enabled
}

func (c *RetentionConfig) defaultWarmCutoff(now time.Time) time.Time {
	days := int32(defaultWarmRetentionDays)
	if c.Default.WarmStore != nil && c.Default.WarmStore.RetentionDays > 0 {
		days = c.Default.WarmStore.RetentionDays
	}
	return now.AddDate(0, 0, -int(days))
}
