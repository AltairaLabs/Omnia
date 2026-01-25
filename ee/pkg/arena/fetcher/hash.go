/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CalculateDirectoryHash calculates a SHA256 hash of directory contents.
// This creates a content-addressable version identifier by hashing:
// - Relative file paths
// - File modes
// - File contents (for regular files)
//
// The hash is deterministic for the same directory contents regardless
// of file timestamps or filesystem metadata.
func CalculateDirectoryHash(dir string) (string, error) {
	h := sha256.New()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Hash the path
		_, _ = h.Write([]byte(relPath))

		// Hash file mode
		_, _ = fmt.Fprintf(h, "%d", info.Mode())

		if info.IsDir() {
			return nil
		}

		// Hash file content
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(h, f); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
