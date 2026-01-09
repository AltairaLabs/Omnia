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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLocalStorage(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	// Verify the directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("Base directory was not created")
	}
}

func TestLocalStorage_GetUploadURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	tests := []struct {
		name    string
		req     UploadRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: UploadRequest{
				SessionID: "session-123",
				Filename:  "test.jpg",
				MIMEType:  "image/jpeg",
				SizeBytes: 1024,
			},
			wantErr: false,
		},
		{
			name: "missing session ID",
			req: UploadRequest{
				Filename:  "test.jpg",
				MIMEType:  "image/jpeg",
				SizeBytes: 1024,
			},
			wantErr: true,
		},
		{
			name: "missing MIME type",
			req: UploadRequest{
				SessionID: "session-123",
				Filename:  "test.jpg",
				SizeBytes: 1024,
			},
			wantErr: true,
		},
		{
			name: "file too large",
			req: UploadRequest{
				SessionID: "session-123",
				Filename:  "test.jpg",
				MIMEType:  "image/jpeg",
				SizeBytes: 200 * 1024 * 1024, // 200MB > 100MB default
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := storage.GetUploadURL(ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUploadURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if creds.UploadID == "" {
					t.Error("UploadID should not be empty")
				}
				if creds.URL == "" {
					t.Error("URL should not be empty")
				}
				if creds.StorageRef == "" {
					t.Error("StorageRef should not be empty")
				}
				if creds.Method != "PUT" {
					t.Errorf("Method = %v, want PUT", creds.Method)
				}
				if creds.ExpiresAt.Before(time.Now()) {
					t.Error("ExpiresAt should be in the future")
				}
			}
		})
	}
}

func TestLocalStorage_UploadAndDownload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Request upload URL
	creds, err := storage.GetUploadURL(ctx, UploadRequest{
		SessionID: "session-123",
		Filename:  "test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 13,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	// Simulate file upload by writing directly to the path
	uploadPath, err := storage.GetUploadPath(creds.UploadID)
	if err != nil {
		t.Fatalf("GetUploadPath() error = %v", err)
	}

	testContent := []byte("Hello, World!")
	if err := os.WriteFile(uploadPath, testContent, 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Complete the upload
	if err := storage.CompleteUpload(ctx, creds.UploadID, int64(len(testContent))); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	// Get media info
	info, err := storage.GetMediaInfo(ctx, creds.StorageRef)
	if err != nil {
		t.Fatalf("GetMediaInfo() error = %v", err)
	}

	if info.Filename != "test.txt" {
		t.Errorf("Filename = %v, want test.txt", info.Filename)
	}
	if info.MIMEType != "text/plain" {
		t.Errorf("MIMEType = %v, want text/plain", info.MIMEType)
	}
	if info.SizeBytes != int64(len(testContent)) {
		t.Errorf("SizeBytes = %v, want %v", info.SizeBytes, len(testContent))
	}

	// Get download URL
	downloadURL, err := storage.GetDownloadURL(ctx, creds.StorageRef)
	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v", err)
	}
	if downloadURL == "" {
		t.Error("Download URL should not be empty")
	}

	// Get media path
	mediaPath, err := storage.GetMediaPath(creds.StorageRef)
	if err != nil {
		t.Fatalf("GetMediaPath() error = %v", err)
	}

	// Verify file content
	content, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatalf("Failed to read media file: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("Content = %v, want %v", string(content), string(testContent))
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create a file
	creds, err := storage.GetUploadURL(ctx, UploadRequest{
		SessionID: "session-123",
		Filename:  "delete-test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 5,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	uploadPath, err := storage.GetUploadPath(creds.UploadID)
	if err != nil {
		t.Fatalf("GetUploadPath() error = %v", err)
	}

	if err := os.WriteFile(uploadPath, []byte("hello"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := storage.CompleteUpload(ctx, creds.UploadID, 5); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	// Delete the file
	if err := storage.Delete(ctx, creds.StorageRef); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, err = storage.GetMediaInfo(ctx, creds.StorageRef)
	if err != ErrMediaNotFound {
		t.Errorf("Expected ErrMediaNotFound, got %v", err)
	}
}

func TestLocalStorage_DeleteSessionMedia(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()
	sessionID := "session-to-delete"

	// Create multiple files for the session
	for i := 0; i < 3; i++ {
		creds, err := storage.GetUploadURL(ctx, UploadRequest{
			SessionID: sessionID,
			Filename:  "test.txt",
			MIMEType:  "text/plain",
			SizeBytes: 5,
		})
		if err != nil {
			t.Fatalf("GetUploadURL() error = %v", err)
		}

		uploadPath, err := storage.GetUploadPath(creds.UploadID)
		if err != nil {
			t.Fatalf("GetUploadPath() error = %v", err)
		}

		if err := os.WriteFile(uploadPath, []byte("hello"), 0600); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		if err := storage.CompleteUpload(ctx, creds.UploadID, 5); err != nil {
			t.Fatalf("CompleteUpload() error = %v", err)
		}
	}

	// Verify session directory exists
	sessionDir := filepath.Join(tmpDir, sessionID)
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Fatal("Session directory should exist")
	}

	// Delete all session media
	if err := storage.DeleteSessionMedia(ctx, sessionID); err != nil {
		t.Fatalf("DeleteSessionMedia() error = %v", err)
	}

	// Verify session directory is gone
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Session directory should be deleted")
	}
}

func TestLocalStorage_GetMediaInfo_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	_, err = storage.GetMediaInfo(ctx, "omnia://sessions/nonexistent/media/nonexistent")
	if err != ErrMediaNotFound {
		t.Errorf("Expected ErrMediaNotFound, got %v", err)
	}
}

func TestLocalStorage_GetDownloadURL_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	_, err = storage.GetDownloadURL(ctx, "omnia://sessions/nonexistent/media/nonexistent")
	if err != ErrMediaNotFound {
		t.Errorf("Expected ErrMediaNotFound, got %v", err)
	}
}

func TestLocalStorage_GetDownloadURL_InvalidRef(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	_, err = storage.GetDownloadURL(ctx, "invalid-ref")
	if err == nil {
		t.Error("Expected error for invalid ref")
	}
}

func TestLocalStorage_Delete_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	err = storage.Delete(ctx, "omnia://sessions/nonexistent/media/nonexistent")
	if err != ErrMediaNotFound {
		t.Errorf("Expected ErrMediaNotFound, got %v", err)
	}
}

func TestLocalStorage_Delete_InvalidRef(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	err = storage.Delete(ctx, "invalid-ref")
	if err == nil {
		t.Error("Expected error for invalid ref")
	}
}

func TestLocalStorage_GetMediaPath_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	_, err = storage.GetMediaPath("omnia://sessions/nonexistent/media/nonexistent")
	if err != ErrMediaNotFound {
		t.Errorf("Expected ErrMediaNotFound, got %v", err)
	}
}

func TestLocalStorage_GetMediaPath_InvalidRef(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	_, err = storage.GetMediaPath("invalid-ref")
	if err == nil {
		t.Error("Expected error for invalid ref")
	}
}

func TestLocalStorage_GetMediaInfo_InvalidRef(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	_, err = storage.GetMediaInfo(ctx, "invalid-ref")
	if err == nil {
		t.Error("Expected error for invalid ref")
	}
}

func TestLocalStorage_CompleteUpload_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	err = storage.CompleteUpload(ctx, "nonexistent-upload-id", 100)
	if err == nil {
		t.Error("Expected error for nonexistent upload ID")
	}
}

func TestLocalStorage_GetUploadPath_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	_, err = storage.GetUploadPath("nonexistent-upload-id")
	if err == nil {
		t.Error("Expected error for nonexistent upload ID")
	}
}

func TestLocalStorage_CleanupExpired(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create a file with default TTL (won't be expired)
	creds, err := storage.GetUploadURL(ctx, UploadRequest{
		SessionID: "session-cleanup",
		Filename:  "test.txt",
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
	if err := storage.CompleteUpload(ctx, creds.UploadID, 5); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	// Run cleanup - should not remove the file since it's not expired
	cleaned, err := storage.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}

	// File should still exist
	_, err = storage.GetMediaInfo(ctx, creds.StorageRef)
	if err != nil {
		t.Errorf("File should still exist after cleanup, got error: %v", err)
	}

	t.Logf("Cleaned %d files", cleaned)
}

func TestLocalStorage_CleanupExpired_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Run cleanup on empty storage
	cleaned, err := storage.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if cleaned != 0 {
		t.Errorf("CleanupExpired() cleaned %d files, want 0", cleaned)
	}
}

func TestLocalStorage_CleanupExpired_WithExpiredFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create a normal file first
	creds, err := storage.GetUploadURL(ctx, UploadRequest{
		SessionID: "session-expired",
		Filename:  "test.txt",
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
	if err := storage.CompleteUpload(ctx, creds.UploadID, 5); err != nil {
		t.Fatalf("CompleteUpload() error = %v", err)
	}

	// Now manually update the metadata to set an expired time
	ref, _ := ParseStorageRef(creds.StorageRef)
	metaPath := filepath.Join(tmpDir, ref.SessionID, ref.MediaID+".meta")
	expiredMeta := struct {
		Filename  string    `json:"filename"`
		MIMEType  string    `json:"mime_type"`
		SizeBytes int64     `json:"size_bytes"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}{
		Filename:  "test.txt",
		MIMEType:  "text/plain",
		SizeBytes: 5,
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	metaData, _ := json.Marshal(expiredMeta)
	if err := os.WriteFile(metaPath, metaData, 0600); err != nil {
		t.Fatalf("Failed to write expired metadata: %v", err)
	}

	// Run cleanup - should remove the expired file
	cleaned, err := storage.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}

	if cleaned != 1 {
		t.Errorf("CleanupExpired() cleaned %d files, want 1", cleaned)
	}

	// File should no longer exist
	_, err = storage.GetMediaInfo(ctx, creds.StorageRef)
	if err != ErrMediaNotFound && err != ErrMediaExpired {
		t.Errorf("Expected ErrMediaNotFound or ErrMediaExpired, got: %v", err)
	}
}

func TestLocalStorage_CleanupExpired_WithNonDirEntries(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Create a regular file (not a directory) in the base path
	// This tests the non-directory entry skip path
	if err := os.WriteFile(filepath.Join(tmpDir, "not-a-dir.txt"), []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Run cleanup - should not error and should skip the file
	cleaned, err := storage.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if cleaned != 0 {
		t.Errorf("CleanupExpired() cleaned %d files, want 0", cleaned)
	}
}

func TestNewLocalStorage_InvalidPath(t *testing.T) {
	// Try to create storage with an invalid path (file instead of directory)
	tmpFile, err := os.CreateTemp("", "invalid-path-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Use the file path as base path - MkdirAll will fail
	config := DefaultLocalStorageConfig(filepath.Join(tmpFile.Name(), "subdir"), "http://localhost:8080")
	_, err = NewLocalStorage(config)
	if err == nil {
		t.Error("Expected error when creating storage with invalid path")
	}
}

func TestLocalStorage_DeleteSessionMedia_NotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "media-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	config := DefaultLocalStorageConfig(tmpDir, "http://localhost:8080")
	storage, err := NewLocalStorage(config)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	ctx := context.Background()

	// Delete non-existent session - should not error
	err = storage.DeleteSessionMedia(ctx, "nonexistent-session")
	if err != nil {
		t.Errorf("DeleteSessionMedia() error = %v, want nil", err)
	}
}
