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

// Package pack provides loading and parsing of PromptPacks.
package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Common file names for system prompts in a PromptPack.
var systemPromptFiles = []string{
	"system.txt",
	"system.md",
	"system.prompt",
	"system",
	"index.txt",
	"index.md",
}

// FileLoader loads PromptPacks from the filesystem.
type FileLoader struct {
	path string
}

// NewFileLoader creates a new file-based pack loader.
func NewFileLoader(path string) *FileLoader {
	return &FileLoader{path: path}
}

// LoadSystemPrompt loads the system prompt from the pack directory.
// It looks for common file names and returns the content of the first found.
func (l *FileLoader) LoadSystemPrompt() (string, error) {
	// First, check if the path itself is a file
	info, err := os.Stat(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("pack path does not exist: %s", l.path)
		}
		return "", fmt.Errorf("failed to stat pack path: %w", err)
	}

	if !info.IsDir() {
		// Path is a file, read it directly
		content, err := os.ReadFile(l.path)
		if err != nil {
			return "", fmt.Errorf("failed to read pack file: %w", err)
		}
		return strings.TrimSpace(string(content)), nil
	}

	// Path is a directory, look for common system prompt files
	for _, filename := range systemPromptFiles {
		filePath := filepath.Join(l.path, filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Try next file
			}
			return "", fmt.Errorf("failed to read %s: %w", filename, err)
		}
		return strings.TrimSpace(string(content)), nil
	}

	// No system prompt file found, try to read any .txt or .md file
	entries, err := os.ReadDir(l.path)
	if err != nil {
		return "", fmt.Errorf("failed to read pack directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".prompt") {
			filePath := filepath.Join(l.path, name)
			content, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			return strings.TrimSpace(string(content)), nil
		}
	}

	// No prompt file found
	return "", nil
}

// LoadFile loads a specific file from the pack.
func (l *FileLoader) LoadFile(filename string) (string, error) {
	filePath := filepath.Join(l.path, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filename, err)
	}
	return string(content), nil
}

// Path returns the pack path.
func (l *FileLoader) Path() string {
	return l.path
}

// Exists checks if the pack exists.
func (l *FileLoader) Exists() bool {
	_, err := os.Stat(l.path)
	return err == nil
}
