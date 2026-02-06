/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDiscoverer(t *testing.T) {
	tests := []struct {
		name          string
		sourcePath    string
		templatesPath string
		wantPath      string
	}{
		{
			name:          "default templates path",
			sourcePath:    "/source",
			templatesPath: "",
			wantPath:      DefaultTemplatesPath,
		},
		{
			name:          "custom templates path",
			sourcePath:    "/source",
			templatesPath: "custom/templates",
			wantPath:      "custom/templates",
		},
		{
			name:          "strips trailing slash",
			sourcePath:    "/source",
			templatesPath: "templates/",
			wantPath:      "templates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDiscoverer(tt.sourcePath, tt.templatesPath)
			if d.SourcePath != tt.sourcePath {
				t.Errorf("SourcePath = %q, want %q", d.SourcePath, tt.sourcePath)
			}
			if d.TemplatesPath != tt.wantPath {
				t.Errorf("TemplatesPath = %q, want %q", d.TemplatesPath, tt.wantPath)
			}
		})
	}
}

func TestDiscoverAutoDiscover(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create templates directory
	templatesDir := filepath.Join(tmpDir, "templates", "basic-chatbot")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create template dir: %v", err)
	}

	// Create template.yaml
	templateYAML := `apiVersion: arena.altairalabs.ai/v1alpha1
kind: ArenaTemplate
metadata:
  name: basic-chatbot
  version: "1.0.0"
spec:
  displayName: Basic Chatbot
  description: A simple chatbot template
  category: chatbot
  tags:
    - chatbot
    - beginner
  variables:
    - name: projectName
      type: string
      required: true
`
	if err := os.WriteFile(filepath.Join(templatesDir, "template.yaml"), []byte(templateYAML), 0644); err != nil {
		t.Fatalf("failed to write template.yaml: %v", err)
	}

	// Create config file for default files discovery
	if err := os.WriteFile(filepath.Join(templatesDir, "config.yaml"), []byte("test: value"), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	d := NewDiscoverer(tmpDir, "templates")
	templates, err := d.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(templates) != 1 {
		t.Errorf("Discover() returned %d templates, want 1", len(templates))
		return
	}

	if templates[0].Name != "basic-chatbot" {
		t.Errorf("templates[0].Name = %q, want %q", templates[0].Name, "basic-chatbot")
	}
	if templates[0].Category != "chatbot" {
		t.Errorf("templates[0].Category = %q, want %q", templates[0].Category, "chatbot")
	}
	if len(templates[0].Variables) != 1 {
		t.Errorf("len(templates[0].Variables) = %d, want 1", len(templates[0].Variables))
	}
}

func TestDiscoverFromIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template directory
	templatesDir := filepath.Join(tmpDir, "templates", "indexed-template")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create template dir: %v", err)
	}

	// Create index file
	indexYAML := `templates:
  - name: indexed-template
    path: templates/indexed-template
`
	if err := os.WriteFile(filepath.Join(tmpDir, IndexFileName), []byte(indexYAML), 0644); err != nil {
		t.Fatalf("failed to write index file: %v", err)
	}

	// Create template.yaml
	templateYAML := `apiVersion: arena.altairalabs.ai/v1alpha1
kind: ArenaTemplate
metadata:
  name: indexed-template
spec:
  displayName: Indexed Template
`
	if err := os.WriteFile(filepath.Join(templatesDir, "template.yaml"), []byte(templateYAML), 0644); err != nil {
		t.Fatalf("failed to write template.yaml: %v", err)
	}

	d := NewDiscoverer(tmpDir, "templates")
	templates, err := d.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(templates) != 1 {
		t.Errorf("Discover() returned %d templates, want 1", len(templates))
		return
	}

	if templates[0].Name != "indexed-template" {
		t.Errorf("templates[0].Name = %q, want %q", templates[0].Name, "indexed-template")
	}
}

func TestShouldRender(t *testing.T) {
	d := &Discoverer{}

	tests := []struct {
		name string
		file string
		want bool
	}{
		{"yaml file", "config.yaml", true},
		{"yml file", "config.yml", true},
		{"json file", "data.json", true},
		{"txt file", "readme.txt", true},
		{"md file", "README.md", true},
		{"binary file", "image.png", false},
		{"go file", "main.go", false},
		{"no extension", "Dockerfile", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.shouldRender(tt.file)
			if got != tt.want {
				t.Errorf("shouldRender(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestGetTemplateByName(t *testing.T) {
	templates := []Template{
		{Name: "template-a", DisplayName: "Template A"},
		{Name: "template-b", DisplayName: "Template B"},
		{Name: "template-c", DisplayName: "Template C"},
	}

	tests := []struct {
		name   string
		search string
		want   string
	}{
		{"found first", "template-a", "Template A"},
		{"found middle", "template-b", "Template B"},
		{"found last", "template-c", "Template C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTemplateByName(templates, tt.search)
			if got == nil {
				t.Errorf("GetTemplateByName() returned nil, want %q", tt.want)
				return
			}
			if got.DisplayName != tt.want {
				t.Errorf("GetTemplateByName().DisplayName = %q, want %q", got.DisplayName, tt.want)
			}
		})
	}

	// Test not found
	if got := GetTemplateByName(templates, "nonexistent"); got != nil {
		t.Errorf("GetTemplateByName(nonexistent) = %v, want nil", got)
	}
}

func TestFilterByCategory(t *testing.T) {
	templates := []Template{
		{Name: "chatbot-1", Category: "chatbot"},
		{Name: "chatbot-2", Category: "chatbot"},
		{Name: "agent-1", Category: "agent"},
		{Name: "no-category", Category: ""},
	}

	tests := []struct {
		name     string
		category string
		want     int
	}{
		{"chatbot category", "chatbot", 2},
		{"agent category", "agent", 1},
		{"case insensitive", "Chatbot", 2},
		{"empty category returns all", "", 4},
		{"nonexistent category", "nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByCategory(templates, tt.category)
			if len(got) != tt.want {
				t.Errorf("FilterByCategory(%q) returned %d templates, want %d", tt.category, len(got), tt.want)
			}
		})
	}
}

func TestFilterByTags(t *testing.T) {
	templates := []Template{
		{Name: "template-1", Tags: []string{"beginner", "chatbot"}},
		{Name: "template-2", Tags: []string{"advanced", "agent"}},
		{Name: "template-3", Tags: []string{"beginner", "agent"}},
		{Name: "no-tags", Tags: nil},
	}

	tests := []struct {
		name string
		tags []string
		want int
	}{
		{"single tag match", []string{"beginner"}, 2},
		{"multiple tags any match", []string{"chatbot", "agent"}, 3},
		{"case insensitive", []string{"BEGINNER"}, 2},
		{"empty tags returns all", []string{}, 4},
		{"nil tags returns all", nil, 4},
		{"no match", []string{"nonexistent"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByTags(templates, tt.tags)
			if len(got) != tt.want {
				t.Errorf("FilterByTags(%v) returned %d templates, want %d", tt.tags, len(got), tt.want)
			}
		})
	}
}

func TestSearchTemplates(t *testing.T) {
	templates := []Template{
		{Name: "chatbot", DisplayName: "Basic Chatbot", Description: "A simple chat interface"},
		{Name: "agent", DisplayName: "Smart Agent", Description: "An intelligent agent"},
		{Name: "assistant", DisplayName: "AI Assistant", Description: "Personal chatbot assistant"},
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"search by name", "agent", 1},
		{"search by display name", "Smart", 1},
		{"search by description", "intelligent", 1},
		{"case insensitive", "AGENT", 1},
		{"partial match", "chat", 2}, // matches chatbot and description "Personal chatbot assistant"
		{"empty query returns all", "", 3},
		{"no match", "nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SearchTemplates(templates, tt.query)
			if len(got) != tt.want {
				t.Errorf("SearchTemplates(%q) returned %d templates, want %d", tt.query, len(got), tt.want)
			}
		})
	}
}

func TestDefaultFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various files
	files := []struct {
		name  string
		isDir bool
	}{
		{"config.yaml", false},
		{"data.json", false},
		{"image.png", false},
		{"prompts", true},
		{".hidden", false},
		{TemplateFileName, false}, // Should be skipped
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f.name)
		if f.isDir {
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatalf("failed to create dir %s: %v", f.name, err)
			}
		} else {
			if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
				t.Fatalf("failed to create file %s: %v", f.name, err)
			}
		}
	}

	d := &Discoverer{}
	specs := d.defaultFiles(tmpDir)

	// Should have: config.yaml (render=true), data.json (render=true), image.png (render=false), prompts/ (render=false)
	// Should NOT have: .hidden, template.yaml
	if len(specs) != 4 {
		t.Errorf("defaultFiles() returned %d specs, want 4", len(specs))
	}

	// Check specific files
	for _, spec := range specs {
		switch spec.Path {
		case "config.yaml":
			if !spec.Render {
				t.Errorf("config.yaml should have Render=true")
			}
		case "image.png":
			if spec.Render {
				t.Errorf("image.png should have Render=false")
			}
		case "prompts/":
			if spec.Render {
				t.Errorf("prompts/ should have Render=false")
			}
		}
	}
}

func TestLoadTemplateErrors(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDiscoverer(tmpDir, "templates")

	// Test missing template.yaml
	_, err := d.loadTemplate(tmpDir)
	if err == nil {
		t.Error("loadTemplate() should fail for missing template.yaml")
	}

	// Test invalid YAML
	if err := os.WriteFile(filepath.Join(tmpDir, "template.yaml"), []byte("invalid: yaml: content"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	_, err = d.loadTemplate(tmpDir)
	if err == nil {
		t.Error("loadTemplate() should fail for invalid YAML")
	}

	// Test missing name
	missingNameYAML := "metadata:\n  version: 1.0.0"
	err = os.WriteFile(filepath.Join(tmpDir, "template.yaml"), []byte(missingNameYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	_, err = d.loadTemplate(tmpDir)
	if err == nil {
		t.Error("loadTemplate() should fail for missing metadata.name")
	}
}

func TestDiscoverEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	d := NewDiscoverer(tmpDir, "templates")
	templates, err := d.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(templates) != 0 {
		t.Errorf("Discover() returned %d templates, want 0", len(templates))
	}
}

func TestLoadTemplateDefaultDisplayName(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template.yaml without displayName
	templateYAML := `apiVersion: arena.altairalabs.ai/v1alpha1
kind: ArenaTemplate
metadata:
  name: my-template
spec:
  description: Test template
`
	if err := os.WriteFile(filepath.Join(tmpDir, "template.yaml"), []byte(templateYAML), 0644); err != nil {
		t.Fatalf("failed to write template.yaml: %v", err)
	}

	d := NewDiscoverer(tmpDir, "")
	template, err := d.loadTemplate(tmpDir)
	if err != nil {
		t.Fatalf("loadTemplate() error = %v", err)
	}

	// DisplayName should default to Name
	if template.DisplayName != "my-template" {
		t.Errorf("DisplayName = %q, want %q (should default to name)", template.DisplayName, "my-template")
	}
}
