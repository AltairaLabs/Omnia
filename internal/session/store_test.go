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

package session

import (
	"testing"
	"time"
)

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "zero expiry - never expires",
			expiresAt: time.Time{},
			want:      false,
		},
		{
			name:      "future expiry - not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "past expiry - expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "just expired",
			expiresAt: time.Now().Add(-1 * time.Second),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{
				ExpiresAt: tt.expiresAt,
			}
			if got := s.IsExpired(); got != tt.want {
				t.Errorf("Session.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessageRole_Constants(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q, want %q", RoleAssistant, "assistant")
	}
	if RoleSystem != "system" {
		t.Errorf("RoleSystem = %q, want %q", RoleSystem, "system")
	}
}

func TestCommonErrors(t *testing.T) {
	if ErrSessionNotFound == nil {
		t.Error("ErrSessionNotFound should not be nil")
	}
	if ErrSessionExpired == nil {
		t.Error("ErrSessionExpired should not be nil")
	}
	if ErrInvalidSessionID == nil {
		t.Error("ErrInvalidSessionID should not be nil")
	}

	// Test error messages
	if ErrSessionNotFound.Error() != "session not found" {
		t.Errorf("ErrSessionNotFound.Error() = %q, want %q", ErrSessionNotFound.Error(), "session not found")
	}
	if ErrSessionExpired.Error() != "session expired" {
		t.Errorf("ErrSessionExpired.Error() = %q, want %q", ErrSessionExpired.Error(), "session expired")
	}
	if ErrInvalidSessionID.Error() != "invalid session ID" {
		t.Errorf("ErrInvalidSessionID.Error() = %q, want %q", ErrInvalidSessionID.Error(), "invalid session ID")
	}
}
