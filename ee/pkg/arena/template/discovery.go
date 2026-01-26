/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// IndexFileName is the name of the index file.
	IndexFileName = ".template-index.yaml"

	// TemplateFileName is the name of the template definition file.
	TemplateFileName = "template.yaml"

	// DefaultTemplatesPath is the default path to templates within a source.
	DefaultTemplatesPath = "templates"
)

// Discoverer finds templates in a source directory.
type Discoverer struct {
	// SourcePath is the root path of the template source.
	SourcePath string

	// TemplatesPath is the relative path to templates within the source.
	TemplatesPath string
}

// NewDiscoverer creates a new template discoverer.
func NewDiscoverer(sourcePath, templatesPath string) *Discoverer {
	if templatesPath == "" {
		templatesPath = DefaultTemplatesPath
	}
	// Clean trailing slashes
	templatesPath = strings.TrimSuffix(templatesPath, "/")

	return &Discoverer{
		SourcePath:    sourcePath,
		TemplatesPath: templatesPath,
	}
}

// Discover finds all templates in the source directory.
// It first looks for an index file, then falls back to auto-discovery.
func (d *Discoverer) Discover() ([]Template, error) {
	// Check if index file exists
	indexPath := filepath.Join(d.SourcePath, IndexFileName)
	if _, err := os.Stat(indexPath); err == nil {
		return d.discoverFromIndex(indexPath)
	}

	// Also check in templates path
	indexPath = filepath.Join(d.SourcePath, d.TemplatesPath, IndexFileName)
	if _, err := os.Stat(indexPath); err == nil {
		return d.discoverFromIndex(indexPath)
	}

	// Fall back to auto-discovery
	return d.autoDiscover()
}

// discoverFromIndex reads templates listed in the index file.
func (d *Discoverer) discoverFromIndex(indexPath string) ([]Template, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	var index Index
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}

	templates := make([]Template, 0, len(index.Templates))
	for _, entry := range index.Templates {
		templatePath := entry.Path
		if !filepath.IsAbs(templatePath) {
			// Path in index is relative to index file location
			indexDir := filepath.Dir(indexPath)
			templatePath = filepath.Join(indexDir, entry.Path)
		}

		template, err := d.loadTemplate(templatePath)
		if err != nil {
			// Log warning but continue with other templates
			fmt.Printf("Warning: failed to load template %s: %v\n", entry.Name, err)
			continue
		}
		templates = append(templates, *template)
	}

	return templates, nil
}

// autoDiscover scans the templates directory for template.yaml files.
func (d *Discoverer) autoDiscover() ([]Template, error) {
	templatesDir := filepath.Join(d.SourcePath, d.TemplatesPath)

	// Check if templates directory exists
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		// Try root directory
		templatesDir = d.SourcePath
	}

	templates := make([]Template, 0)

	// Walk the directory looking for template.yaml files
	err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't read
		}

		if info.IsDir() {
			return nil
		}

		if info.Name() == TemplateFileName {
			templateDir := filepath.Dir(path)
			template, err := d.loadTemplate(templateDir)
			if err != nil {
				// Log warning but continue
				fmt.Printf("Warning: failed to load template at %s: %v\n", templateDir, err)
				return nil
			}
			templates = append(templates, *template)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan templates directory: %w", err)
	}

	return templates, nil
}

// loadTemplate loads a template from a directory containing template.yaml.
func (d *Discoverer) loadTemplate(templateDir string) (*Template, error) {
	templatePath := filepath.Join(templateDir, TemplateFileName)

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	var def TemplateDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse template file: %w", err)
	}

	// Validate required fields
	if def.Metadata.Name == "" {
		return nil, fmt.Errorf("template metadata.name is required")
	}

	// Calculate relative path from source root
	relPath, err := filepath.Rel(d.SourcePath, templateDir)
	if err != nil {
		relPath = templateDir // Fall back to absolute path
	}

	template := &Template{
		Name:        def.Metadata.Name,
		Version:     def.Metadata.Version,
		DisplayName: def.Spec.DisplayName,
		Description: def.Spec.Description,
		Category:    def.Spec.Category,
		Tags:        def.Spec.Tags,
		Variables:   def.Spec.Variables,
		Files:       def.Spec.Files,
		Path:        relPath,
	}

	// Use name as display name if not specified
	if template.DisplayName == "" {
		template.DisplayName = template.Name
	}

	// Default files if not specified
	if len(template.Files) == 0 {
		template.Files = d.defaultFiles(templateDir)
	}

	return template, nil
}

// defaultFiles returns default file specs by scanning the template directory.
func (d *Discoverer) defaultFiles(templateDir string) []FileSpec {
	files := make([]FileSpec, 0)

	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip the template.yaml itself and hidden files
		if name == TemplateFileName || strings.HasPrefix(name, ".") {
			continue
		}

		spec := FileSpec{
			Path:   name,
			Render: d.shouldRender(name),
		}

		// For directories, add trailing slash
		if entry.IsDir() {
			spec.Path = name + "/"
		}

		files = append(files, spec)
	}

	return files
}

// shouldRender determines if a file should be rendered based on its name.
func (d *Discoverer) shouldRender(name string) bool {
	// Files that should be rendered by default
	renderExtensions := []string{".yaml", ".yml", ".json", ".txt", ".md"}

	for _, ext := range renderExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}

	return false
}

// GetTemplateByName finds a template by name from a list.
func GetTemplateByName(templates []Template, name string) *Template {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}
	return nil
}

// FilterByCategory returns templates matching the given category.
func FilterByCategory(templates []Template, category string) []Template {
	if category == "" {
		return templates
	}

	result := make([]Template, 0)
	for _, t := range templates {
		if strings.EqualFold(t.Category, category) {
			result = append(result, t)
		}
	}
	return result
}

// FilterByTags returns templates containing any of the given tags.
func FilterByTags(templates []Template, tags []string) []Template {
	if len(tags) == 0 {
		return templates
	}

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[strings.ToLower(tag)] = true
	}

	result := make([]Template, 0)
	for _, t := range templates {
		for _, templateTag := range t.Tags {
			if tagSet[strings.ToLower(templateTag)] {
				result = append(result, t)
				break
			}
		}
	}
	return result
}

// SearchTemplates searches templates by name or description.
func SearchTemplates(templates []Template, query string) []Template {
	if query == "" {
		return templates
	}

	query = strings.ToLower(query)
	result := make([]Template, 0)

	for _, t := range templates {
		if strings.Contains(strings.ToLower(t.Name), query) ||
			strings.Contains(strings.ToLower(t.DisplayName), query) ||
			strings.Contains(strings.ToLower(t.Description), query) {
			result = append(result, t)
		}
	}

	return result
}
