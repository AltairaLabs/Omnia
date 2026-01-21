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
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
)

func TestDefaultAzureConfig(t *testing.T) {
	cfg := DefaultAzureConfig("myaccount", "mycontainer")

	if cfg.AccountName != "myaccount" {
		t.Errorf("AccountName = %q, want %q", cfg.AccountName, "myaccount")
	}
	if cfg.ContainerName != "mycontainer" {
		t.Errorf("ContainerName = %q, want %q", cfg.ContainerName, "mycontainer")
	}
	if cfg.UploadURLTTL != 15*time.Minute {
		t.Errorf("UploadURLTTL = %v, want %v", cfg.UploadURLTTL, 15*time.Minute)
	}
	if cfg.DownloadURLTTL != 1*time.Hour {
		t.Errorf("DownloadURLTTL = %v, want %v", cfg.DownloadURLTTL, 1*time.Hour)
	}
	if cfg.DefaultTTL != 24*time.Hour {
		t.Errorf("DefaultTTL = %v, want %v", cfg.DefaultTTL, 24*time.Hour)
	}
	if cfg.MaxFileSize != 100*1024*1024 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 100*1024*1024)
	}
}

func TestAzureStorage_sessionPrefix(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		sessionID string
		want      string
	}{
		{
			name:      "with prefix",
			prefix:    "media",
			sessionID: "session-123",
			want:      "media/session-123/",
		},
		{
			name:      "without prefix",
			prefix:    "",
			sessionID: "session-456",
			want:      "session-456/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AzureStorage{
				config: AzureConfig{Prefix: tt.prefix},
			}
			got := a.sessionPrefix(tt.sessionID)
			if got != tt.want {
				t.Errorf("sessionPrefix(%q) = %q, want %q", tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestAzureStorage_blobName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ref    *StorageRef
		want   string
	}{
		{
			name:   "with prefix",
			prefix: "media",
			ref:    &StorageRef{SessionID: "sess-1", MediaID: "media-1"},
			want:   "media/sess-1/media-1",
		},
		{
			name:   "without prefix",
			prefix: "",
			ref:    &StorageRef{SessionID: "sess-2", MediaID: "media-2"},
			want:   "sess-2/media-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AzureStorage{
				config: AzureConfig{Prefix: tt.prefix},
			}
			got := a.blobName(tt.ref)
			if got != tt.want {
				t.Errorf("blobName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAzureStorage_metadataBlobName(t *testing.T) {
	a := &AzureStorage{
		config: AzureConfig{Prefix: "media"},
	}
	ref := &StorageRef{SessionID: "sess-1", MediaID: "media-1"}

	got := a.metadataBlobName(ref)
	want := "media/sess-1/media-1.meta" //nolint:goconst // test expectation value, intentionally explicit

	if got != want {
		t.Errorf("metadataBlobName() = %q, want %q", got, want)
	}
}

func TestAzureStorage_generateID(t *testing.T) {
	a := &AzureStorage{}

	id1, err := a.generateID()
	if err != nil {
		t.Fatalf("generateID() error = %v", err)
	}
	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("generateID() returned ID of length %d, want 32", len(id1))
	}

	// Generate another ID to ensure they're unique
	id2, err := a.generateID()
	if err != nil {
		t.Fatalf("generateID() error = %v", err)
	}
	if id1 == id2 {
		t.Error("generateID() returned duplicate IDs")
	}
}

func TestAzureStorage_Close(t *testing.T) {
	a := &AzureStorage{
		pendingUploads: map[string]*azurePendingUpload{
			"upload-1": {StorageRef: "test"},
		},
	}

	err := a.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if len(a.pendingUploads) != 0 {
		t.Errorf("Close() did not clear pendingUploads, got %d entries", len(a.pendingUploads))
	}
}

func TestAzureStorage_generateSASURL_NoCredential(t *testing.T) {
	a := &AzureStorage{
		sharedKeyCredential: nil,
	}

	_, err := a.generateSASURL("test-blob", sas.BlobPermissions{Read: true}, time.Now().Add(time.Hour))
	if err == nil {
		t.Error("generateSASURL() expected error when no shared key credential")
	}
	if err.Error() != "SAS URL generation requires shared key credential; set AccountKey in config" {
		t.Errorf("generateSASURL() error = %q, want credential error", err.Error())
	}
}
