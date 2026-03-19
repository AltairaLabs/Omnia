/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string not truncated",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length not truncated",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string truncated",
			input:  "this is a long message with sensitive data",
			maxLen: 10,
			want:   "this is a " + truncatedSuffix,
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "zero maxLen",
			input:  "hello",
			maxLen: 0,
			want:   truncatedSuffix,
		},
		{
			name:   "negative maxLen",
			input:  "hello",
			maxLen: -1,
			want:   "",
		},
		{
			name:   "unicode string truncated correctly",
			input:  "hello world",
			maxLen: 7,
			want:   "hello w" + truncatedSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHashID(t *testing.T) {
	t.Run("consistent hashes", func(t *testing.T) {
		h1 := HashID("user-123")
		h2 := HashID("user-123")
		assert.Equal(t, h1, h2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1 := HashID("user-123")
		h2 := HashID("user-456")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("format is correct", func(t *testing.T) {
		h := HashID("test-id")
		assert.Regexp(t, `^\[hash:[0-9a-f]{12}\]$`, h)
	})

	t.Run("empty string produces valid hash", func(t *testing.T) {
		h := HashID("")
		assert.Regexp(t, `^\[hash:[0-9a-f]{12}\]$`, h)
	})
}

func TestSafeMapKeys(t *testing.T) {
	t.Run("returns sorted keys", func(t *testing.T) {
		m := map[string]interface{}{
			"zebra":  "secret-value",
			"alpha":  42,
			"middle": true,
		}
		keys := SafeMapKeys(m)
		require.Len(t, keys, 3)
		assert.Equal(t, []string{"alpha", "middle", "zebra"}, keys)
	})

	t.Run("empty map returns empty slice", func(t *testing.T) {
		m := map[string]interface{}{}
		keys := SafeMapKeys(m)
		assert.Empty(t, keys)
	})

	t.Run("nil map returns empty slice", func(t *testing.T) {
		keys := SafeMapKeys(nil)
		assert.Empty(t, keys)
	})
}

func TestContentLength(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 0},
		{"short string", "hello", 5},
		{"longer string", "this is a test message", 22},
		{"unicode bytes", "\u00e9", 2}, // UTF-8 encoded length
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContentLength(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
