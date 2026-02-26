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

package pgutil

import (
	"encoding/json"
	"time"
)

// NullString returns nil when s is empty, otherwise a pointer to s.
// Useful for mapping Go strings to nullable TEXT/VARCHAR columns.
func NullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DerefString returns the empty string when s is nil, otherwise *s.
func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// NullInt returns nil when v is zero, otherwise a pointer to v.
func NullInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// NullInt32 returns nil when v is zero, otherwise a pointer to v.
func NullInt32(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}

// NullInt64 returns nil when v is zero, otherwise a pointer to v.
func NullInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// NullTime returns nil when t is the zero value, otherwise a pointer to t.
func NullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// TimeOrZero returns the zero time when t is nil, otherwise *t.
func TimeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// MarshalJSONB marshals m to JSON bytes. Returns "{}" when m is nil.
func MarshalJSONB(m map[string]string) []byte {
	if m == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(m)
	return b
}

// UnmarshalJSONB unmarshals JSON bytes into a map[string]string.
// Returns nil when data is empty or does not contain valid key/value pairs.
func UnmarshalJSONB(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(data, &m) != nil || len(m) == 0 {
		return nil
	}
	return m
}
