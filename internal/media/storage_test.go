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
)

func TestParseStorageRef(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		want      *StorageRef
		wantErr   bool
		errString string
	}{
		{
			name: "valid reference",
			ref:  "omnia://sessions/abc123/media/file456",
			want: &StorageRef{
				SessionID: "abc123",
				MediaID:   "file456",
			},
			wantErr: false,
		},
		{
			name:      "missing scheme",
			ref:       "sessions/abc123/media/file456",
			want:      nil,
			wantErr:   true,
			errString: "missing scheme prefix",
		},
		{
			name:      "wrong scheme",
			ref:       "s3://sessions/abc123/media/file456",
			want:      nil,
			wantErr:   true,
			errString: "missing scheme prefix",
		},
		{
			name:      "invalid path format - missing segments",
			ref:       "omnia://abc123/file456",
			want:      nil,
			wantErr:   true,
			errString: "invalid path format",
		},
		{
			name:      "invalid path format - wrong prefix",
			ref:       "omnia://buckets/abc123/files/file456",
			want:      nil,
			wantErr:   true,
			errString: "invalid path format",
		},
		{
			name:      "empty session ID",
			ref:       "omnia://sessions//media/file456",
			want:      nil,
			wantErr:   true,
			errString: "empty session ID",
		},
		{
			name:      "empty media ID",
			ref:       "omnia://sessions/abc123/media/",
			want:      nil,
			wantErr:   true,
			errString: "empty media ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStorageRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStorageRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errString != "" && err != nil {
					if !contains(err.Error(), tt.errString) {
						t.Errorf("ParseStorageRef() error = %v, want error containing %q", err, tt.errString)
					}
				}
				return
			}
			if got.SessionID != tt.want.SessionID || got.MediaID != tt.want.MediaID {
				t.Errorf("ParseStorageRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStorageRefString(t *testing.T) {
	ref := StorageRef{
		SessionID: "session-123",
		MediaID:   "media-456",
	}

	expected := "omnia://sessions/session-123/media/media-456"
	if got := ref.String(); got != expected {
		t.Errorf("StorageRef.String() = %v, want %v", got, expected)
	}
}

func TestMediaInfoIsExpired(t *testing.T) {
	tests := []struct {
		name string
		info MediaInfo
		want bool
	}{
		{
			name: "zero expiry - not expired",
			info: MediaInfo{},
			want: false,
		},
		// Note: Testing actual expiry requires time manipulation
		// which we can add with a clock interface if needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsExpired(); got != tt.want {
				t.Errorf("MediaInfo.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
