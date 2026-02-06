/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestNewServer(t *testing.T) {
	s := NewServer(":8080", logr.Discard())
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
	if s.addr != ":8080" {
		t.Errorf("addr = %q, want %q", s.addr, ":8080")
	}
}

func TestHandleHealthz(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	s.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestHandleRenderTemplate_MethodNotAllowed(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/render-template", nil)
			w := httptest.NewRecorder()

			s.handleRenderTemplate(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandleRenderTemplate_InvalidJSON(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/render-template", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	s.handleRenderTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRenderTemplate_MissingTemplatePath(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := RenderTemplateRequest{
		OutputPath:  "/output",
		ProjectName: "test",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/render-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handleRenderTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() != "templatePath is required\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "templatePath is required\n")
	}
}

func TestHandleRenderTemplate_MissingOutputPath(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := RenderTemplateRequest{
		TemplatePath: "/template",
		ProjectName:  "test",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/render-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handleRenderTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() != "outputPath is required\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "outputPath is required\n")
	}
}

func TestHandleRenderTemplate_MissingProjectName(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := RenderTemplateRequest{
		TemplatePath: "/template",
		OutputPath:   "/output",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/render-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handleRenderTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() != "projectName is required\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "projectName is required\n")
	}
}

func TestHandlePreviewTemplate_MethodNotAllowed(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/preview-template", nil)
			w := httptest.NewRecorder()

			s.handlePreviewTemplate(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandlePreviewTemplate_InvalidJSON(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	req := httptest.NewRequest(http.MethodPost, "/api/preview-template", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	s.handlePreviewTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlePreviewTemplate_MissingTemplatePath(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := PreviewTemplateRequest{
		ProjectName: "test",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/preview-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handlePreviewTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() != "templatePath is required\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "templatePath is required\n")
	}
}

func TestHandlePreviewTemplate_MissingProjectName(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := PreviewTemplateRequest{
		TemplatePath: "/template",
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/preview-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handlePreviewTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() != "projectName is required\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "projectName is required\n")
	}
}

func TestServerShutdown_NilServer(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	// Server is nil before Start is called
	err := s.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown() error = %v, want nil", err)
	}
}

func TestServerStart_InvalidAddress(t *testing.T) {
	// Test that Start returns an error when the address is invalid
	// This covers the Start() function without needing goroutines
	s := NewServer("invalid:::address", logr.Discard())

	err := s.Start(context.Background())
	if err == nil {
		t.Error("Start() should fail with invalid address")
	}
}

func TestHandleRenderTemplate_InvalidTemplatePath(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := RenderTemplateRequest{
		TemplatePath: "/nonexistent/path",
		OutputPath:   t.TempDir(),
		ProjectName:  "test",
		Variables:    map[string]any{},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/render-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handleRenderTemplate(w, req)

	// Should return 500 with error in response body
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp RenderTemplateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Success {
		t.Error("expected Success = false")
	}
	if len(resp.Errors) == 0 {
		t.Error("expected errors in response")
	}
}

func TestHandlePreviewTemplate_InvalidTemplatePath(t *testing.T) {
	s := NewServer(":8080", logr.Discard())

	body := PreviewTemplateRequest{
		TemplatePath: "/nonexistent/path",
		ProjectName:  "test",
		Variables:    map[string]any{},
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/preview-template", bytes.NewBuffer(bodyJSON))
	w := httptest.NewRecorder()

	s.handlePreviewTemplate(w, req)

	// Should return 500 with error in response body
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp PreviewTemplateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Errors) == 0 {
		t.Error("expected errors in response")
	}
}
