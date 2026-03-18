/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestExtractRequiredGroups(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected []string
	}{
		{
			name: "default group from provider without explicit group",
			yaml: `spec:
  providers:
    - file: providers/claude.yaml`,
			expected: []string{"default"},
		},
		{
			name: "explicit groups",
			yaml: `spec:
  providers:
    - file: providers/claude.yaml
      group: default
    - file: providers/judge.yaml
      group: judge`,
			expected: []string{"default", "judge"},
		},
		{
			name: "self-play role references",
			yaml: `spec:
  providers:
    - file: providers/target.yaml
    - file: providers/sim.yaml
      group: selfplay
  self_play:
    enabled: true
    roles:
      - id: user-sim
        provider: selfplay`,
			expected: []string{"default", "selfplay"},
		},
		{
			name: "self-play disabled ignores roles",
			yaml: `spec:
  providers:
    - file: providers/target.yaml
  self_play:
    enabled: false
    roles:
      - id: user-sim
        provider: selfplay`,
			expected: []string{"default"},
		},
		{
			name: "no providers returns empty",
			yaml: `spec:
  scenarios:
    - file: s1.yaml`,
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.arena.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			got, err := extractRequiredGroups(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.expected) {
				t.Errorf("expected %d groups, got %d: %v", len(tc.expected), len(got), got)
				return
			}

			gotSet := make(map[string]bool)
			for _, g := range got {
				gotSet[g] = true
			}
			for _, e := range tc.expected {
				if !gotSet[e] {
					t.Errorf("expected group %q not found in %v", e, got)
				}
			}
		})
	}
}

func TestValidateProviderGroups(t *testing.T) {
	r := &ArenaJobReconciler{}

	writeConfig := func(t *testing.T, yaml string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.arena.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("passes when all required groups have entries", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
      group: default
    - file: j.yaml
      group: judge`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}}}},
					"judge":   {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}}}},
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg != "" {
			t.Errorf("expected no error, got: %s", msg)
		}
	})

	t.Run("fails when required group is missing", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
    - file: j.yaml
      group: judge`)

		job := &omniav1alpha1.ArenaJob{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}}}},
					// judge group missing
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg == "" {
			t.Error("expected validation error, got none")
		}
		if !contains(msg, "judge") {
			t.Errorf("expected error to mention 'judge', got: %s", msg)
		}
	})

	t.Run("fails when required group is empty", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default":  {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}}}},
					"selfplay": {}, // empty
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg == "" {
			t.Error("expected validation error for empty selfplay group")
		}
		if !contains(msg, "selfplay") {
			t.Errorf("expected error to mention 'selfplay', got: %s", msg)
		}
	})

	t.Run("skips validation when config file not found", func(t *testing.T) {
		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
			},
		}

		msg := r.validateProviderGroups(job, "/nonexistent/config.yaml")
		if msg != "" {
			t.Errorf("expected empty (skip), got: %s", msg)
		}
	})

	t.Run("passes when map-mode groups have entries", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
      group: default
    - file: j.yaml
      group: judge`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Mapping: map[string]omniav1alpha1.ArenaProviderEntry{
						"claude": {ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
					}},
					"judge": {Mapping: map[string]omniav1alpha1.ArenaProviderEntry{
						"haiku": {ProviderRef: &corev1alpha1.ProviderRef{Name: "haiku"}},
					}},
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg != "" {
			t.Errorf("expected no error for map-mode groups, got: %s", msg)
		}
	})

	t.Run("fails when map-mode group is empty", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
    - file: j.yaml
      group: judge`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Mapping: map[string]omniav1alpha1.ArenaProviderEntry{
						"claude": {ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}},
					}},
					"judge": {Mapping: map[string]omniav1alpha1.ArenaProviderEntry{}}, // empty map
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg == "" {
			t.Error("expected validation error for empty map-mode judge group")
		}
		if !contains(msg, "judge") {
			t.Errorf("expected error to mention 'judge', got: %s", msg)
		}
	})
}

func TestValidateProviderGroups_MultipleProvidersAllowed(t *testing.T) {
	r := &ArenaJobReconciler{}

	writeConfig := func(t *testing.T, yaml string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.arena.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("allows multiple providers in self-play group", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
    - file: sim.yaml
      group: selfplay
  self_play:
    enabled: true
    roles:
      - id: sim
        provider: selfplay`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}}}},
					"selfplay": {Entries: []omniav1alpha1.ArenaProviderEntry{
						{ProviderRef: &corev1alpha1.ProviderRef{Name: "ollama-a"}},
						{ProviderRef: &corev1alpha1.ProviderRef{Name: "ollama-b"}},
					}},
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg != "" {
			t.Errorf("multiple providers in a group should be allowed, got: %s", msg)
		}
	})

	t.Run("allows multiple providers in judge group", func(t *testing.T) {
		path := writeConfig(t, `spec:
  providers:
    - file: p.yaml
    - file: j.yaml
      group: judge
  judges:
    - name: quality
      provider: judge`)

		job := &omniav1alpha1.ArenaJob{
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
				Providers: map[string]omniav1alpha1.ArenaProviderGroup{
					"default": {Entries: []omniav1alpha1.ArenaProviderEntry{{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude"}}}},
					"judge": {Entries: []omniav1alpha1.ArenaProviderEntry{
						{ProviderRef: &corev1alpha1.ProviderRef{Name: "gpt-4o"}},
						{ProviderRef: &corev1alpha1.ProviderRef{Name: "claude-haiku"}},
					}},
				},
			},
		}

		msg := r.validateProviderGroups(job, path)
		if msg != "" {
			t.Errorf("multiple providers in judge group should be allowed, got: %s", msg)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
