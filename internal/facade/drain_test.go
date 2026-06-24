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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestDrain_ReturnsWhenSessionsZero(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.DrainTimeout = time.Second
	s := NewServer(cfg, nil, nil, logr.Discard())
	// No active or parked sessions → drains immediately.
	if left := s.Drain(context.Background()); left != 0 {
		t.Fatalf("want 0 left, got %d", left)
	}
	if !s.IsDraining() {
		t.Fatal("Drain must mark draining")
	}
}

func TestDrain_DeadlineReturnsRemaining(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.DrainTimeout = 20 * time.Millisecond
	s := NewServer(cfg, nil, nil, logr.Discard())
	// Park one session so the count stays > 0 for the whole window.
	s.parked.park(context.Background(), "sid", "u", newAudioSession("sid", &fakeDuplexSink{audio: make(chan []byte, 1)}, nil))
	left := s.Drain(context.Background())
	if left < 1 {
		t.Fatalf("want >=1 remaining at deadline, got %d", left)
	}
}
