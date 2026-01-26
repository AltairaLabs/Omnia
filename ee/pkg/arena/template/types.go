/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package template provides template discovery and rendering for Arena.
package template

// TemplateVariableType defines the type of a template variable.
type TemplateVariableType string

const (
	// VariableTypeString is a string variable.
	VariableTypeString TemplateVariableType = "string"
	// VariableTypeNumber is a numeric variable.
	VariableTypeNumber TemplateVariableType = "number"
	// VariableTypeBoolean is a boolean variable.
	VariableTypeBoolean TemplateVariableType = "boolean"
	// VariableTypeEnum is an enumeration variable.
	VariableTypeEnum TemplateVariableType = "enum"
)

// Variable defines a configurable parameter for a template.
type Variable struct {
	// Name is the variable name used in templates.
	Name string `json:"name" yaml:"name"`

	// Type is the variable type.
	Type TemplateVariableType `json:"type" yaml:"type"`

	// Description explains the purpose of this variable.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Required indicates whether the variable must be provided.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Default is the default value for the variable.
	Default string `json:"default,omitempty" yaml:"default,omitempty"`

	// Pattern is a regex pattern for validating string values.
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`

	// Options are the allowed values for enum variables.
	Options []string `json:"options,omitempty" yaml:"options,omitempty"`

	// Min is the minimum value for number variables (as string).
	Min string `json:"min,omitempty" yaml:"min,omitempty"`

	// Max is the maximum value for number variables (as string).
	Max string `json:"max,omitempty" yaml:"max,omitempty"`
}

// FileSpec defines how a file in the template should be processed.
type FileSpec struct {
	// Path is the path to the file or directory within the template.
	Path string `json:"path" yaml:"path"`

	// Render indicates whether to apply Go template rendering.
	Render bool `json:"render" yaml:"render"`
}

// TemplateSpec is the specification section of a template.yaml file.
type TemplateSpec struct {
	// DisplayName is the human-readable name.
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty"`

	// Description explains what the template does.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Category groups templates by type.
	Category string `json:"category,omitempty" yaml:"category,omitempty"`

	// Tags are searchable labels for the template.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Variables are the configurable parameters.
	Variables []Variable `json:"variables,omitempty" yaml:"variables,omitempty"`

	// Files specifies which files to include and how to process them.
	Files []FileSpec `json:"files,omitempty" yaml:"files,omitempty"`
}

// TemplateMetadata is the metadata section of a template.yaml file.
type TemplateMetadataYAML struct {
	// Name is the unique name of the template.
	Name string `json:"name" yaml:"name"`

	// Version is the semantic version of the template.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// TemplateDefinition represents a parsed template.yaml file.
type TemplateDefinition struct {
	// APIVersion is the API version (e.g., arena.altairalabs.ai/v1alpha1).
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`

	// Kind is the resource kind (ArenaTemplate).
	Kind string `json:"kind" yaml:"kind"`

	// Metadata contains name and version.
	Metadata TemplateMetadataYAML `json:"metadata" yaml:"metadata"`

	// Spec contains the template specification.
	Spec TemplateSpec `json:"spec" yaml:"spec"`
}

// Template represents a discovered template with its metadata and path.
type Template struct {
	// Name is the unique name of the template.
	Name string `json:"name"`

	// Version is the semantic version of the template.
	Version string `json:"version,omitempty"`

	// DisplayName is the human-readable name.
	DisplayName string `json:"displayName,omitempty"`

	// Description explains what the template does.
	Description string `json:"description,omitempty"`

	// Category groups templates by type.
	Category string `json:"category,omitempty"`

	// Tags are searchable labels for the template.
	Tags []string `json:"tags,omitempty"`

	// Variables are the configurable parameters.
	Variables []Variable `json:"variables,omitempty"`

	// Files specifies which files to include and how to process them.
	Files []FileSpec `json:"files,omitempty"`

	// Path is the path to the template within the source.
	Path string `json:"path"`
}

// IndexEntry represents a template entry in the index file.
type IndexEntry struct {
	// Name is the template name.
	Name string `json:"name" yaml:"name"`

	// Path is the path to the template directory.
	Path string `json:"path" yaml:"path"`
}

// Index represents the .template-index.yaml file.
type Index struct {
	// APIVersion is the API version.
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`

	// Kind is the resource kind (TemplateIndex).
	Kind string `json:"kind" yaml:"kind"`

	// Templates lists all templates in the source.
	Templates []IndexEntry `json:"templates" yaml:"templates"`
}

// RenderInput contains the input for template rendering.
type RenderInput struct {
	// Template is the template to render.
	Template *Template

	// Variables are the variable values provided by the user.
	Variables map[string]any

	// SourcePath is the path to the template source files.
	SourcePath string

	// OutputPath is the path where rendered files will be written.
	OutputPath string
}

// RenderOutput contains the result of template rendering.
type RenderOutput struct {
	// Files contains the rendered file contents keyed by path.
	Files map[string]string

	// Errors contains any errors encountered during rendering.
	Errors []error
}
