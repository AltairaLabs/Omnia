/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
)

func TestSafeOutputDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{
			name:       "empty config falls back to default",
			configured: "",
			want:       devConsoleOutputDir,
		},
		{
			name:       "clean absolute path passes through",
			configured: "/tmp/custom-arena-out",
			want:       "/tmp/custom-arena-out",
		},
		{
			name:       "uncleaned absolute path gets cleaned",
			configured: "/tmp//custom//out/",
			want:       "/tmp/custom/out",
		},
		{
			name:       "absolute path outside /tmp is allowed",
			configured: "/var/lib/arena",
			want:       "/var/lib/arena",
		},
		{
			name:       "relative path is rooted under safe default",
			configured: "foo/bar",
			want:       filepath.Join(devConsoleOutputDir, "foo/bar"),
		},
		{
			name:       "dot-slash relative path is rooted",
			configured: "./outputs",
			want:       filepath.Join(devConsoleOutputDir, "outputs"),
		},
		{
			name:       "absolute path with .. traversal is rejected, falls back to default",
			configured: "/tmp/foo/../../../etc",
			want:       devConsoleOutputDir,
		},
		{
			name:       "relative path with .. traversal is rejected, falls back to default",
			configured: "../../../etc",
			want:       devConsoleOutputDir,
		},
		{
			name:       "leading .. is rejected",
			configured: "..",
			want:       devConsoleOutputDir,
		},
		{
			name:       "embedded /../ is rejected",
			configured: "/tmp/custom/../etc/shadow",
			want:       devConsoleOutputDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := safeOutputDir(tc.configured, logr.Discard())
			if got != tc.want {
				t.Errorf("safeOutputDir(%q) = %q, want %q", tc.configured, got, tc.want)
			}
		})
	}
}
