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

package content

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfine_NestedAllowed(t *testing.T) {
	base := t.TempDir()
	got, err := Confine(base, "arena/projects/p1/config.yaml")
	if err != nil {
		t.Fatalf("nested path: %v", err)
	}
	want := filepath.Join(base, "arena", "projects", "p1", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfine_RootAllowed(t *testing.T) {
	base := t.TempDir()
	got, err := Confine(base, "")
	if err != nil {
		t.Fatalf("empty path: %v", err)
	}
	if got != base {
		t.Errorf("got %q, want base %q", got, base)
	}
}

func TestConfine_InternalDotDotStaysInside(t *testing.T) {
	base := t.TempDir()
	got, err := Confine(base, "a/../b")
	if err != nil {
		t.Fatalf("internal ..: %v", err)
	}
	if got != filepath.Join(base, "b") {
		t.Errorf("got %q, want %q", got, filepath.Join(base, "b"))
	}
}

func TestConfine_DotDotEscapeRejected(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"../escape", "a/../../escape", "../../etc/passwd"} {
		if _, err := Confine(base, rel); !errors.Is(err, ErrPathEscape) {
			t.Errorf("Confine(%q) err = %v, want ErrPathEscape", rel, err)
		}
	}
}

func TestConfine_AbsoluteRejected(t *testing.T) {
	base := t.TempDir()
	if _, err := Confine(base, "/etc/passwd"); !errors.Is(err, ErrPathEscape) {
		t.Errorf("absolute path err = %v, want ErrPathEscape", err)
	}
}

func TestConfine_SymlinkEscapeRejected(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// A symlink inside base pointing at a directory outside base.
	link := filepath.Join(base, "evil")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// Reading/writing through the symlink must be rejected even though the
	// lexical path stays under base.
	if _, err := Confine(base, "evil/secret.txt"); !errors.Is(err, ErrPathEscape) {
		t.Errorf("symlinked dir err = %v, want ErrPathEscape", err)
	}
	if _, err := Confine(base, "evil"); !errors.Is(err, ErrPathEscape) {
		t.Errorf("symlink itself err = %v, want ErrPathEscape", err)
	}
}

func TestConfine_LegitSymlinkInsideAllowed(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "real"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// A symlink that stays within base is fine.
	if _, err := Confine(base, "link/file.txt"); err != nil {
		t.Errorf("in-base symlink: %v", err)
	}
}
