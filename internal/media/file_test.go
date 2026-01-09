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

package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFileImpl(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "test-file.txt")

	file, err := createFileImpl(filePath)
	if err != nil {
		t.Fatalf("createFileImpl() error = %v", err)
	}
	defer func() { _ = file.Close() }()

	// Write some content
	content := []byte("test content")
	n, err := file.Write(content)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(content) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(content))
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}

func TestCreateFileImpl_InvalidPath(t *testing.T) {
	// Try to create a file in a non-existent directory
	_, err := createFileImpl("/nonexistent/directory/file.txt")
	if err == nil {
		t.Error("createFileImpl() should fail for invalid path")
	}
}
