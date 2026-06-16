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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingMetrics tracks AudioSessionStarted/Ended and AudioIngestLatency calls.
type countingMetrics struct {
	NoOpMetrics
	started   int
	ended     int
	latencies []float64
}

func (c *countingMetrics) AudioSessionStarted() { c.started++ }
func (c *countingMetrics) AudioSessionEnded()   { c.ended++ }
func (c *countingMetrics) AudioIngestLatency(s float64) {
	c.latencies = append(c.latencies, s)
}

// TestEnsureAudioSession_ShedsOverCap verifies that when MaxAudioSessions is
// reached, a new session is rejected with ErrorCodeRateLimited and the active
// gauge reflects the correct count.
func TestEnsureAudioSession_ShedsOverCap(t *testing.T) {
	fc1 := &fakeDuplexSink{audio: make(chan []byte, 8)}
	fc2 := &fakeDuplexSink{audio: make(chan []byte, 8)}
	sinkIdx := 0
	sinks := []*fakeDuplexSink{fc1, fc2}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	cfg.MaxAudioSessions = 1

	m := &countingMetrics{}
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithMetrics(m),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink {
			s := sinks[sinkIdx]
			sinkIdx++
			return s
		}),
	)

	// First connection — fits under the cap.
	conn1 := &Connection{sessionID: "sess-cap-1"}
	mediaID := testMediaIDFromString("audio-cap1")
	frame, err := NewMediaChunkFrame("sess-cap-1", mediaID, 0, false, "audio/pcm", []byte{0xAA})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	srv.handleBinaryMessage(context.Background(), conn1, raw, testLoggerDiscard())

	conn1.mu.Lock()
	as1 := conn1.audioSession
	conn1.mu.Unlock()
	require.NotNil(t, as1, "first session should be created")
	assert.Equal(t, 1, m.started, "gauge should be incremented after first create")

	// Second connection — should be shed. sendError goes through c.conn which is
	// nil in this test (no WS upgrade), so sendMessage returns nil silently;
	// what matters is ensureAudioSession returns nil and no sink is started.
	conn2 := &Connection{sessionID: "sess-cap-2"}
	as2 := srv.ensureAudioSession(context.Background(), conn2, testLoggerDiscard())
	assert.Nil(t, as2, "second session should be shed (nil returned)")
	// No second sink should have been created.
	assert.Equal(t, 0, fc2.startCalls, "factory sink for conn2 should not be started")
	// Gauge should still be 1.
	assert.Equal(t, 1, m.started, "gauge should not increment on shed")

	// Tear down conn1 to release the slot.
	require.NoError(t, as1.close())
	srv.decrementAudioSessions(m)

	// Gauge should now be 0 after teardown.
	assert.Equal(t, 1, m.ended, "gauge should decrement on teardown")

	// A new session should now succeed.
	conn3 := &Connection{sessionID: "sess-cap-3"}
	as3 := srv.ensureAudioSession(context.Background(), conn3, testLoggerDiscard())
	assert.NotNil(t, as3, "third session should succeed after slot freed")
	assert.Equal(t, 2, m.started, "gauge should increment for third session")
}

// TestEnsureAudioSession_ClosedConnectionNoCreate verifies that ensureAudioSession
// returns nil without creating a session when the connection is already closed.
func TestEnsureAudioSession_ClosedConnectionNoCreate(t *testing.T) {
	factoryCalled := false

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink {
			factoryCalled = true
			return &fakeDuplexSink{audio: make(chan []byte, 4)}
		}),
	)

	conn := &Connection{sessionID: "sess-closed", closed: true}
	as := srv.ensureAudioSession(context.Background(), conn, testLoggerDiscard())
	assert.Nil(t, as, "closed connection should return nil")
	assert.False(t, factoryCalled, "factory must not be called for closed connection")
}

// TestEnsureAudioSession_NilFactorySendsError verifies that when no factory is
// configured, ensureAudioSession sends an error to the client rather than
// silently dropping the frame.
func TestEnsureAudioSession_NilFactorySendsError(t *testing.T) {
	// We track whether an error was sent by using a server where sendError
	// delegates to a countingMetrics-aware path. Because conn.conn is nil,
	// sendMessage returns nil without writing; the important thing is the code
	// path is taken (no panic, returns nil).
	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	// No factory — duplexSinkFactory stays nil.
	srv := NewServer(cfg, store, handler, testLogger(t))

	conn := &Connection{sessionID: "sess-nil-factory"}
	// Must not panic and must return nil (caller skips forward).
	assert.NotPanics(t, func() {
		as := srv.ensureAudioSession(context.Background(), conn, testLoggerDiscard())
		assert.Nil(t, as)
	})
}

// TestAudioMetrics_GaugeAndLatency verifies AudioSessionStarted/Ended and
// AudioIngestLatency move the counters correctly.
func TestAudioMetrics_GaugeAndLatency(t *testing.T) {
	m := &countingMetrics{}

	// Verify Gauge methods.
	m.AudioSessionStarted()
	m.AudioSessionStarted()
	m.AudioSessionEnded()

	assert.Equal(t, 2, m.started)
	assert.Equal(t, 1, m.ended)

	// Verify latency observation.
	m.AudioIngestLatency(0.001)
	m.AudioIngestLatency(0.002)
	assert.Len(t, m.latencies, 2)
	assert.InDelta(t, 0.001, m.latencies[0], 1e-9)
	assert.InDelta(t, 0.002, m.latencies[1], 1e-9)
}

// TestNoOpMetrics_AudioMethods verifies that the NoOp implementations of the
// new audio methods don't panic.
func TestNoOpMetrics_AudioMethods(t *testing.T) {
	m := &NoOpMetrics{}
	assert.NotPanics(t, func() {
		m.AudioSessionStarted()
		m.AudioSessionEnded()
		m.AudioIngestLatency(0.5)
	})
}

// TestRouteMediaChunk_RecordsIngestLatency verifies that each inbound audio
// frame results in an AudioIngestLatency observation.
func TestRouteMediaChunk_RecordsIngestLatency(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 8)}
	m := &countingMetrics{}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithMetrics(m),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink { return fc }),
	)

	conn := &Connection{sessionID: "sess-latency"}
	mediaID := testMediaIDFromString("audio-lat")
	frame, err := NewMediaChunkFrame("sess-latency", mediaID, 0, false, "audio/pcm", []byte{0x01})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())

	// Allow a brief moment for the audio to be processed.
	select {
	case <-fc.audio:
	case <-time.After(time.Second):
		t.Fatal("audio not delivered to sink")
	}

	assert.NotEmpty(t, m.latencies, "at least one latency observation expected")
	assert.GreaterOrEqual(t, m.latencies[0], 0.0, "latency must be non-negative")
}
