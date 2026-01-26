/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestArenaTemplateSourceTypesRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	// Verify ArenaTemplateSource is registered
	gvk := GroupVersion.WithKind("ArenaTemplateSource")
	if !scheme.Recognizes(gvk) {
		t.Errorf("scheme does not recognize %v", gvk)
	}

	// Verify ArenaTemplateSourceList is registered
	gvkList := GroupVersion.WithKind("ArenaTemplateSourceList")
	if !scheme.Recognizes(gvkList) {
		t.Errorf("scheme does not recognize %v", gvkList)
	}
}

func TestArenaTemplateSourceTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      ArenaTemplateSourceType
		expected string
	}{
		{"Git", ArenaTemplateSourceTypeGit, "git"},
		{"OCI", ArenaTemplateSourceTypeOCI, "oci"},
		{"ConfigMap", ArenaTemplateSourceTypeConfigMap, "configmap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("ArenaTemplateSourceType %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestTemplateVariableTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      TemplateVariableType
		expected string
	}{
		{"String", TemplateVariableTypeString, "string"},
		{"Number", TemplateVariableTypeNumber, "number"},
		{"Boolean", TemplateVariableTypeBoolean, "boolean"},
		{"Enum", TemplateVariableTypeEnum, "enum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("TemplateVariableType %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestArenaTemplateSourcePhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      ArenaTemplateSourcePhase
		expected string
	}{
		{"Pending", ArenaTemplateSourcePhasePending, "Pending"},
		{"Ready", ArenaTemplateSourcePhaseReady, "Ready"},
		{"Fetching", ArenaTemplateSourcePhaseFetching, "Fetching"},
		{"Scanning", ArenaTemplateSourcePhaseScanning, "Scanning"},
		{"Error", ArenaTemplateSourcePhaseError, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("ArenaTemplateSourcePhase %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestArenaTemplateSourceCreation(t *testing.T) {
	source := &ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "test-ns",
		},
		Spec: ArenaTemplateSourceSpec{
			Type: ArenaTemplateSourceTypeGit,
			Git: &GitSource{
				URL: "https://github.com/test/repo",
				Ref: &GitReference{
					Branch: "main",
				},
			},
			SyncInterval:  "1h",
			TemplatesPath: "templates/",
		},
	}

	if source.Spec.Type != ArenaTemplateSourceTypeGit {
		t.Errorf("Type = %q, want %q", source.Spec.Type, ArenaTemplateSourceTypeGit)
	}
	if source.Spec.Git.URL != "https://github.com/test/repo" {
		t.Errorf("Git.URL = %q, want %q", source.Spec.Git.URL, "https://github.com/test/repo")
	}
	if source.Spec.Git.Ref.Branch != "main" {
		t.Errorf("Git.Ref.Branch = %q, want %q", source.Spec.Git.Ref.Branch, "main")
	}
	if source.Spec.SyncInterval != "1h" {
		t.Errorf("SyncInterval = %q, want %q", source.Spec.SyncInterval, "1h")
	}
}

func TestTemplateVariableCreation(t *testing.T) {
	variable := TemplateVariable{
		Name:        "projectName",
		Type:        TemplateVariableTypeString,
		Description: "Name for your project",
		Required:    true,
		Pattern:     "^[a-z][a-z0-9-]*$",
	}

	if variable.Name != "projectName" {
		t.Errorf("Name = %q, want %q", variable.Name, "projectName")
	}
	if variable.Type != TemplateVariableTypeString {
		t.Errorf("Type = %q, want %q", variable.Type, TemplateVariableTypeString)
	}
	if !variable.Required {
		t.Error("Required should be true")
	}
	if variable.Pattern != "^[a-z][a-z0-9-]*$" {
		t.Errorf("Pattern = %q, want %q", variable.Pattern, "^[a-z][a-z0-9-]*$")
	}
}

func TestTemplateVariableWithEnum(t *testing.T) {
	variable := TemplateVariable{
		Name:     "providerType",
		Type:     TemplateVariableTypeEnum,
		Default:  "mock",
		Options:  []string{"mock", "openai", "anthropic"},
		Required: false,
	}

	if variable.Type != TemplateVariableTypeEnum {
		t.Errorf("Type = %q, want %q", variable.Type, TemplateVariableTypeEnum)
	}
	if variable.Default != "mock" {
		t.Errorf("Default = %q, want %q", variable.Default, "mock")
	}
	if len(variable.Options) != 3 {
		t.Errorf("len(Options) = %d, want %d", len(variable.Options), 3)
	}
}

func TestTemplateVariableWithNumber(t *testing.T) {
	variable := TemplateVariable{
		Name:    "temperature",
		Type:    TemplateVariableTypeNumber,
		Default: "0.7",
		Min:     "0",
		Max:     "2",
	}

	if variable.Type != TemplateVariableTypeNumber {
		t.Errorf("Type = %q, want %q", variable.Type, TemplateVariableTypeNumber)
	}
	if variable.Min != "0" {
		t.Errorf("Min = %q, want %q", variable.Min, "0")
	}
	if variable.Max != "2" {
		t.Errorf("Max = %q, want %q", variable.Max, "2")
	}
}

func TestTemplateMetadataCreation(t *testing.T) {
	metadata := TemplateMetadata{
		Name:        "basic-chatbot",
		Version:     "1.0.0",
		DisplayName: "Basic Chatbot",
		Description: "A simple chatbot template",
		Category:    "chatbot",
		Tags:        []string{"chatbot", "beginner"},
		Path:        "templates/basic-chatbot",
		Variables: []TemplateVariable{
			{Name: "projectName", Type: TemplateVariableTypeString, Required: true},
		},
		Files: []TemplateFileSpec{
			{Path: "config.yaml", Render: true},
		},
	}

	if metadata.Name != "basic-chatbot" {
		t.Errorf("Name = %q, want %q", metadata.Name, "basic-chatbot")
	}
	if metadata.Category != "chatbot" {
		t.Errorf("Category = %q, want %q", metadata.Category, "chatbot")
	}
	if len(metadata.Tags) != 2 {
		t.Errorf("len(Tags) = %d, want %d", len(metadata.Tags), 2)
	}
	if len(metadata.Variables) != 1 {
		t.Errorf("len(Variables) = %d, want %d", len(metadata.Variables), 1)
	}
	if len(metadata.Files) != 1 {
		t.Errorf("len(Files) = %d, want %d", len(metadata.Files), 1)
	}
}

func TestTemplateFileSpecCreation(t *testing.T) {
	spec := TemplateFileSpec{
		Path:   "config.yaml",
		Render: true,
	}

	if spec.Path != "config.yaml" {
		t.Errorf("Path = %q, want %q", spec.Path, "config.yaml")
	}
	if !spec.Render {
		t.Error("Render should be true")
	}
}

func TestArenaTemplateSourceStatus(t *testing.T) {
	now := metav1.Now()
	status := ArenaTemplateSourceStatus{
		Phase:              ArenaTemplateSourcePhaseReady,
		ObservedGeneration: 5,
		TemplateCount:      3,
		Templates: []TemplateMetadata{
			{Name: "template-1", Path: "templates/template-1"},
		},
		LastFetchTime: &now,
		HeadVersion:   "abc123",
		Message:       "Successfully synced",
	}

	if status.Phase != ArenaTemplateSourcePhaseReady {
		t.Errorf("Phase = %q, want %q", status.Phase, ArenaTemplateSourcePhaseReady)
	}
	if status.TemplateCount != 3 {
		t.Errorf("TemplateCount = %d, want %d", status.TemplateCount, 3)
	}
	if len(status.Templates) != 1 {
		t.Errorf("len(Templates) = %d, want %d", len(status.Templates), 1)
	}
	if status.HeadVersion != "abc123" {
		t.Errorf("HeadVersion = %q, want %q", status.HeadVersion, "abc123")
	}
}

func TestArenaTemplateSourceListCreation(t *testing.T) {
	list := &ArenaTemplateSourceList{
		Items: []ArenaTemplateSource{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "source-1", Namespace: "ns-1"},
				Spec:       ArenaTemplateSourceSpec{Type: ArenaTemplateSourceTypeGit},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "source-2", Namespace: "ns-2"},
				Spec:       ArenaTemplateSourceSpec{Type: ArenaTemplateSourceTypeOCI},
			},
		},
	}

	if len(list.Items) != 2 {
		t.Errorf("len(Items) = %d, want %d", len(list.Items), 2)
	}
	if list.Items[0].Name != "source-1" {
		t.Errorf("Items[0].Name = %q, want %q", list.Items[0].Name, "source-1")
	}
	if list.Items[1].Spec.Type != ArenaTemplateSourceTypeOCI {
		t.Errorf("Items[1].Spec.Type = %q, want %q", list.Items[1].Spec.Type, ArenaTemplateSourceTypeOCI)
	}
}

func TestArenaTemplateSourceDeepCopy(t *testing.T) {
	original := &ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "test-ns",
		},
		Spec: ArenaTemplateSourceSpec{
			Type: ArenaTemplateSourceTypeGit,
			Git: &GitSource{
				URL: "https://github.com/test/repo",
			},
			SyncInterval: "1h",
		},
		Status: ArenaTemplateSourceStatus{
			Phase:         ArenaTemplateSourcePhaseReady,
			TemplateCount: 5,
		},
	}

	copied := original.DeepCopy()

	// Verify the copy is equal
	if copied.Name != original.Name {
		t.Errorf("copied.Name = %q, want %q", copied.Name, original.Name)
	}
	if copied.Spec.Type != original.Spec.Type {
		t.Errorf("copied.Spec.Type = %q, want %q", copied.Spec.Type, original.Spec.Type)
	}

	// Verify it's a deep copy
	copied.Spec.SyncInterval = "2h"
	if original.Spec.SyncInterval != "1h" {
		t.Errorf("original.Spec.SyncInterval was modified, got %q, want %q", original.Spec.SyncInterval, "1h")
	}
}

func TestArenaTemplateSourceListDeepCopy(t *testing.T) {
	original := &ArenaTemplateSourceList{
		Items: []ArenaTemplateSource{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "source-1"},
				Spec:       ArenaTemplateSourceSpec{Type: ArenaTemplateSourceTypeGit},
			},
		},
	}

	copied := original.DeepCopy()

	if len(copied.Items) != len(original.Items) {
		t.Errorf("len(copied.Items) = %d, want %d", len(copied.Items), len(original.Items))
	}

	// Modify the copy
	copied.Items[0].Name = "modified-source"
	if original.Items[0].Name != "source-1" {
		t.Errorf("original.Items[0].Name was modified, got %q, want %q", original.Items[0].Name, "source-1")
	}
}

func TestMinimalArenaTemplateSource(t *testing.T) {
	source := &ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-source",
			Namespace: "default",
		},
		Spec: ArenaTemplateSourceSpec{
			Type: ArenaTemplateSourceTypeGit,
			Git: &GitSource{
				URL: "https://github.com/test/repo",
			},
		},
	}

	if source.Spec.Type != ArenaTemplateSourceTypeGit {
		t.Errorf("Type = %q, want %q", source.Spec.Type, ArenaTemplateSourceTypeGit)
	}
	if source.Spec.OCI != nil {
		t.Error("OCI should be nil")
	}
	if source.Spec.ConfigMap != nil {
		t.Error("ConfigMap should be nil")
	}
	if source.Spec.Suspend {
		t.Error("Suspend should be false by default")
	}
}
