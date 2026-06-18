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

// Package content serves the operator's authenticated workspace-content API:
// confined filesystem primitives (list/read/write/mkdir/delete) under a
// per-workspace, per-namespace subtree of the shared content root. It is the
// single cross-workspace writer, so path-confinement here is the regression
// guard against one workspace's request reaching another's content.
package content

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscape is returned when a requested path resolves outside its base,
// whether lexically (.., absolute) or via a symlink. Callers map it to 400.
var ErrPathEscape = errors.New("content: path escapes workspace root")

// Confine resolves relpath within base and returns the cleaned absolute target
// path, or ErrPathEscape if relpath would escape base. It rejects absolute
// paths and lexical ".." escapes, then verifies no symlink on the resolved
// portion of the path points outside base.
//
// base must already exist (it is the workspace's content subtree). relpath is
// the client-supplied path relative to base; "" resolves to base itself.
func Confine(base, relpath string) (string, error) {
	if filepath.IsAbs(relpath) {
		return "", fmt.Errorf("%w: absolute path %q", ErrPathEscape, relpath)
	}

	target := filepath.Join(base, relpath)
	if !withinBase(base, target) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, relpath)
	}

	if err := checkNoSymlinkEscape(base, target); err != nil {
		return "", err
	}
	return target, nil
}

// withinBase reports whether target is base itself or lexically nested under it.
func withinBase(base, target string) bool {
	if target == base {
		return true
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// checkNoSymlinkEscape resolves symlinks on the deepest existing ancestor of
// target and verifies the real path remains within the real base. This catches
// a symlinked directory (or the target itself being a symlink) that points
// outside base, which a purely lexical check would miss.
func checkNoSymlinkEscape(base, target string) error {
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return fmt.Errorf("content: resolve base: %w", err)
	}

	probe := target
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if !withinBase(realBase, real) {
				return fmt.Errorf("%w: symlink target outside root", ErrPathEscape)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("content: resolve path: %w", err)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			// Walked to the filesystem root without finding an existing
			// ancestor — should not happen since base exists.
			return fmt.Errorf("%w: no existing ancestor", ErrPathEscape)
		}
		probe = parent
	}
}
