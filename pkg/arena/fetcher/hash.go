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
