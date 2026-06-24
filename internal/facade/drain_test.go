/*
Copyright 2025-2026.

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

package facade

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestServeHTTP_RejectsNewUpgradesWhenDraining(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard())
	s.markDraining()
	if !s.IsDraining() {
		t.Fatal("IsDraining should be true after markDraining")
	}
	r := httptest.NewRequest(http.MethodGet, "/ws?agent=a&namespace=n", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 while draining, got %d", w.Code)
	}
}

func TestMarkDraining_IsIdempotent(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard())
	s.markDraining()
	s.markDraining() // second call must not panic
	if !s.IsDraining() {
		t.Fatal("IsDraining should be true after repeated markDraining calls")
	}
}
