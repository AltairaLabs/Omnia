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

package pack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLoader_LoadSystemPrompt_File(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "pack-*.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := "You are a helpful assistant."
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	loader := NewFileLoader(tmpFile.Name())
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, content, prompt)
}

func TestFileLoader_LoadSystemPrompt_Directory(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create system.txt
	content := "You are a helpful assistant."
	err = os.WriteFile(filepath.Join(tmpDir, "system.txt"), []byte(content), 0644)
	require.NoError(t, err)

	loader := NewFileLoader(tmpDir)
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, content, prompt)
}

func TestFileLoader_LoadSystemPrompt_DirectoryWithMD(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create system.md
	content := "# System Prompt\n\nYou are a helpful assistant."
	err = os.WriteFile(filepath.Join(tmpDir, "system.md"), []byte(content), 0644)
	require.NoError(t, err)

	loader := NewFileLoader(tmpDir)
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, content, prompt)
}

func TestFileLoader_LoadSystemPrompt_FallbackToAnyTxt(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a non-standard prompt file
	content := "Custom prompt content"
	err = os.WriteFile(filepath.Join(tmpDir, "custom.txt"), []byte(content), 0644)
	require.NoError(t, err)

	loader := NewFileLoader(tmpDir)
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, content, prompt)
}

func TestFileLoader_LoadSystemPrompt_NoPromptFile(t *testing.T) {
	// Create empty temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	loader := NewFileLoader(tmpDir)
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Empty(t, prompt)
}

func TestFileLoader_LoadSystemPrompt_NotExists(t *testing.T) {
	loader := NewFileLoader("/nonexistent/path")
	_, err := loader.LoadSystemPrompt()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestFileLoader_LoadFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a file
	content := "Some content"
	err = os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644)
	require.NoError(t, err)

	loader := NewFileLoader(tmpDir)
	loaded, err := loader.LoadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, loaded)
}

func TestFileLoader_LoadFile_NotExists(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	loader := NewFileLoader(tmpDir)
	_, err = loader.LoadFile("nonexistent.txt")
	require.Error(t, err)
}

func TestFileLoader_Exists(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pack-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	loader := NewFileLoader(tmpDir)
	assert.True(t, loader.Exists())

	loader2 := NewFileLoader("/nonexistent/path")
	assert.False(t, loader2.Exists())
}

func TestFileLoader_Path(t *testing.T) {
	loader := NewFileLoader("/some/path")
	assert.Equal(t, "/some/path", loader.Path())
}

func TestFileLoader_LoadSystemPrompt_Whitespace(t *testing.T) {
	// Create temp file with whitespace
	tmpFile, err := os.CreateTemp("", "pack-*.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_, err = tmpFile.WriteString("  Hello World  \n\n")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	loader := NewFileLoader(tmpFile.Name())
	prompt, err := loader.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, "Hello World", prompt) // Trimmed
}
