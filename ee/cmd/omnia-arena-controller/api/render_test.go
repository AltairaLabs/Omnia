/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// getTestdataPath returns the absolute path to the testdata directory.
func getTestdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestRenderTemplate_InvalidTemplatePath(t *testing.T) {
	outputDir := t.TempDir()

	_, err := RenderTemplate("/nonexistent/template/path", outputDir, "test-project", nil)
	if err == nil {
		t.Error("RenderTemplate() should fail for nonexistent template path")
	}
}

func TestRenderTemplate_InvalidOutputPath(t *testing.T) {
	// Create a file at the output path so directory creation fails
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output")
	if err := os.WriteFile(outputPath, []byte("file"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Try to use the file as a directory path
	_, err := RenderTemplate("/some/template", filepath.Join(outputPath, "subdir"), "test", nil)
	if err == nil {
		t.Error("RenderTemplate() should fail when output directory cannot be created")
	}
}

func TestPreviewTemplate_InvalidTemplatePath(t *testing.T) {
	_, err := PreviewTemplate("/nonexistent/template/path", "test-project", nil)
	if err == nil {
		t.Error("PreviewTemplate() should fail for nonexistent template path")
	}
}

func TestRenderTemplate_EmptyProjectName(t *testing.T) {
	// Even with empty project name, it should reach PromptKit validation
	// The function will fail during template loading since path is invalid
	_, err := RenderTemplate("/nonexistent", t.TempDir(), "", nil)
	if err == nil {
		t.Error("RenderTemplate() should fail for invalid template path")
	}
}

func TestPreviewTemplate_EmptyProjectName(t *testing.T) {
	// Even with empty project name, it should fail during template loading
	_, err := PreviewTemplate("/nonexistent", "", nil)
	if err == nil {
		t.Error("PreviewTemplate() should fail for invalid template path")
	}
}

func TestRenderTemplate_NilVariables(t *testing.T) {
	// Nil variables should be handled gracefully
	// Will still fail due to invalid path
	_, err := RenderTemplate("/nonexistent", t.TempDir(), "test", nil)
	if err == nil {
		t.Error("RenderTemplate() should fail for invalid template path")
	}
}

func TestPreviewTemplate_NilVariables(t *testing.T) {
	// Nil variables should be handled gracefully
	// Will still fail due to invalid path
	_, err := PreviewTemplate("/nonexistent", "test", nil)
	if err == nil {
		t.Error("PreviewTemplate() should fail for invalid template path")
	}
}

func TestRenderTemplate_EmptyVariables(t *testing.T) {
	// Empty map should be handled gracefully
	// Will still fail due to invalid path
	_, err := RenderTemplate("/nonexistent", t.TempDir(), "test", map[string]any{})
	if err == nil {
		t.Error("RenderTemplate() should fail for invalid template path")
	}
}

func TestPreviewTemplate_EmptyVariables(t *testing.T) {
	// Empty map should be handled gracefully
	// Will still fail due to invalid path
	_, err := PreviewTemplate("/nonexistent", "test", map[string]any{})
	if err == nil {
		t.Error("PreviewTemplate() should fail for invalid template path")
	}
}

// TestRenderTemplateResponse_Fields verifies the response struct fields.
func TestRenderTemplateResponse_Fields(t *testing.T) {
	resp := &RenderTemplateResponse{
		Success:      true,
		FilesCreated: []string{"file1.yaml", "file2.yaml"},
		Errors:       []string{},
		Warnings:     []string{"warning1"},
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if len(resp.FilesCreated) != 2 {
		t.Errorf("FilesCreated len = %d, want 2", len(resp.FilesCreated))
	}
	if len(resp.Errors) != 0 {
		t.Errorf("Errors len = %d, want 0", len(resp.Errors))
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("Warnings len = %d, want 1", len(resp.Warnings))
	}
}

// TestPreviewTemplateResponse_Fields verifies the response struct fields.
func TestPreviewTemplateResponse_Fields(t *testing.T) {
	resp := &PreviewTemplateResponse{
		Files: []PreviewFile{
			{Path: "config.yaml", Content: "name: test"},
		},
		Errors: []string{},
	}

	if len(resp.Files) != 1 {
		t.Errorf("Files len = %d, want 1", len(resp.Files))
	}
	if resp.Files[0].Path != "config.yaml" {
		t.Errorf("Files[0].Path = %q, want %q", resp.Files[0].Path, "config.yaml")
	}
	if resp.Files[0].Content != "name: test" {
		t.Errorf("Files[0].Content = %q, want %q", resp.Files[0].Content, "name: test")
	}
}

// TestPreviewFile_Fields verifies the preview file struct.
func TestPreviewFile_Fields(t *testing.T) {
	pf := PreviewFile{
		Path:    "prompts/main.yaml",
		Content: "system: You are an assistant",
	}

	if pf.Path != "prompts/main.yaml" {
		t.Errorf("Path = %q, want %q", pf.Path, "prompts/main.yaml")
	}
	if pf.Content != "system: You are an assistant" {
		t.Errorf("Content = %q, want %q", pf.Content, "system: You are an assistant")
	}
}

func TestRenderTemplate_Success(t *testing.T) {
	templatePath := filepath.Join(getTestdataPath(), "simple-template")
	outputDir := t.TempDir()

	result, err := RenderTemplate(templatePath, outputDir, "test-project", map[string]any{
		"project_name": "my-test-project",
		"greeting":     "Hi",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true; errors: %v", result.Errors)
	}

	if len(result.FilesCreated) == 0 {
		t.Error("FilesCreated is empty, expected at least one file")
	}

	// Verify files were created
	for _, f := range result.FilesCreated {
		fullPath := filepath.Join(outputDir, f)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("File %q was not created", f)
		}
	}
}

func TestRenderTemplate_SuccessWithDefaults(t *testing.T) {
	templatePath := filepath.Join(getTestdataPath(), "simple-template")
	outputDir := t.TempDir()

	// Use nil variables to test defaults
	result, err := RenderTemplate(templatePath, outputDir, "default-project", nil)
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true; errors: %v", result.Errors)
	}
}

func TestPreviewTemplate_Success(t *testing.T) {
	templatePath := filepath.Join(getTestdataPath(), "simple-template")

	result, err := PreviewTemplate(templatePath, "test-project", map[string]any{
		"project_name": "preview-project",
		"greeting":     "Welcome",
	})
	if err != nil {
		t.Fatalf("PreviewTemplate() error = %v", err)
	}

	if len(result.Files) == 0 {
		t.Error("Files is empty, expected at least one file")
	}

	// Check that files contain rendered content
	foundConfig := false
	foundReadme := false
	for _, f := range result.Files {
		if strings.HasSuffix(f.Path, "config.yaml") {
			foundConfig = true
			if !strings.Contains(f.Content, "preview-project") {
				t.Errorf("config.yaml should contain project name, got: %s", f.Content)
			}
		}
		if strings.HasSuffix(f.Path, "README.md") {
			foundReadme = true
			if !strings.Contains(f.Content, "Welcome") {
				t.Errorf("README.md should contain greeting, got: %s", f.Content)
			}
		}
	}

	if !foundConfig {
		t.Error("config.yaml not found in preview files")
	}
	if !foundReadme {
		t.Error("README.md not found in preview files")
	}
}

func TestPreviewTemplate_SuccessWithDefaults(t *testing.T) {
	templatePath := filepath.Join(getTestdataPath(), "simple-template")

	result, err := PreviewTemplate(templatePath, "default-project", nil)
	if err != nil {
		t.Fatalf("PreviewTemplate() error = %v", err)
	}

	if len(result.Files) == 0 {
		t.Error("Files is empty, expected at least one file")
	}
}

func TestRenderTemplate_VerifyContent(t *testing.T) {
	templatePath := filepath.Join(getTestdataPath(), "simple-template")
	outputDir := t.TempDir()

	result, err := RenderTemplate(templatePath, outputDir, "content-test", map[string]any{
		"project_name": "verified-project",
		"greeting":     "Greetings",
	})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	if !result.Success {
		t.Fatalf("Success = false, errors: %v", result.Errors)
	}

	// Read and verify the config file content
	configPath := filepath.Join(outputDir, "content-test", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.yaml: %v", err)
	}

	if !strings.Contains(string(content), "verified-project") {
		t.Errorf("config.yaml should contain 'verified-project', got: %s", string(content))
	}
	if !strings.Contains(string(content), "Greetings") {
		t.Errorf("config.yaml should contain 'Greetings', got: %s", string(content))
	}
}
