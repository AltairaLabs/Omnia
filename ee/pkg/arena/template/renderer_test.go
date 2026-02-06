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
	"strings"
	"testing"
)

func TestNewRenderer(t *testing.T) {
	r := NewRenderer()
	if r == nil {
		t.Fatal("NewRenderer() returned nil")
	}
	if r.FuncMap == nil {
		t.Error("FuncMap should not be nil")
	}
}

func TestRenderString(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name      string
		content   string
		variables map[string]any
		want      string
		wantErr   bool
	}{
		{
			name:      "simple substitution",
			content:   "Hello, {{ .name }}!",
			variables: map[string]any{"name": "World"},
			want:      "Hello, World!",
		},
		{
			name:      "multiple variables",
			content:   "{{ .greeting }}, {{ .name }}!",
			variables: map[string]any{"greeting": "Hi", "name": "Test"},
			want:      "Hi, Test!",
		},
		{
			name:      "empty variables",
			content:   "Static text",
			variables: map[string]any{},
			want:      "Static text",
		},
		{
			name:      "with lower function",
			content:   "{{ lower .name }}",
			variables: map[string]any{"name": "TEST"},
			want:      "test",
		},
		{
			name:      "with upper function",
			content:   "{{ upper .name }}",
			variables: map[string]any{"name": "test"},
			want:      "TEST",
		},
		{
			name:      "invalid template syntax",
			content:   "{{ .name }",
			variables: map[string]any{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RenderString(tt.content, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("RenderString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateFunctions(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name      string
		content   string
		variables map[string]any
		want      string
	}{
		{"trimSpace", "{{ trimSpace .val }}", map[string]any{"val": "  test  "}, "test"},
		{"trimPrefix", "{{ trimPrefix \"pre-\" .val }}", map[string]any{"val": "pre-test"}, "test"},
		{"trimSuffix", "{{ trimSuffix \"-suf\" .val }}", map[string]any{"val": "test-suf"}, "test"},
		{"replace", "{{ replace \"-\" \"_\" .val }}", map[string]any{"val": "a-b-c"}, "a_b_c"},
		{"contains true", "{{ contains .val \"test\" }}", map[string]any{"val": "testing"}, "true"},
		{"contains false", "{{ contains .val \"xyz\" }}", map[string]any{"val": "testing"}, "false"},
		{"hasPrefix", "{{ hasPrefix .val \"pre\" }}", map[string]any{"val": "prefix"}, "true"},
		{"hasSuffix", "{{ hasSuffix .val \"fix\" }}", map[string]any{"val": "suffix"}, "true"},
		{"join", "{{ join \",\" .val }}", map[string]any{"val": []string{"a", "b", "c"}}, "a,b,c"},
		{"toString", "{{ toString .val }}", map[string]any{"val": 123}, "123"},
		{"quote", "{{ quote .val }}", map[string]any{"val": "test"}, "\"test\""},
		{"kebabCase", "{{ kebabCase .val }}", map[string]any{"val": "myTestName"}, "my-test-name"},
		{"snakeCase", "{{ snakeCase .val }}", map[string]any{"val": "myTestName"}, "my_test_name"},
		{"camelCase", "{{ camelCase .val }}", map[string]any{"val": "my-test-name"}, "myTestName"},
		{"default with value", "{{ default \"fallback\" .val }}", map[string]any{"val": "value"}, "value"},
		{"default without value", "{{ default \"fallback\" .val }}", map[string]any{"val": ""}, "fallback"},
		{"ternary true", "{{ ternary \"yes\" \"no\" .cond }}", map[string]any{"cond": true}, "yes"},
		{"ternary false", "{{ ternary \"yes\" \"no\" .cond }}", map[string]any{"cond": false}, "no"},
		{"indent", "{{ indent 2 .val }}", map[string]any{"val": "line1\nline2"}, "  line1\n  line2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RenderString(tt.content, tt.variables)
			if err != nil {
				t.Errorf("RenderString() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("RenderString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToInt(t *testing.T) {
	fm := defaultFuncMap()
	toInt := fm["toInt"].(func(any) int)

	tests := []struct {
		name string
		val  any
		want int
	}{
		{"int", 42, 42},
		{"int64", int64(42), 42},
		{"float64", float64(42.7), 42},
		{"string", "42", 42},
		{"invalid string", "not a number", 0},
		{"unknown type", []int{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt(tt.val)
			if got != tt.want {
				t.Errorf("toInt(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestToFloat(t *testing.T) {
	fm := defaultFuncMap()
	toFloat := fm["toFloat"].(func(any) float64)

	tests := []struct {
		name string
		val  any
		want float64
	}{
		{"float64", float64(3.14), 3.14},
		{"float32", float32(3.14), float64(float32(3.14))},
		{"int", 42, 42.0},
		{"int64", int64(42), 42.0},
		{"string", "3.14", 3.14},
		{"invalid string", "not a number", 0},
		{"unknown type", []int{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat(tt.val)
			if got != tt.want {
				t.Errorf("toFloat(%v) = %f, want %f", tt.val, got, tt.want)
			}
		})
	}
}

func TestToBool(t *testing.T) {
	fm := defaultFuncMap()
	toBool := fm["toBool"].(func(any) bool)

	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string false", "false", false},
		{"int non-zero", 1, true},
		{"int zero", 0, false},
		{"unknown type", []int{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toBool(tt.val)
			if got != tt.want {
				t.Errorf("toBool(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestCaseConversions(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"kebab from camel", toKebabCase, "myTestName", "my-test-name"},
		{"kebab from underscore", toKebabCase, "my_test_name", "my-test-name"},
		{"kebab from space", toKebabCase, "my test name", "my-test-name"},
		{"snake from camel", toSnakeCase, "myTestName", "my_test_name"},
		{"snake from dash", toSnakeCase, "my-test-name", "my_test_name"},
		{"snake from space", toSnakeCase, "my test name", "my_test_name"},
		{"camel from dash", toCamelCase, "my-test-name", "myTestName"},
		{"camel from underscore", toCamelCase, "my_test_name", "myTestName"},
		{"camel from space", toCamelCase, "my test name", "myTestName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.in)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateVariables(t *testing.T) {
	tests := []struct {
		name      string
		template  *Template
		variables map[string]any
		wantErrs  int
	}{
		{
			name: "all required present",
			template: &Template{
				Variables: []Variable{
					{Name: "name", Type: VariableTypeString, Required: true},
				},
			},
			variables: map[string]any{"name": "test"},
			wantErrs:  0,
		},
		{
			name: "missing required",
			template: &Template{
				Variables: []Variable{
					{Name: "name", Type: VariableTypeString, Required: true},
				},
			},
			variables: map[string]any{},
			wantErrs:  1,
		},
		{
			name: "string pattern valid",
			template: &Template{
				Variables: []Variable{
					{Name: "name", Type: VariableTypeString, Pattern: "^[a-z]+$"},
				},
			},
			variables: map[string]any{"name": "test"},
			wantErrs:  0,
		},
		{
			name: "string pattern invalid",
			template: &Template{
				Variables: []Variable{
					{Name: "name", Type: VariableTypeString, Pattern: "^[a-z]+$"},
				},
			},
			variables: map[string]any{"name": "TEST123"},
			wantErrs:  1,
		},
		{
			name: "number in range",
			template: &Template{
				Variables: []Variable{
					{Name: "count", Type: VariableTypeNumber, Min: "0", Max: "10"},
				},
			},
			variables: map[string]any{"count": 5},
			wantErrs:  0,
		},
		{
			name: "number below min",
			template: &Template{
				Variables: []Variable{
					{Name: "count", Type: VariableTypeNumber, Min: "0", Max: "10"},
				},
			},
			variables: map[string]any{"count": -1},
			wantErrs:  1,
		},
		{
			name: "number above max",
			template: &Template{
				Variables: []Variable{
					{Name: "count", Type: VariableTypeNumber, Min: "0", Max: "10"},
				},
			},
			variables: map[string]any{"count": 15},
			wantErrs:  1,
		},
		{
			name: "enum valid",
			template: &Template{
				Variables: []Variable{
					{Name: "type", Type: VariableTypeEnum, Options: []string{"a", "b", "c"}},
				},
			},
			variables: map[string]any{"type": "a"},
			wantErrs:  0,
		},
		{
			name: "enum invalid",
			template: &Template{
				Variables: []Variable{
					{Name: "type", Type: VariableTypeEnum, Options: []string{"a", "b", "c"}},
				},
			},
			variables: map[string]any{"type": "invalid"},
			wantErrs:  1,
		},
		{
			name: "boolean valid true",
			template: &Template{
				Variables: []Variable{
					{Name: "flag", Type: VariableTypeBoolean},
				},
			},
			variables: map[string]any{"flag": true},
			wantErrs:  0,
		},
		{
			name: "boolean string true",
			template: &Template{
				Variables: []Variable{
					{Name: "flag", Type: VariableTypeBoolean},
				},
			},
			variables: map[string]any{"flag": "true"},
			wantErrs:  0,
		},
		{
			name: "boolean invalid string",
			template: &Template{
				Variables: []Variable{
					{Name: "flag", Type: VariableTypeBoolean},
				},
			},
			variables: map[string]any{"flag": "yes"},
			wantErrs:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateVariables(tt.template, tt.variables)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateVariables() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	template := &Template{
		Variables: []Variable{
			{Name: "name", Type: VariableTypeString, Default: "default-name"},
			{Name: "count", Type: VariableTypeNumber, Default: "10"},
			{Name: "enabled", Type: VariableTypeBoolean, Default: "true"},
			{Name: "type", Type: VariableTypeEnum, Default: "option1", Options: []string{"option1", "option2"}},
		},
	}

	// Test with empty variables
	result := ApplyDefaults(template, map[string]any{})

	if result["name"] != "default-name" {
		t.Errorf("name = %v, want %q", result["name"], "default-name")
	}
	if result["count"] != float64(10) {
		t.Errorf("count = %v, want %v", result["count"], float64(10))
	}
	if result["enabled"] != true {
		t.Errorf("enabled = %v, want %v", result["enabled"], true)
	}
	if result["type"] != "option1" {
		t.Errorf("type = %v, want %q", result["type"], "option1")
	}

	// Test with provided values (should not override)
	result = ApplyDefaults(template, map[string]any{"name": "custom-name"})
	if result["name"] != "custom-name" {
		t.Errorf("name = %v, want %q", result["name"], "custom-name")
	}
}

func TestRender(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a template file
	templateDir := filepath.Join(tmpDir, "templates", "test")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("failed to create template dir: %v", err)
	}

	// Create config.yaml
	configContent := "name: {{ .projectName }}\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(templateDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	r := NewRenderer()

	template := &Template{
		Name: "test",
		Path: "templates/test",
		Files: []FileSpec{
			{Path: "config.yaml", Render: true},
		},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: templateDir,
		Variables:  map[string]any{"projectName": "my-project"},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(output.Errors) > 0 {
		t.Errorf("Render() had errors: %v", output.Errors)
	}

	content, ok := output.Files["config.yaml"]
	if !ok {
		t.Fatal("config.yaml not in output")
	}

	if !strings.Contains(content, "name: my-project") {
		t.Errorf("rendered content = %q, want to contain 'name: my-project'", content)
	}
}

func TestRenderNilTemplate(t *testing.T) {
	r := NewRenderer()

	_, err := r.Render(&RenderInput{Template: nil})
	if err == nil {
		t.Error("Render() should fail for nil template")
	}
}

func TestWriteOutput(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewRenderer()

	output := &RenderOutput{
		Files: map[string]string{
			"config.yaml":     "name: test",
			"nested/data.txt": "nested content",
		},
	}

	if err := r.WriteOutput(output, tmpDir); err != nil {
		t.Fatalf("WriteOutput() error = %v", err)
	}

	// Verify files exist
	content, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Errorf("failed to read config.yaml: %v", err)
	}
	if string(content) != "name: test" {
		t.Errorf("config.yaml = %q, want %q", string(content), "name: test")
	}

	content, err = os.ReadFile(filepath.Join(tmpDir, "nested/data.txt"))
	if err != nil {
		t.Errorf("failed to read nested/data.txt: %v", err)
	}
	if string(content) != "nested content" {
		t.Errorf("nested/data.txt = %q, want %q", string(content), "nested content")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "nested", "dest.txt")

	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Errorf("failed to read dest: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("dest content = %q, want %q", string(content), "content")
	}
}

func TestPreview(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template file
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("name: {{ .name }}"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	r := NewRenderer()
	template := &Template{
		Name:  "test",
		Files: []FileSpec{{Path: "config.yaml", Render: true}},
	}

	files, err := r.Preview(&RenderInput{
		Template:   template,
		SourcePath: tmpDir,
		Variables:  map[string]any{"name": "preview-test"},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if content, ok := files["config.yaml"]; !ok || !strings.Contains(content, "preview-test") {
		t.Errorf("Preview() did not render correctly: %v", files)
	}
}

func TestRenderDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a template with a directory
	templateDir := filepath.Join(tmpDir, "templates", "test")
	promptsDir := filepath.Join(templateDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("failed to create prompts dir: %v", err)
	}

	// Create template.yaml (should be skipped)
	tmplYAML := "apiVersion: v1\nkind: ArenaTemplate\nmetadata:\n  name: test"
	if err := os.WriteFile(filepath.Join(templateDir, "template.yaml"), []byte(tmplYAML), 0644); err != nil {
		t.Fatalf("failed to write template.yaml: %v", err)
	}

	// Create files in prompts directory
	prompt1 := "system: You are {{ .name }}"
	if err := os.WriteFile(filepath.Join(promptsDir, "main.prompt.yaml"), []byte(prompt1), 0644); err != nil {
		t.Fatalf("failed to write prompt: %v", err)
	}

	// Create a subdirectory with a file
	subDir := filepath.Join(promptsDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.yaml"), []byte("nested: value"), 0644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	r := NewRenderer()

	template := &Template{
		Name: "test",
		Path: "templates/test",
		Files: []FileSpec{
			{Path: "prompts/", Render: true},
		},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: templateDir,
		Variables:  map[string]any{"name": "test-assistant"},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(output.Errors) > 0 {
		t.Errorf("Render() had errors: %v", output.Errors)
	}

	// Should have rendered both files
	if content, ok := output.Files["prompts/main.prompt.yaml"]; !ok {
		t.Error("prompts/main.prompt.yaml not in output")
	} else if !strings.Contains(content, "test-assistant") {
		t.Errorf("prompt not rendered: %q", content)
	}

	if _, ok := output.Files["prompts/sub/nested.yaml"]; !ok {
		t.Error("prompts/sub/nested.yaml not in output")
	}

	// template.yaml should NOT be in output
	if _, ok := output.Files["template.yaml"]; ok {
		t.Error("template.yaml should be skipped")
	}
}

func TestRenderDirectoryWithoutRender(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a template with a directory that should not be rendered
	templateDir := filepath.Join(tmpDir, "templates", "test")
	scenariosDir := filepath.Join(templateDir, "scenarios")
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		t.Fatalf("failed to create scenarios dir: %v", err)
	}

	// Create a file with template syntax that should NOT be rendered
	scenario := "name: {{ .name }}\n" // This should remain as-is
	if err := os.WriteFile(filepath.Join(scenariosDir, "test.yaml"), []byte(scenario), 0644); err != nil {
		t.Fatalf("failed to write scenario: %v", err)
	}

	r := NewRenderer()

	template := &Template{
		Name: "test",
		Path: "templates/test",
		Files: []FileSpec{
			{Path: "scenarios/", Render: false}, // Don't render templates
		},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: templateDir,
		Variables:  map[string]any{"name": "should-not-appear"},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Content should still have the template syntax
	if content, ok := output.Files["scenarios/test.yaml"]; !ok {
		t.Error("scenarios/test.yaml not in output")
	} else if !strings.Contains(content, "{{ .name }}") {
		t.Errorf("template syntax should be preserved: %q", content)
	}
}

func TestRenderMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	r := NewRenderer()

	template := &Template{
		Name: "test",
		Path: "templates/test",
		Files: []FileSpec{
			{Path: "nonexistent.yaml", Render: true},
		},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: tmpDir,
		Variables:  map[string]any{},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Should have errors for missing file
	if len(output.Errors) == 0 {
		t.Error("Render() should report errors for missing files")
	}
}

func TestRenderMissingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	r := NewRenderer()

	template := &Template{
		Name: "test",
		Path: "templates/test",
		Files: []FileSpec{
			{Path: "nonexistent/", Render: true},
		},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: tmpDir,
		Variables:  map[string]any{},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Should have no files and no errors (missing dir is just empty)
	if len(output.Files) > 0 {
		t.Errorf("Render() should have no files: %v", output.Files)
	}
}

func TestCopyFileSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	err := CopyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "dest.txt"))
	if err == nil {
		t.Error("CopyFile() should fail for nonexistent source")
	}
}

func TestPreviewMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	r := NewRenderer()
	template := &Template{
		Name:  "test",
		Files: []FileSpec{{Path: "missing.yaml", Render: true}},
	}

	_, err := r.Preview(&RenderInput{
		Template:   template,
		SourcePath: tmpDir,
		Variables:  map[string]any{},
	})

	// Preview returns error for missing files
	if err == nil {
		t.Error("Preview() should return error for missing files")
	}
}

func TestRenderTemplateError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with invalid template syntax
	if err := os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte("{{ .name }"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	r := NewRenderer()
	template := &Template{
		Name:  "test",
		Files: []FileSpec{{Path: "bad.yaml", Render: true}},
	}

	input := &RenderInput{
		Template:   template,
		SourcePath: tmpDir,
		Variables:  map[string]any{"name": "test"},
	}

	output, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Should have errors for bad template
	if len(output.Errors) == 0 {
		t.Error("Render() should report errors for invalid templates")
	}
}

func TestValidateNumberVarNonNumericString(t *testing.T) {
	template := &Template{
		Variables: []Variable{
			{Name: "count", Type: VariableTypeNumber},
		},
	}

	errs := ValidateVariables(template, map[string]any{"count": "not-a-number"})
	if len(errs) == 0 {
		t.Error("ValidateVariables() should fail for non-numeric string")
	}
}

func TestValidateStringVarNonString(t *testing.T) {
	template := &Template{
		Variables: []Variable{
			{Name: "name", Type: VariableTypeString},
		},
	}

	errs := ValidateVariables(template, map[string]any{"name": 123})
	if len(errs) == 0 {
		t.Error("ValidateVariables() should fail for non-string value")
	}
}
