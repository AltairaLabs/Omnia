/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package template

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

// Renderer renders templates with variable substitution.
type Renderer struct {
	// FuncMap contains custom template functions.
	FuncMap template.FuncMap
}

// NewRenderer creates a new template renderer.
func NewRenderer() *Renderer {
	return &Renderer{
		FuncMap: defaultFuncMap(),
	}
}

// defaultFuncMap returns the default template functions.
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// String functions
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     strings.Title, //nolint:staticcheck // Title is intentional for templates
		"trimSpace": strings.TrimSpace,
		"trimPrefix": func(prefix, s string) string {
			return strings.TrimPrefix(s, prefix)
		},
		"trimSuffix": func(suffix, s string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"hasPrefix": func(s, prefix string) bool {
			return strings.HasPrefix(s, prefix)
		},
		"hasSuffix": func(s, suffix string) bool {
			return strings.HasSuffix(s, suffix)
		},
		"split": strings.Split,
		"join": func(sep string, elems []string) string {
			return strings.Join(elems, sep)
		},

		// Type conversion
		"toString": func(v any) string {
			return fmt.Sprintf("%v", v)
		},
		"toInt": func(v any) int {
			switch val := v.(type) {
			case int:
				return val
			case int64:
				return int(val)
			case float64:
				return int(val)
			case string:
				i, _ := strconv.Atoi(val)
				return i
			default:
				return 0
			}
		},
		"toFloat": func(v any) float64 {
			switch val := v.(type) {
			case float64:
				return val
			case float32:
				return float64(val)
			case int:
				return float64(val)
			case int64:
				return float64(val)
			case string:
				f, _ := strconv.ParseFloat(val, 64)
				return f
			default:
				return 0
			}
		},
		"toBool": func(v any) bool {
			switch val := v.(type) {
			case bool:
				return val
			case string:
				b, _ := strconv.ParseBool(val)
				return b
			case int:
				return val != 0
			default:
				return false
			}
		},

		// Default value
		"default": func(defaultVal, val any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},

		// Conditional
		"ternary": func(trueVal, falseVal any, condition bool) any {
			if condition {
				return trueVal
			}
			return falseVal
		},

		// YAML/indentation helpers
		"indent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"nindent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			return "\n" + pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},

		// Quote string for YAML
		"quote": func(s string) string {
			return fmt.Sprintf("%q", s)
		},

		// Convert to kebab-case
		"kebabCase": func(s string) string {
			return toKebabCase(s)
		},

		// Convert to snake_case
		"snakeCase": func(s string) string {
			return toSnakeCase(s)
		},

		// Convert to camelCase
		"camelCase": func(s string) string {
			return toCamelCase(s)
		},
	}
}

// toKebabCase converts a string to kebab-case.
func toKebabCase(s string) string {
	// Insert hyphens before uppercase letters and convert to lowercase
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	s = re.ReplaceAllString(s, "${1}-${2}")
	// Replace underscores and spaces with hyphens
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return strings.ToLower(s)
}

// toSnakeCase converts a string to snake_case.
func toSnakeCase(s string) string {
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	s = re.ReplaceAllString(s, "${1}_${2}")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return strings.ToLower(s)
}

// toCamelCase converts a string to camelCase.
func toCamelCase(s string) string {
	// Split by common separators
	parts := regexp.MustCompile(`[-_\s]+`).Split(s, -1)
	if len(parts) == 0 {
		return s
	}

	result := strings.ToLower(parts[0])
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}
	return result
}

// Render renders a template with the given variables.
func (r *Renderer) Render(input *RenderInput) (*RenderOutput, error) {
	output := &RenderOutput{
		Files:  make(map[string]string),
		Errors: make([]error, 0),
	}

	if input.Template == nil {
		return nil, fmt.Errorf("template is required")
	}

	// Process each file spec
	for _, fileSpec := range input.Template.Files {
		sourcePath := filepath.Join(input.SourcePath, fileSpec.Path)

		// Check if it's a directory
		info, err := os.Stat(sourcePath)
		if err != nil {
			output.Errors = append(output.Errors, fmt.Errorf("failed to stat %s: %w", fileSpec.Path, err))
			continue
		}

		if info.IsDir() {
			// Process directory recursively
			err := r.renderDirectory(sourcePath, fileSpec.Path, input, output, fileSpec.Render)
			if err != nil {
				output.Errors = append(output.Errors, err)
			}
		} else {
			// Process single file
			content, err := r.renderFile(sourcePath, input.Variables, fileSpec.Render)
			if err != nil {
				output.Errors = append(output.Errors, fmt.Errorf("failed to render %s: %w", fileSpec.Path, err))
				continue
			}
			output.Files[fileSpec.Path] = content
		}
	}

	return output, nil
}

// renderDirectory recursively renders all files in a directory.
func (r *Renderer) renderDirectory(
	sourcePath, relPath string, input *RenderInput, output *RenderOutput, render bool,
) error {
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't read
		}

		if info.IsDir() {
			return nil
		}

		// Skip template.yaml
		if info.Name() == TemplateFileName {
			return nil
		}

		// Calculate relative path for output
		fileRelPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return nil
		}

		// Combine with the spec's relative path
		outputPath := filepath.Join(relPath, fileRelPath)
		// Remove trailing slash from relPath if it was a directory spec
		outputPath = strings.TrimPrefix(outputPath, "/")

		content, err := r.renderFile(path, input.Variables, render)
		if err != nil {
			output.Errors = append(output.Errors, fmt.Errorf("failed to render %s: %w", outputPath, err))
			return nil
		}

		output.Files[outputPath] = content
		return nil
	})
}

// renderFile reads and optionally renders a single file.
func (r *Renderer) renderFile(path string, variables map[string]any, render bool) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if !render {
		return string(content), nil
	}

	return r.RenderString(string(content), variables)
}

// RenderString renders a template string with the given variables.
func (r *Renderer) RenderString(content string, variables map[string]any) (string, error) {
	tmpl, err := template.New("template").Funcs(r.FuncMap).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// WriteOutput writes the rendered output to the filesystem.
func (r *Renderer) WriteOutput(output *RenderOutput, outputPath string) error {
	for filePath, content := range output.Files {
		fullPath := filepath.Join(outputPath, filePath)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", filePath, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filePath, err)
		}
	}

	return nil
}

// validateStringVar validates a string variable.
func validateStringVar(v Variable, val any) error {
	strVal, ok := val.(string)
	if !ok {
		return fmt.Errorf("variable %q must be a string", v.Name)
	}
	if v.Pattern == "" {
		return nil
	}
	matched, err := regexp.MatchString(v.Pattern, strVal)
	if err != nil {
		return fmt.Errorf("variable %q has invalid pattern: %w", v.Name, err)
	}
	if !matched {
		return fmt.Errorf("variable %q does not match pattern %q", v.Name, v.Pattern)
	}
	return nil
}

// validateNumberVar validates a number variable.
func validateNumberVar(v Variable, val any) error {
	var numVal float64
	switch n := val.(type) {
	case float64:
		numVal = n
	case int:
		numVal = float64(n)
	case string:
		var err error
		numVal, err = strconv.ParseFloat(n, 64)
		if err != nil {
			return fmt.Errorf("variable %q must be a number", v.Name)
		}
	default:
		return fmt.Errorf("variable %q must be a number", v.Name)
	}
	if v.Min != "" {
		minVal, err := strconv.ParseFloat(v.Min, 64)
		if err == nil && numVal < minVal {
			return fmt.Errorf("variable %q must be >= %v", v.Name, v.Min)
		}
	}
	if v.Max != "" {
		maxVal, err := strconv.ParseFloat(v.Max, 64)
		if err == nil && numVal > maxVal {
			return fmt.Errorf("variable %q must be <= %v", v.Name, v.Max)
		}
	}
	return nil
}

// validateBooleanVar validates a boolean variable.
func validateBooleanVar(v Variable, val any) error {
	switch strVal := val.(type) {
	case bool:
		return nil
	case string:
		if strVal != "true" && strVal != "false" {
			return fmt.Errorf("variable %q must be a boolean", v.Name)
		}
		return nil
	default:
		return fmt.Errorf("variable %q must be a boolean", v.Name)
	}
}

// validateEnumVar validates an enum variable.
func validateEnumVar(v Variable, val any) error {
	strVal := fmt.Sprintf("%v", val)
	if !slices.Contains(v.Options, strVal) {
		return fmt.Errorf("variable %q must be one of: %v", v.Name, v.Options)
	}
	return nil
}

// ValidateVariables validates that all required variables are provided.
func ValidateVariables(tmpl *Template, variables map[string]any) []error {
	errs := make([]error, 0)

	for _, v := range tmpl.Variables {
		val, exists := variables[v.Name]

		// Check required
		if v.Required && (!exists || val == nil || val == "") {
			errs = append(errs, fmt.Errorf("variable %q is required", v.Name))
			continue
		}

		if !exists || val == nil {
			continue
		}

		// Type-specific validation
		var err error
		switch v.Type {
		case VariableTypeString:
			err = validateStringVar(v, val)
		case VariableTypeNumber:
			err = validateNumberVar(v, val)
		case VariableTypeBoolean:
			err = validateBooleanVar(v, val)
		case VariableTypeEnum:
			err = validateEnumVar(v, val)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// ApplyDefaults sets default values for missing variables.
func ApplyDefaults(tmpl *Template, variables map[string]any) map[string]any {
	result := make(map[string]any, len(variables))
	maps.Copy(result, variables)

	// Apply defaults
	for _, v := range tmpl.Variables {
		if _, exists := result[v.Name]; !exists && v.Default != "" {
			// Convert default to appropriate type
			switch v.Type {
			case VariableTypeNumber:
				if f, err := strconv.ParseFloat(v.Default, 64); err == nil {
					result[v.Name] = f
				}
			case VariableTypeBoolean:
				if b, err := strconv.ParseBool(v.Default); err == nil {
					result[v.Name] = b
				}
			default:
				result[v.Name] = v.Default
			}
		}
	}

	return result
}

// Preview generates a preview of rendered content without writing to disk.
func (r *Renderer) Preview(input *RenderInput) (map[string]string, error) {
	output, err := r.Render(input)
	if err != nil {
		return nil, err
	}

	if len(output.Errors) > 0 {
		return output.Files, output.Errors[0]
	}

	return output.Files, nil
}

// CopyFile copies a file from src to dst without rendering.
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
