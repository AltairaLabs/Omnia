/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	_ "embed" // Required for //go:embed directives to embed JSON schema files
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// Embed the authoritative PromptKit schemas from schemas/v1alpha1
//
//go:embed schemas/arena.json
var arenaSchema []byte

//go:embed schemas/promptconfig.json
var promptConfigSchema []byte

//go:embed schemas/provider.json
var providerSchema []byte

//go:embed schemas/scenario.json
var scenarioSchema []byte

//go:embed schemas/tool.json
var toolSchema []byte

//go:embed schemas/persona.json
var personaSchema []byte

//go:embed schemas/eval.json
var evalSchema []byte

//go:embed schemas/logging.json
var loggingSchema []byte

// Validator validates PromptKit YAML documents.
type Validator struct {
	log             logr.Logger
	dashboardAPIURL string
	httpClient      *http.Client
	schemaLoaders   map[string]gojsonschema.JSONLoader
	refPatterns     []*regexp.Regexp

	// Cache for project files
	fileCache   map[string]*cachedFiles
	fileCacheMu sync.RWMutex
	cacheTTL    time.Duration
}

type cachedFiles struct {
	files     []string
	expiresAt time.Time
}

// NewValidator creates a new Validator.
func NewValidator(dashboardAPIURL string, log logr.Logger) (*Validator, error) {
	// Load all embedded schemas keyed by kind
	schemaLoaders := map[string]gojsonschema.JSONLoader{
		"Arena":        gojsonschema.NewBytesLoader(arenaSchema),
		"PromptConfig": gojsonschema.NewBytesLoader(promptConfigSchema),
		"Provider":     gojsonschema.NewBytesLoader(providerSchema),
		"Scenario":     gojsonschema.NewBytesLoader(scenarioSchema),
		"Tool":         gojsonschema.NewBytesLoader(toolSchema),
		"Persona":      gojsonschema.NewBytesLoader(personaSchema),
		"Eval":         gojsonschema.NewBytesLoader(evalSchema),
		"Logging":      gojsonschema.NewBytesLoader(loggingSchema),
	}

	// Compile reference patterns for cross-reference validation
	refPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*-?\s*file:\s*["']?([^"'\s]+)["']?\s*$`),
	}

	return &Validator{
		log:             log.WithName("validator"),
		dashboardAPIURL: dashboardAPIURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		schemaLoaders: schemaLoaders,
		refPatterns:   refPatterns,
		fileCache:     make(map[string]*cachedFiles),
		cacheTTL:      30 * time.Second,
	}, nil
}

// ValidateDocument validates a document and returns diagnostics.
func (v *Validator) ValidateDocument(ctx context.Context, doc *Document, workspace, projectID string) []Diagnostic {
	var diagnostics []Diagnostic

	// 1. YAML syntax validation
	syntaxDiags := v.validateYAMLSyntax(doc)
	diagnostics = append(diagnostics, syntaxDiags...)
	if len(syntaxDiags) > 0 {
		// If there are syntax errors, skip further validation
		return diagnostics
	}

	// Parse YAML for schema validation
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(doc.Content), &parsed); err != nil {
		// Shouldn't happen since we already validated syntax
		return diagnostics
	}

	// 2. JSON Schema validation based on kind
	schemaDiags := v.validateJSONSchema(doc, parsed)
	diagnostics = append(diagnostics, schemaDiags...)

	// 3. Cross-reference validation (file refs exist)
	refDiags := v.validateCrossReferences(ctx, doc, workspace, projectID)
	diagnostics = append(diagnostics, refDiags...)

	// 4. PromptKit semantic validation
	semanticDiags := v.validateSemantics(doc, parsed)
	diagnostics = append(diagnostics, semanticDiags...)

	return diagnostics
}

// validateYAMLSyntax validates YAML syntax and returns diagnostics for parse errors.
func (v *Validator) validateYAMLSyntax(doc *Document) []Diagnostic {
	var diagnostics []Diagnostic

	var parsed any
	err := yaml.Unmarshal([]byte(doc.Content), &parsed)
	if err != nil {
		// Extract line number from YAML error if possible
		line := 0
		col := 0
		errMsg := err.Error()

		// YAML errors often contain "line X" information
		if pos := extractYAMLPosition(errMsg); pos != nil {
			line = pos.Line
			col = pos.Character
		}

		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Character: col},
				End:   Position{Line: line, Character: col + 10},
			},
			Severity: SeverityError,
			Source:   "yaml",
			Message:  fmt.Sprintf("YAML syntax error: %v", err),
		})
	}

	return diagnostics
}

// extractYAMLPosition tries to extract position from a YAML error string.
func extractYAMLPosition(errStr string) *Position {
	// YAML errors often contain "line X" or "line X column Y"
	re := regexp.MustCompile(`line (\d+)(?:\s+column (\d+))?`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) >= 2 {
		var line, col int
		_, _ = fmt.Sscanf(matches[1], "%d", &line)
		if len(matches) >= 3 && matches[2] != "" {
			_, _ = fmt.Sscanf(matches[2], "%d", &col)
		}
		// Convert to 0-indexed
		return &Position{Line: max(0, line-1), Character: max(0, col-1)}
	}
	return nil
}

// validateJSONSchema validates the document against the appropriate PromptKit JSON schema.
func (v *Validator) validateJSONSchema(doc *Document, parsed map[string]any) []Diagnostic {
	var diagnostics []Diagnostic

	// Get the kind to select the appropriate schema
	kind, ok := parsed["kind"].(string)
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 10},
			},
			Severity: SeverityError,
			Source:   "schema",
			Message:  "missing or invalid 'kind' field",
		})
		return diagnostics
	}

	// Get the schema loader for this kind
	schemaLoader, ok := v.schemaLoaders[kind]
	if !ok {
		// Unknown kind - provide a warning but don't error
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 10},
			},
			Severity: SeverityWarning,
			Source:   "schema",
			Message:  fmt.Sprintf("unknown kind '%s', schema validation skipped", kind),
		})
		return diagnostics
	}

	// Convert parsed YAML to JSON for schema validation
	jsonData, err := json.Marshal(parsed)
	if err != nil {
		v.log.Error(err, "failed to convert YAML to JSON")
		return diagnostics
	}

	documentLoader := gojsonschema.NewBytesLoader(jsonData)
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		v.log.Error(err, "schema validation error")
		return diagnostics
	}

	if !result.Valid() {
		for _, schemaErr := range result.Errors() {
			// Try to find the line for this error
			line, col := v.findFieldPosition(doc, schemaErr.Field())

			diagnostics = append(diagnostics, Diagnostic{
				Range: Range{
					Start: Position{Line: line, Character: col},
					End:   Position{Line: line, Character: col + 20},
				},
				Severity: SeverityError,
				Source:   "schema",
				Message:  fmt.Sprintf("%s: %s", schemaErr.Field(), schemaErr.Description()),
			})
		}
	}

	return diagnostics
}

// findFieldPosition finds the position of a field in the document.
func (v *Validator) findFieldPosition(doc *Document, field string) (int, int) {
	// Field comes as "root.field.subfield" or "(root).field"
	// Extract the last part as the field name
	parts := strings.Split(field, ".")
	fieldName := parts[len(parts)-1]
	if fieldName == "(root)" {
		fieldName = ""
	}

	// Search for the field in the document
	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)
		if fieldName == "" {
			// Root level error, return first line
			return 0, 0
		}
		if strings.HasPrefix(trimmed, fieldName+":") || strings.HasPrefix(trimmed, "- "+fieldName+":") {
			col := strings.Index(line, fieldName)
			return i, col
		}
	}

	return 0, 0
}

// validateCrossReferences validates that file references exist.
func (v *Validator) validateCrossReferences(
	ctx context.Context, doc *Document, workspace, projectID string,
) []Diagnostic {
	var diagnostics []Diagnostic

	// Get list of files in the project
	files, err := v.getProjectFiles(ctx, workspace, projectID)
	if err != nil {
		v.log.V(1).Info("failed to get project files, skipping cross-reference validation", "error", err.Error())
		return diagnostics
	}

	// Build a set of available files
	fileSet := make(map[string]bool)
	for _, file := range files {
		fileSet[file] = true
		// Also add without leading slash
		fileSet[strings.TrimPrefix(file, "/")] = true
	}

	// Check file references in the document
	for i, line := range doc.Lines {
		for _, pattern := range v.refPatterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) >= 2 {
				refPath := strings.TrimSpace(matches[1])
				// Remove quotes if present
				refPath = strings.Trim(refPath, `"'`)

				if refPath != "" && !fileSet[refPath] {
					col := strings.Index(line, refPath)
					diagnostics = append(diagnostics, Diagnostic{
						Range: Range{
							Start: Position{Line: i, Character: col},
							End:   Position{Line: i, Character: col + len(refPath)},
						},
						Severity: SeverityWarning,
						Source:   "cross-reference",
						Message:  fmt.Sprintf("file '%s' not found in project", refPath),
					})
				}
			}
		}
	}

	return diagnostics
}

// getProjectFiles returns the list of files in a project.
func (v *Validator) getProjectFiles(ctx context.Context, workspace, projectID string) ([]string, error) {
	cacheKey := fmt.Sprintf("%s/%s", workspace, projectID)

	// Check cache
	v.fileCacheMu.RLock()
	cached, ok := v.fileCache[cacheKey]
	v.fileCacheMu.RUnlock()

	if ok && time.Now().Before(cached.expiresAt) {
		return cached.files, nil
	}

	// Fetch from dashboard API
	url := fmt.Sprintf("%s/api/workspaces/%s/arena/projects/%s/files", v.dashboardAPIURL, workspace, projectID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch files: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Files []string `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update cache
	v.fileCacheMu.Lock()
	v.fileCache[cacheKey] = &cachedFiles{
		files:     result.Files,
		expiresAt: time.Now().Add(v.cacheTTL),
	}
	v.fileCacheMu.Unlock()

	return result.Files, nil
}

// validateSemantics performs PromptKit-specific semantic validation.
func (v *Validator) validateSemantics(doc *Document, parsed map[string]any) []Diagnostic {
	var diagnostics []Diagnostic

	// Check for required apiVersion field
	apiVersion, hasAPIVersion := parsed["apiVersion"]
	if !hasAPIVersion {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 10},
			},
			Severity: SeverityError,
			Source:   "semantic",
			Message:  "missing required field 'apiVersion'",
		})
	} else if apiVersionStr, ok := apiVersion.(string); ok {
		// Validate apiVersion format
		if !strings.HasPrefix(apiVersionStr, "promptkit.altairalabs.ai/") {
			line, col := v.findFieldPosition(doc, "apiVersion")
			diagnostics = append(diagnostics, Diagnostic{
				Range: Range{
					Start: Position{Line: line, Character: col},
					End:   Position{Line: line, Character: col + 20},
				},
				Severity: SeverityWarning,
				Source:   "semantic",
				Message:  fmt.Sprintf("apiVersion '%s' should start with 'promptkit.altairalabs.ai/'", apiVersionStr),
			})
		}
	}

	// Check for required kind field
	kind, hasKind := parsed["kind"]
	if !hasKind {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 10},
			},
			Severity: SeverityError,
			Source:   "semantic",
			Message:  "missing required field 'kind'",
		})
		return diagnostics
	}

	kindStr, ok := kind.(string)
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 10},
			},
			Severity: SeverityError,
			Source:   "semantic",
			Message:  "'kind' must be a string",
		})
		return diagnostics
	}

	// Validate based on kind
	switch kindStr {
	case "Tool":
		diagnostics = append(diagnostics, v.validateToolSemantics(doc, parsed)...)
	case "Provider":
		diagnostics = append(diagnostics, v.validateProviderSemantics(doc, parsed)...)
	case "PromptConfig":
		diagnostics = append(diagnostics, v.validatePromptConfigSemantics(doc, parsed)...)
	case "Scenario":
		diagnostics = append(diagnostics, v.validateScenarioSemantics(doc, parsed)...)
	case "Arena":
		diagnostics = append(diagnostics, v.validateArenaSemantics(doc, parsed)...)
	case "Persona":
		diagnostics = append(diagnostics, v.validatePersonaSemantics(doc, parsed)...)
	}

	return diagnostics
}

// specFieldReq defines a required or recommended field in a spec.
type specFieldReq struct {
	name     string
	severity DiagnosticSeverity
	article  string // "a" or "an" for grammar
}

// validateSpecFields is a helper that validates required/recommended spec fields.
func (v *Validator) validateSpecFields(kind string, parsed map[string]any, fields []specFieldReq) []Diagnostic {
	var diagnostics []Diagnostic

	spec, hasSpec := parsed["spec"].(map[string]any)
	if !hasSpec {
		return []Diagnostic{{
			Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 10}},
			Severity: SeverityError,
			Source:   "semantic",
			Message:  kind + " must have a 'spec' field",
		}}
	}

	for _, field := range fields {
		if _, ok := spec[field.name]; !ok {
			verb := "must have"
			if field.severity == SeverityWarning {
				verb = "should have"
			}
			diagnostics = append(diagnostics, Diagnostic{
				Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 10}},
				Severity: field.severity,
				Source:   "semantic",
				Message:  kind + " spec " + verb + " " + field.article + " '" + field.name + "' field",
			})
		}
	}

	return diagnostics
}

// validateToolSemantics validates Tool-specific semantics.
func (v *Validator) validateToolSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("Tool", parsed, []specFieldReq{
		{name: "name", severity: SeverityError, article: "a"},
		{name: "description", severity: SeverityError, article: "a"},
	})
}

// validateProviderSemantics validates Provider-specific semantics.
func (v *Validator) validateProviderSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("Provider", parsed, []specFieldReq{
		{name: "id", severity: SeverityError, article: "an"},
		{name: "type", severity: SeverityError, article: "a"},
		{name: "model", severity: SeverityError, article: "a"},
	})
}

// validatePromptConfigSemantics validates PromptConfig-specific semantics.
func (v *Validator) validatePromptConfigSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("PromptConfig", parsed, []specFieldReq{
		{name: "task_type", severity: SeverityError, article: "a"},
		{name: "system_template", severity: SeverityError, article: "a"},
	})
}

// validateScenarioSemantics validates Scenario-specific semantics.
func (v *Validator) validateScenarioSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("Scenario", parsed, []specFieldReq{
		{name: "id", severity: SeverityError, article: "an"},
		{name: "turns", severity: SeverityWarning, article: ""},
	})
}

// validateArenaSemantics validates Arena-specific semantics.
func (v *Validator) validateArenaSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("Arena", parsed, []specFieldReq{
		{name: "providers", severity: SeverityError, article: ""},
		{name: "defaults", severity: SeverityError, article: ""},
	})
}

// validatePersonaSemantics validates Persona-specific semantics.
func (v *Validator) validatePersonaSemantics(_ *Document, parsed map[string]any) []Diagnostic {
	return v.validateSpecFields("Persona", parsed, []specFieldReq{
		{name: "name", severity: SeverityError, article: "a"},
		{name: "description", severity: SeverityError, article: "a"},
	})
}
