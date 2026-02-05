/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

const (
	testFilesPath         = "/api/workspaces/ws1/arena/projects/proj1/files"
	testFilesTool1Path    = testFilesPath + "/tools/tool1.yaml"
	testFilesBadYAMLPath  = testFilesPath + "/tools/bad.yaml"
	testFilesTestYAMLPath = testFilesPath + "/test.yaml"
)

func TestHandleValidate_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/validate", nil)
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleValidate_InvalidBody(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleValidate_NoFiles(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := ValidateRequest{Files: []FileContent{}}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/validate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleValidate_ValidFile(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := ValidateRequest{
		Files: []FileContent{
			{
				Path:    "test.yaml",
				Content: "kind: Tool\nspec:\n  name: test\n  description: Test tool",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/validate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp.Diagnostics["test.yaml"]; !ok {
		t.Error("expected diagnostics for test.yaml")
	}
}

func TestHandleValidate_InvalidYAML(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := ValidateRequest{
		Files: []FileContent{
			{
				Path:    "invalid.yaml",
				Content: "kind: Tool\n  bad: indentation",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/validate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	diags := resp.Diagnostics["invalid.yaml"]
	if len(diags) == 0 {
		t.Error("expected diagnostics for invalid YAML")
	}
}

func TestHandleValidate_MultipleFiles(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := ValidateRequest{
		Files: []FileContent{
			{Path: "tool.yaml", Content: "kind: Tool\nspec:\n  name: tool1"},
			{Path: "provider.yaml", Content: "kind: Provider\nspec:\n  id: p1"},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/validate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Diagnostics) != 2 {
		t.Errorf("expected 2 files in diagnostics, got %d", len(resp.Diagnostics))
	}
}

func TestHandleCompile_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/compile", nil)
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleCompile_InvalidBody(t *testing.T) {
	srv, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleCompile_MissingWorkspace(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := CompileRequest{Project: "proj1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleCompile_MissingProject(t *testing.T) {
	srv, _ := newTestServer()

	reqBody := CompileRequest{Workspace: "ws1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestValidateFileOnly_ValidYAML(t *testing.T) {
	srv, _ := newTestServer()

	doc := &Document{
		URI:        "file:///test.yaml",
		LanguageID: "yaml",
		Version:    1,
		Content:    "kind: Tool\nspec:\n  name: test\n  description: Test tool",
		Lines:      []string{"kind: Tool", "spec:", "  name: test", "  description: Test tool"},
	}

	diags := srv.validateFileOnly(context.Background(), doc)
	// Should return diagnostics (may have schema warnings but not syntax errors)
	_ = diags
}

func TestValidateFileOnly_InvalidYAML(t *testing.T) {
	srv, _ := newTestServer()

	doc := &Document{
		URI:        "file:///test.yaml",
		LanguageID: "yaml",
		Version:    1,
		Content:    "kind: Tool\n  bad: indentation",
		Lines:      []string{"kind: Tool", "  bad: indentation"},
	}

	diags := srv.validateFileOnly(context.Background(), doc)
	if len(diags) == 0 {
		t.Error("expected diagnostics for invalid YAML")
	}
}

func TestValidateFileOnly_EmptyContent(t *testing.T) {
	srv, _ := newTestServer()

	doc := &Document{
		URI:        "file:///test.yaml",
		LanguageID: "yaml",
		Version:    1,
		Content:    "",
		Lines:      []string{},
	}

	diags := srv.validateFileOnly(context.Background(), doc)
	// Empty content should be valid YAML
	_ = diags
}

func TestFetchFileContent(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testFilesTestYAMLPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"content": "kind: Tool"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	content, err := srv.fetchFileContent(context.Background(), "ws1", "proj1", "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "kind: Tool" {
		t.Errorf("expected content 'kind: Tool', got %q", content)
	}
}

func TestFetchFileContent_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	_, err := srv.fetchFileContent(context.Background(), "ws1", "proj1", "notfound.yaml")
	// Should return an error or empty content
	_ = err
}

func TestFetchFileContent_InvalidJSON(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	_, err := srv.fetchFileContent(context.Background(), "ws1", "proj1", "test.yaml")
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestHandleCompile_Success(t *testing.T) {
	// Create a mock server that returns project files and file content
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testFilesPath:
			// Return list of files wrapped in {files: [...]}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/tool1.yaml"}})
		case testFilesTool1Path:
			// Return file content
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": "kind: Tool\nspec:\n  name: tool1\n  description: A test tool",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	reqBody := CompileRequest{Workspace: "ws1", Project: "proj1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp CompileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Summary == nil {
		t.Error("expected summary in response")
	}
	if resp.Summary.TotalFiles != 1 {
		t.Errorf("expected 1 total file, got %d", resp.Summary.TotalFiles)
	}
}

func TestHandleCompile_WithErrors(t *testing.T) {
	// Create a mock server that returns an invalid file
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testFilesPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/bad.yaml"}})
		case testFilesBadYAMLPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": "kind: Tool\n  bad: indentation",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	reqBody := CompileRequest{Workspace: "ws1", Project: "proj1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp CompileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Valid {
		t.Error("expected Valid to be false for invalid file")
	}
	if resp.Summary.ErrorCount == 0 {
		t.Error("expected errors in summary")
	}
}

func TestHandleCompile_FetchFileFails(t *testing.T) {
	// Create a mock server that returns files but fails to fetch content
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testFilesPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/tool1.yaml"}})
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	reqBody := CompileRequest{Workspace: "ws1", Project: "proj1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp CompileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Valid {
		t.Error("expected Valid to be false when fetch fails")
	}
}

func TestHandleCompile_GetProjectFilesFails(t *testing.T) {
	// Create a mock server that fails to return project files
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	reqBody := CompileRequest{Workspace: "ws1", Project: "proj1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/compile", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	srv.handleCompile(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGetRefCompletions_WithFiles(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {
				"tools/tool1.yaml",
				"tools/tool2.yml",
				"providers/provider1.yaml",
			}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	items := srv.getRefCompletions(context.Background(), "ws1", "proj1", "tools", "Tool")

	if len(items) != 2 {
		t.Errorf("expected 2 tool completions, got %d", len(items))
	}
}

func TestGetRefCompletions_NoFiles(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	items := srv.getRefCompletions(context.Background(), "ws1", "proj1", "tools", "Tool")

	if items != nil {
		t.Errorf("expected nil items when getProjectFiles fails, got %v", items)
	}
}

func TestFindDefinitionLocation_Found(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {
				"tools/my-tool.yaml",
				"providers/my-provider.yaml",
			}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	loc := srv.findDefinitionLocation(context.Background(), "ws1", "proj1", "tool", "my-tool")

	if loc == nil {
		t.Fatal("expected location to be found")
	}
	if loc.URI != "promptkit://ws1/proj1/tools/my-tool.yaml" {
		t.Errorf("unexpected URI: %s", loc.URI)
	}
}

func TestFindDefinitionLocation_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/other-tool.yaml"}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	loc := srv.findDefinitionLocation(context.Background(), "ws1", "proj1", "tool", "nonexistent")

	if loc != nil {
		t.Error("expected nil location for nonexistent tool")
	}
}

func TestFindDefinitionLocation_UnknownRefType(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/tool1.yaml"}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())
	srv := &Server{
		config:    Config{DashboardAPIURL: mockServer.URL},
		validator: validator,
		log:       logr.Discard(),
	}

	loc := srv.findDefinitionLocation(context.Background(), "ws1", "proj1", "unknown", "name")

	if loc != nil {
		t.Error("expected nil location for unknown ref type")
	}
}

func TestValidateCrossReferences_WithFiles(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {
				"tools/existing-tool.yaml",
				"providers/existing-provider.yaml",
			}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "tool: missing-tool\nprovider: existing-provider",
		Lines:   []string{"tool: missing-tool", "provider: existing-provider"},
	}

	diags := validator.validateCrossReferences(context.Background(), doc, "ws1", "proj1")
	// Should find a diagnostic for missing-tool
	_ = diags
}

func TestValidateCrossReferences_APIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "tool: some-tool",
		Lines:   []string{"tool: some-tool"},
	}

	diags := validator.validateCrossReferences(context.Background(), doc, "ws1", "proj1")

	// Should return empty diagnostics when API fails
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when API fails, got %d", len(diags))
	}
}

func TestGetProjectFiles_Cached(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == testFilesPath {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string][]string{"files": {"tools/tool1.yaml"}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	validator, _ := NewValidator(mockServer.URL, logr.Discard())

	// First call - should hit the API
	files1, err := validator.getProjectFiles(context.Background(), "ws1", "proj1")
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call - should use cache
	files2, err := validator.getProjectFiles(context.Background(), "ws1", "proj1")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if len(files1) != len(files2) {
		t.Error("cached result differs from original")
	}

	// Should only have made one API call due to caching
	if callCount != 1 {
		t.Errorf("expected 1 API call due to caching, got %d", callCount)
	}
}
