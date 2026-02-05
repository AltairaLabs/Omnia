/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package api

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	templates "github.com/AltairaLabs/PromptKit/tools/arena/templates"
)

// ErrInvalidPath is returned when a path contains path traversal sequences.
var ErrInvalidPath = errors.New("invalid path: contains path traversal sequences")

// ErrInvalidProjectName is returned when a project name contains invalid characters.
var ErrInvalidProjectName = errors.New("invalid project name: contains path separators or traversal sequences")

// ErrPathOutsideBase is returned when a path resolves outside the allowed base directory.
var ErrPathOutsideBase = errors.New("path resolves outside allowed base directory")

// allowedOutputPrefixes defines the allowed output path prefixes for template rendering.
// This prevents path traversal attacks by ensuring output is only written to safe locations.
// - /workspace-content: Production workspace storage mounted in containers
// - /tmp: For tests and temporary operations
// - /var/folders: macOS test temp directories
var allowedOutputPrefixes = []string{"/workspace-content", "/tmp", "/var/folders"}

// validateOutputPath ensures the output path is within an allowed directory prefix.
// This is a security check to prevent writing files to arbitrary locations.
func validateOutputPath(absPath string) error {
	for _, prefix := range allowedOutputPrefixes {
		// Check if path equals the prefix exactly, or starts with prefix followed by separator
		if absPath == prefix || strings.HasPrefix(absPath, prefix+"/") {
			return nil
		}
	}
	return fmt.Errorf("%w: output path must be within an allowed directory", ErrPathOutsideBase)
}

// validateProjectName ensures a project name doesn't contain path separators or traversal sequences.
func validateProjectName(name string) error {
	// Check for path traversal sequences
	if strings.Contains(name, "..") {
		return ErrInvalidProjectName
	}

	// Check for path separators
	if strings.ContainsAny(name, "/\\") {
		return ErrInvalidProjectName
	}

	// Ensure name is not empty and doesn't start with a dot (hidden file)
	if name == "" || strings.HasPrefix(name, ".") {
		return ErrInvalidProjectName
	}

	return nil
}

// validatePathWithinBase ensures that a path resolves within the given base directory.
// This prevents path traversal attacks even with cleaned paths.
func validatePathWithinBase(basePath, targetPath string) error {
	// Get absolute paths for comparison
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to resolve base path: %w", err)
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Ensure target is within base (using filepath.Rel to check containment)
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return ErrPathOutsideBase
	}

	// If the relative path starts with "..", target is outside base
	if strings.HasPrefix(rel, "..") {
		return ErrPathOutsideBase
	}

	return nil
}

// RenderTemplate renders a template using PromptKit's Generator.
// This is the canonical way to generate projects from Arena templates.
// The outputPath must be an absolute path within /workspace-content or similar safe directory.
func RenderTemplate(
	templatePath string,
	outputPath string,
	projectName string,
	variables map[string]any,
) (*RenderTemplateResponse, error) {
	// Validate project name to prevent path traversal
	if err := validateProjectName(projectName); err != nil {
		return nil, fmt.Errorf("invalid project name: %w", err)
	}

	// Clean and validate output path
	cleanOutputPath := filepath.Clean(outputPath)
	if strings.Contains(cleanOutputPath, "..") {
		return nil, ErrInvalidPath
	}

	// Ensure output path is absolute and doesn't escape intended directory
	absOutputPath, err := filepath.Abs(cleanOutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve output path: %w", err)
	}
	// Re-check for traversal after resolution
	if strings.Contains(absOutputPath, "..") {
		return nil, ErrInvalidPath
	}

	// Validate that output path is within an allowed directory (security check)
	if err := validateOutputPath(absOutputPath); err != nil {
		return nil, err
	}

	// Create a temporary cache directory for the loader
	cacheDir, err := os.MkdirTemp("", "arena-template-cache-")
	if err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()

	// Create loader and load template from path
	loader := templates.NewLoader(cacheDir)
	tmpl, err := loader.LoadFromPath(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load template from %s: %w", templatePath, err)
	}

	// Ensure output directory exists (use absolute path)
	// #nosec G301 - directory permissions are intentionally 0755 for workspace content
	if err := os.MkdirAll(absOutputPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create generator and generate project
	generator := templates.NewGenerator(tmpl, loader)
	config := &templates.TemplateConfig{
		ProjectName: projectName,
		OutputDir:   absOutputPath,
		Variables:   variables,
		Verbose:     false,
	}

	result, err := generator.Generate(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate project: %w", err)
	}

	// Convert errors to strings
	errorStrings := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		errorStrings = append(errorStrings, e.Error())
	}

	// Make file paths relative to output directory for cleaner response
	relativeFiles := make([]string, 0, len(result.FilesCreated))
	for _, f := range result.FilesCreated {
		rel, err := filepath.Rel(absOutputPath, filepath.Join(result.ProjectPath, f))
		if err != nil {
			rel = f
		}
		relativeFiles = append(relativeFiles, rel)
	}

	return &RenderTemplateResponse{
		Success:      result.Success,
		FilesCreated: relativeFiles,
		Errors:       errorStrings,
		Warnings:     result.Warnings,
	}, nil
}

// PreviewTemplate renders a template to a temp directory using PromptKit's Generator,
// reads the rendered files, and returns their contents without persisting them.
func PreviewTemplate(
	templatePath string,
	projectName string,
	variables map[string]any,
) (*PreviewTemplateResponse, error) {
	// Validate project name to prevent path traversal
	if err := validateProjectName(projectName); err != nil {
		return nil, fmt.Errorf("invalid project name: %w", err)
	}

	// Create a temporary output directory
	tempDir, err := os.MkdirTemp("", "arena-template-preview-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a temporary cache directory for the loader
	cacheDir, err := os.MkdirTemp("", "arena-template-cache-")
	if err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()

	// Create loader and load template from path
	loader := templates.NewLoader(cacheDir)
	tmpl, err := loader.LoadFromPath(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load template from %s: %w", templatePath, err)
	}

	// Create generator and generate project to temp directory
	generator := templates.NewGenerator(tmpl, loader)
	config := &templates.TemplateConfig{
		ProjectName: projectName,
		OutputDir:   tempDir,
		Variables:   variables,
		Verbose:     false,
	}

	result, err := generator.Generate(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate project: %w", err)
	}

	// Convert errors to strings
	errorStrings := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		errorStrings = append(errorStrings, e.Error())
	}

	// Read rendered files from temp directory
	// Construct and validate project path to ensure it stays within tempDir
	var files []PreviewFile
	projectPath := filepath.Join(tempDir, projectName)

	// Validate that projectPath is within tempDir (defense in depth)
	if err := validatePathWithinBase(tempDir, projectPath); err != nil {
		return nil, fmt.Errorf("invalid project path: %w", err)
	}

	err = filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(projectPath, path)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", relPath, err)
		}

		files = append(files, PreviewFile{
			Path:    relPath,
			Content: string(content),
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read rendered files: %w", err)
	}

	return &PreviewTemplateResponse{
		Files:  files,
		Errors: errorStrings,
	}, nil
}
