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
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

const testParkSessionID = "sid"

// testParkOwnerID is the owning userID used across parked-session tests.
const testParkOwnerID = "user-1"

func TestRegistry_ParkThenExpireClosesSession(t *testing.T) {
	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	as := newAudioSession(testParkSessionID, sink, nil)

	// expiredID is written by the timer goroutine and read by the test goroutine;
	// protect with a mutex to avoid a data race.
	var expiredMu sync.Mutex
	var expiredID string
	setExpired := func(id string) {
		expiredMu.Lock()
		expiredID = id
		expiredMu.Unlock()
	}
	getExpired := func() string {
		expiredMu.Lock()
		defer expiredMu.Unlock()
		return expiredID
	}

	reg := newRealtimeRegistry(noopRouteStore{}, "10.0.0.1:8080", 20*time.Millisecond, logr.Discard())
	reg.onExpire = func(id string, _ bool) { setExpired(id) }

	reg.park(context.Background(), testParkSessionID, testParkOwnerID, as, true)
	if reg.len() != 1 {
		t.Fatalf("want 1 parked, got %d", reg.len())
	}

	// Wait past the grace window; poll until the registry is empty AND
	// onExpire has been called (getExpired non-empty), so both happen-before
	// the assertions below with no data race.
	deadline := time.Now().Add(time.Second)
	for (reg.len() != 0 || getExpired() == "") && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if reg.len() != 0 {
		t.Fatalf("session not expired")
	}
	if sink.closeCount() == 0 {
		t.Fatalf("expired session was not closed")
	}
	if got := getExpired(); got != testParkSessionID {
		t.Fatalf("onExpire not called with sid, got %q", got)
	}
}

func TestRegistry_Take(t *testing.T) {
	newReg := func() (*realtimeRegistry, *fakeDuplexSink, *audioSession) {
		sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
		as := newAudioSession(testParkSessionID, sink, nil)
		reg := newRealtimeRegistry(noopRouteStore{}, "addr", time.Minute, logr.Discard())
		reg.park(context.Background(), testParkSessionID, testParkOwnerID, as, true)
		return reg, sink, as
	}

	t.Run("success returns session and stops timer", func(t *testing.T) {
		reg, sink, want := newReg()
		got, ok := reg.take(context.Background(), testParkSessionID, testParkOwnerID)
		if !ok || got != want {
			t.Fatalf("take failed: ok=%v got=%v", ok, got)
		}
		if reg.len() != 0 {
			t.Fatalf("session still parked after take")
		}
		// Past the (1m) window the timer must NOT have closed our session.
		time.Sleep(10 * time.Millisecond)
		if sink.closeCount() != 0 {
			t.Fatalf("reattached session was closed by a stale timer")
		}
	})

	t.Run("owner mismatch refuses", func(t *testing.T) {
		reg, _, _ := newReg()
		if _, ok := reg.take(context.Background(), testParkSessionID, "attacker"); ok {
			t.Fatalf("take succeeded for wrong owner")
		}
		if reg.len() != 1 {
			t.Fatalf("session removed on owner mismatch")
		}
	})

	t.Run("miss returns false", func(t *testing.T) {
		reg, _, _ := newReg()
		if _, ok := reg.take(context.Background(), "nope", testParkOwnerID); ok {
			t.Fatalf("take succeeded for unknown session")
		}
	})
}
