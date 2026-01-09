/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package media

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-logr/logr"
)

func setupTestHandler(t *testing.T) (*Handler, *LocalStorage, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "handler-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("NewLocalStorage() error = %v", err)
	}

	handler := NewHandler(storage, logr.Discard())
	return handler, storage, tmpDir
}

func TestHandler_RequestUpload(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	tests := []struct {
		name       string
		method     string
		body       interface{}
		wantStatus int
	}{
		{
			name:   "valid request",
			method: http.MethodPost,
			body: map[string]interface{}{
				"session_id": "session-123",
				"filename":   "test.jpg",
				"mime_type":  "image/jpeg",
				"size_bytes": 1024,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong method",
			method:     http.MethodGet,
			body:       nil,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "invalid body",
			method:     http.MethodPost,
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "missing session_id",
			method: http.MethodPost,
			body: map[string]interface{}{
				"filename":   "test.jpg",
				"mime_type":  "image/jpeg",
				"size_bytes": 1024,
			},
			wantStatus: http.StatusBadRequest, // ErrInvalidStorageRef
		},
		{
			name:   "missing mime_type",
			method: http.MethodPost,
			body: map[string]interface{}{
				"session_id": "session-123",
				"filename":   "test.jpg",
				"size_bytes": 1024,
			},
			wantStatus: http.StatusUnsupportedMediaType, // ErrUnsupportedMIMEType
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != nil {
				switch v := tt.body.(type) {
				case string:
					bodyReader = bytes.NewBufferString(v)
				default:
					bodyBytes, _ := json.Marshal(v)
					bodyReader = bytes.NewBuffer(bodyBytes)
				}
			}

			req := httptest.NewRequest(tt.method, "/media/request-upload", bodyReader)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.handleRequestUpload(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("handleRequestUpload() status = %v, want %v", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var creds UploadCredentials
				if err := json.NewDecoder(rec.Body).Decode(&creds); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if creds.UploadID == "" {
					t.Error("UploadID should not be empty")
				}
			}
		})
	}
}

func TestHandler_Upload(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	// First, request an upload URL
	creds, err := storage.GetUploadURL(context.TODO(), UploadRequest{
		SessionID: "session-123",
		Filename:  "test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 5,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid upload",
			method:     http.MethodPut,
			path:       "/media/upload/" + creds.UploadID,
			body:       "hello",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			path:       "/media/upload/some-id",
			body:       "hello",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "missing upload ID",
			method:     http.MethodPut,
			path:       "/media/upload/",
			body:       "hello",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid upload ID",
			method:     http.MethodPut,
			path:       "/media/upload/nonexistent",
			body:       "hello",
			wantStatus: http.StatusBadRequest, // ErrUploadFailed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			handler.handleUpload(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("handleUpload() status = %v, want %v, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHandler_Download(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	// Create a file to download
	creds, err := storage.GetUploadURL(context.TODO(), UploadRequest{
		SessionID: "session-123",
		Filename:  "download-test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 5,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	uploadPath, _ := storage.GetUploadPath(creds.UploadID)
	if err := os.WriteFile(uploadPath, []byte("hello"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if err := storage.CompleteUpload(context.TODO(), creds.UploadID, 5); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	// Parse the storage ref to get session/media IDs
	ref, _ := ParseStorageRef(creds.StorageRef)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "valid download",
			method:     http.MethodGet,
			path:       "/media/download/" + ref.SessionID + "/" + ref.MediaID,
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			path:       "/media/download/session/media",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "invalid path - missing media ID",
			method:     http.MethodGet,
			path:       "/media/download/session-only",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			method:     http.MethodGet,
			path:       "/media/download/nonexistent/nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.handleDownload(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("handleDownload() status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandler_Info(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	// Create a file
	creds, err := storage.GetUploadURL(context.TODO(), UploadRequest{
		SessionID: "session-123",
		Filename:  "info-test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 5,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	uploadPath, _ := storage.GetUploadPath(creds.UploadID)
	if err := os.WriteFile(uploadPath, []byte("hello"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if err := storage.CompleteUpload(context.TODO(), creds.UploadID, 5); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	ref, _ := ParseStorageRef(creds.StorageRef)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "valid info",
			method:     http.MethodGet,
			path:       "/media/info/" + ref.SessionID + "/" + ref.MediaID,
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			path:       "/media/info/session/media",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "invalid path",
			method:     http.MethodGet,
			path:       "/media/info/session-only",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			method:     http.MethodGet,
			path:       "/media/info/nonexistent/nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.handleInfo(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("handleInfo() status = %v, want %v", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var info MediaInfo
				if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if info.Filename != "info-test.txt" {
					t.Errorf("Filename = %v, want info-test.txt", info.Filename)
				}
			}
		})
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that routes are registered by making requests
	// Note: These will return errors (400, 404, etc.) because the resources don't exist,
	// but they should NOT return 404 "page not found" which indicates unregistered route
	tests := []struct {
		path         string
		method       string
		expectStatus int // Expected status that proves route is registered
	}{
		{"/media/request-upload", http.MethodPost, http.StatusBadRequest},      // Invalid JSON body
		{"/media/upload/test", http.MethodPut, http.StatusBadRequest},          // Upload not found
		{"/media/download/session/media", http.MethodGet, http.StatusNotFound}, // Media not found
		{"/media/info/session/media", http.MethodGet, http.StatusNotFound},     // Media not found
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			// Verify we get the expected status (proving route is registered)
			if rec.Code != tt.expectStatus {
				t.Errorf("Route %s returned %d, want %d", tt.path, rec.Code, tt.expectStatus)
			}
		})
	}
}

func TestHandler_WriteError(t *testing.T) {
	handler, storage, tmpDir := setupTestHandler(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = storage.Close() }()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"media not found", ErrMediaNotFound, http.StatusNotFound},
		{"media expired", ErrMediaExpired, http.StatusGone},
		{"invalid storage ref", ErrInvalidStorageRef, http.StatusBadRequest},
		{"upload failed", ErrUploadFailed, http.StatusBadRequest},
		{"file too large", ErrFileTooLarge, http.StatusRequestEntityTooLarge},
		{"unsupported mime type", ErrUnsupportedMIMEType, http.StatusUnsupportedMediaType},
		{"unknown error", io.EOF, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.writeError(rec, tt.err)

			if rec.Code != tt.wantStatus {
				t.Errorf("writeError() status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}
