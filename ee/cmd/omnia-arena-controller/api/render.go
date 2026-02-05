/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package api

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	templates "github.com/AltairaLabs/PromptKit/tools/arena/templates"
)

// RenderTemplate renders a template using PromptKit's Generator.
// This is the canonical way to generate projects from Arena templates.
func RenderTemplate(
	templatePath string,
	outputPath string,
	projectName string,
	variables map[string]interface{},
) (*RenderTemplateResponse, error) {
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

	// Ensure output directory exists
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create generator and generate project
	generator := templates.NewGenerator(tmpl, loader)
	config := &templates.TemplateConfig{
		ProjectName: projectName,
		OutputDir:   outputPath,
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
		rel, err := filepath.Rel(outputPath, filepath.Join(result.ProjectPath, f))
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
	variables map[string]interface{},
) (*PreviewTemplateResponse, error) {
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
	var files []PreviewFile
	projectPath := filepath.Join(tempDir, projectName)

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
