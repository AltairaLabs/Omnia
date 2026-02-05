/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

// ValidateRequest is the request body for /api/validate.
type ValidateRequest struct {
	Files []FileContent `json:"files"`
}

// FileContent represents a file and its content for validation.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ValidateResponse is the response for /api/validate.
type ValidateResponse struct {
	Diagnostics map[string][]Diagnostic `json:"diagnostics"`
}

// CompileRequest is the request body for /api/compile.
type CompileRequest struct {
	Workspace string `json:"workspace"`
	Project   string `json:"project"`
}

// CompileResponse is the response for /api/compile.
type CompileResponse struct {
	Valid       bool                    `json:"valid"`
	Diagnostics map[string][]Diagnostic `json:"diagnostics"`
	Warnings    []string                `json:"warnings,omitempty"`
	Summary     *CompileSummary         `json:"summary,omitempty"`
}

// CompileSummary provides summary information about the compile.
type CompileSummary struct {
	TotalFiles   int `json:"totalFiles"`
	ValidFiles   int `json:"validFiles"`
	InvalidFiles int `json:"invalidFiles"`
	ErrorCount   int `json:"errorCount"`
	WarningCount int `json:"warningCount"`
}

// handleValidate handles POST /api/validate.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Files) == 0 {
		http.Error(w, "No files provided", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	response := ValidateResponse{
		Diagnostics: make(map[string][]Diagnostic),
	}

	// Validate each file
	for _, file := range req.Files {
		doc := &Document{
			URI:        file.Path,
			LanguageID: "yaml",
			Version:    1,
			Content:    file.Content,
			Lines:      splitLines(file.Content),
		}

		// Skip cross-reference validation since we don't have workspace context
		diags := s.validateFileOnly(ctx, doc)
		response.Diagnostics[file.Path] = diags
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error(err, "failed to encode validate response")
	}
}

// countDiagnostics counts errors and warnings in a slice of diagnostics.
func countDiagnostics(diags []Diagnostic) (errors, warnings int) {
	for _, diag := range diags {
		switch diag.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		}
	}
	return errors, warnings
}

// collectWarnings extracts warning messages from diagnostics.
func collectWarnings(filePath string, diags []Diagnostic) []string {
	var warnings []string
	for _, diag := range diags {
		if diag.Severity == SeverityWarning {
			warnings = append(warnings, filePath+": "+diag.Message)
		}
	}
	return warnings
}

// handleCompile handles POST /api/compile.
func (s *Server) handleCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Workspace == "" || req.Project == "" {
		http.Error(w, "workspace and project are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	files, err := s.validator.getProjectFiles(ctx, req.Workspace, req.Project)
	if err != nil {
		s.log.Error(err, "failed to get project files")
		http.Error(w, "Failed to get project files", http.StatusInternalServerError)
		return
	}

	response := CompileResponse{
		Valid:       true,
		Diagnostics: make(map[string][]Diagnostic),
		Warnings:    []string{},
		Summary:     &CompileSummary{TotalFiles: len(files)},
	}

	for _, filePath := range files {
		content, err := s.fetchFileContent(ctx, req.Workspace, req.Project, filePath)
		if err != nil {
			s.log.Error(err, "failed to fetch file", "path", filePath)
			response.Diagnostics[filePath] = []Diagnostic{{
				Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				Severity: SeverityError,
				Source:   "compile",
				Message:  "Failed to fetch file: " + err.Error(),
			}}
			response.Valid = false
			response.Summary.InvalidFiles++
			response.Summary.ErrorCount++
			continue
		}

		doc := &Document{URI: filePath, LanguageID: "yaml", Version: 1, Content: content, Lines: splitLines(content)}
		diags := s.validator.ValidateDocument(ctx, doc, req.Workspace, req.Project)
		response.Diagnostics[filePath] = diags

		errors, warnings := countDiagnostics(diags)
		response.Summary.ErrorCount += errors
		response.Summary.WarningCount += warnings
		response.Warnings = append(response.Warnings, collectWarnings(filePath, diags)...)

		if errors > 0 {
			response.Valid = false
			response.Summary.InvalidFiles++
		} else {
			response.Summary.ValidFiles++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error(err, "failed to encode compile response")
	}
}

// validateFileOnly validates a file without cross-reference checks.
func (s *Server) validateFileOnly(_ context.Context, doc *Document) []Diagnostic {
	var diagnostics []Diagnostic

	// 1. YAML syntax validation
	syntaxDiags := s.validator.validateYAMLSyntax(doc)
	diagnostics = append(diagnostics, syntaxDiags...)
	if len(syntaxDiags) > 0 {
		return diagnostics
	}

	// Parse YAML for schema validation
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(doc.Content), &parsed); err != nil {
		// Shouldn't happen since we already validated syntax
		return diagnostics
	}

	// 2. JSON Schema validation
	schemaDiags := s.validator.validateJSONSchema(doc, parsed)
	diagnostics = append(diagnostics, schemaDiags...)

	// 3. Semantic validation
	semanticDiags := s.validator.validateSemantics(doc, parsed)
	diagnostics = append(diagnostics, semanticDiags...)

	return diagnostics
}

// fetchFileContent fetches a file's content from the dashboard API.
func (s *Server) fetchFileContent(ctx context.Context, workspace, project, path string) (string, error) {
	url := s.config.DashboardAPIURL + "/api/workspaces/" + workspace + "/arena/projects/" + project + "/files/" + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.validator.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", err
	}

	var result struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Content, nil
}
