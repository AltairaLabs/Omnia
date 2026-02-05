/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package api provides HTTP API endpoints for the arena controller.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Server provides HTTP API endpoints for arena operations.
type Server struct {
	addr   string
	log    logr.Logger
	server *http.Server
}

// NewServer creates a new API server.
func NewServer(addr string, log logr.Logger) *Server {
	return &Server{
		addr: addr,
		log:  log.WithName("api-server"),
	}
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/render-template", s.handleRenderTemplate)
	mux.HandleFunc("/api/preview-template", s.handlePreviewTemplate)
	mux.HandleFunc("/healthz", s.handleHealthz)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.log.Info("starting API server", "addr", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// RenderTemplateRequest is the request body for /api/render-template.
type RenderTemplateRequest struct {
	// TemplatePath is the path to the template directory (containing template.yaml).
	TemplatePath string `json:"templatePath"`
	// OutputPath is the path where rendered files will be written.
	OutputPath string `json:"outputPath"`
	// ProjectName is the name for the generated project.
	ProjectName string `json:"projectName"`
	// Variables are the variable values to substitute.
	Variables map[string]interface{} `json:"variables"`
}

// RenderTemplateResponse is the response for /api/render-template.
type RenderTemplateResponse struct {
	// Success indicates whether the rendering succeeded.
	Success bool `json:"success"`
	// FilesCreated lists the paths of created files.
	FilesCreated []string `json:"filesCreated,omitempty"`
	// Errors lists any errors encountered.
	Errors []string `json:"errors,omitempty"`
	// Warnings lists any warnings.
	Warnings []string `json:"warnings,omitempty"`
}

// handleRenderTemplate handles POST /api/render-template.
// Uses PromptKit's Generator to render templates.
func (s *Server) handleRenderTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RenderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.TemplatePath == "" {
		http.Error(w, "templatePath is required", http.StatusBadRequest)
		return
	}
	if req.OutputPath == "" {
		http.Error(w, "outputPath is required", http.StatusBadRequest)
		return
	}
	if req.ProjectName == "" {
		http.Error(w, "projectName is required", http.StatusBadRequest)
		return
	}

	// Use PromptKit's Generator to render the template
	result, err := RenderTemplate(req.TemplatePath, req.OutputPath, req.ProjectName, req.Variables)
	if err != nil {
		s.log.Error(err, "failed to render template",
			"templatePath", req.TemplatePath,
			"outputPath", req.OutputPath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(RenderTemplateResponse{
			Success: false,
			Errors:  []string{err.Error()},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		s.log.Error(err, "failed to encode response")
	}
}

// PreviewTemplateRequest is the request body for /api/preview-template.
type PreviewTemplateRequest struct {
	// TemplatePath is the path to the template directory (containing template.yaml).
	TemplatePath string `json:"templatePath"`
	// ProjectName is the name for the generated project (used in template rendering).
	ProjectName string `json:"projectName"`
	// Variables are the variable values to substitute.
	Variables map[string]interface{} `json:"variables"`
}

// PreviewTemplateResponse is the response for /api/preview-template.
type PreviewTemplateResponse struct {
	// Files contains the rendered file contents keyed by path.
	Files []PreviewFile `json:"files"`
	// Errors lists any errors encountered.
	Errors []string `json:"errors,omitempty"`
}

// PreviewFile represents a single rendered file.
type PreviewFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// handlePreviewTemplate handles POST /api/preview-template.
// Uses PromptKit's Generator to render templates to a temp directory,
// then returns the file contents without persisting them.
func (s *Server) handlePreviewTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PreviewTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.TemplatePath == "" {
		http.Error(w, "templatePath is required", http.StatusBadRequest)
		return
	}
	if req.ProjectName == "" {
		http.Error(w, "projectName is required", http.StatusBadRequest)
		return
	}

	// Use PromptKit's Generator to preview the template
	result, err := PreviewTemplate(req.TemplatePath, req.ProjectName, req.Variables)
	if err != nil {
		s.log.Error(err, "failed to preview template", "templatePath", req.TemplatePath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(PreviewTemplateResponse{
			Errors: []string{err.Error()},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		s.log.Error(err, "failed to encode response")
	}
}

// handleHealthz handles health checks.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
