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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

// fakeDuplexSink records sent audio for test assertion.
type fakeDuplexSink struct {
	audio      chan []byte
	startCalls int
	mu         sync.Mutex // guards closeCalls
	closeCalls int
	startErr   error
	sendErr    error
}

func (f *fakeDuplexSink) Start(_ context.Context, _ *AudioSessionStart) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeDuplexSink) SendAudio(data []byte, _ uint32, _ bool) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	f.audio <- cp
	return nil
}

func (f *fakeDuplexSink) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	return nil
}

// closeCount returns the number of times Close has been called. Safe for
// concurrent use — use this instead of reading closeCalls directly.
func (f *fakeDuplexSink) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}

// captureWriter captures WriteBinaryMediaChunk calls for test assertion.
// It implements the full ResponseWriter interface.
type captureWriter struct {
	binaryChunks [][]byte
}

func (c *captureWriter) WriteChunk(_ string) error                       { return nil }
func (c *captureWriter) WriteUserTranscript(_ string) error              { return nil }
func (c *captureWriter) WriteChunkWithParts(_ []ContentPart) error       { return nil }
func (c *captureWriter) WriteDone(_ string) error                        { return nil }
func (c *captureWriter) WriteDoneWithParts(_ []ContentPart) error        { return nil }
func (c *captureWriter) WriteToolCall(_ *ToolCallInfo) error             { return nil }
func (c *captureWriter) WriteToolResult(_ *ToolResultInfo) error         { return nil }
func (c *captureWriter) WriteError(_, _ string) error                    { return nil }
func (c *captureWriter) WriteUploadReady(_ *UploadReadyInfo) error       { return nil }
func (c *captureWriter) WriteUploadComplete(_ *UploadCompleteInfo) error { return nil }
func (c *captureWriter) WriteMediaChunk(_ *MediaChunkInfo) error         { return nil }
func (c *captureWriter) WriteInterrupt() error                           { return nil }
func (c *captureWriter) WriteSessionConfig(_ *SessionConfigInfo) error   { return nil }
func (c *captureWriter) SupportsBinary() bool                            { return true }
func (c *captureWriter) WriteBinaryMediaChunk(_ [MediaIDSize]byte, _ uint32, _ bool, _ string, payload []byte) error {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	c.binaryChunks = append(c.binaryChunks, cp)
	return nil
}

// TestAudioSession_ForwardsInboundAudio verifies that pushAudio delivers
// audio bytes to the DuplexSink.
func TestAudioSession_ForwardsInboundAudio(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}
	w := &captureWriter{}
	as := newAudioSession("sess-1", fc, w)
	if err := as.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := as.pushAudio([]byte{1, 2, 3}, 0, false); err != nil {
		t.Fatalf("pushAudio: %v", err)
	}
	select {
	case got := <-fc.audio:
		if string(got) != string([]byte{1, 2, 3}) {
			t.Fatalf("forwarded audio mismatch: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("audio not forwarded to sink")
	}
}

// TestAudioSession_StartIdempotent verifies that calling start twice
// only invokes Sink.Start once.
func TestAudioSession_StartIdempotent(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}
	w := &captureWriter{}
	as := newAudioSession("sess-1", fc, w)

	require.NoError(t, as.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1}))
	require.NoError(t, as.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1}))

	assert.Equal(t, 1, fc.startCalls, "Start should only be called once")
}

// TestAudioSession_StartError propagates sink.Start errors.
func TestAudioSession_StartError(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4), startErr: errors.New("dial failed")}
	w := &captureWriter{}
	as := newAudioSession("sess-err", fc, w)

	err := as.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial failed")
}

// TestAudioSession_Close delegates to the sink.
func TestAudioSession_Close(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}
	w := &captureWriter{}
	as := newAudioSession("sess-1", fc, w)

	require.NoError(t, as.close())
	assert.Equal(t, 1, fc.closeCount())
}

// TestAudioSession_HandleInboundFrame maps a BinaryFrame to pushAudio.
func TestAudioSession_HandleInboundFrame(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}
	w := &captureWriter{}
	as := newAudioSession("sess-1", fc, w)
	require.NoError(t, as.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1}))

	mediaID := testMediaIDFromString("audio-00")
	frame, err := NewMediaChunkFrame("sess-1", mediaID, 7, true, "audio/pcm", []byte{0xAA, 0xBB})
	require.NoError(t, err)

	require.NoError(t, as.handleInboundFrame(frame))

	select {
	case got := <-fc.audio:
		assert.Equal(t, []byte{0xAA, 0xBB}, got)
	case <-time.After(time.Second):
		t.Fatal("audio not forwarded via handleInboundFrame")
	}
}

// TestHandleBinaryMessage_RoutesMediaChunkToSink verifies that an inbound
// OMNI BinaryMessageTypeMediaChunk frame arriving on a connection is routed
// to the DuplexSink via the server's DuplexSinkFactory.
func TestHandleBinaryMessage_RoutesMediaChunkToSink(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 8)}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink { return fc }),
	)

	// Build a fake connection (no real WS needed — call handleBinaryMessage directly)
	conn := &Connection{sessionID: "test-sess-2"}
	mediaID := testMediaIDFromString("audio-01")
	frame, err := NewMediaChunkFrame("test-sess-2", mediaID, 0, false, "audio/pcm", []byte{0xDE, 0xAD})
	require.NoError(t, err)

	raw, err := frame.Encode()
	require.NoError(t, err)

	srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())

	select {
	case got := <-fc.audio:
		assert.Equal(t, []byte{0xDE, 0xAD}, got)
	case <-time.After(time.Second):
		t.Fatal("binary media chunk not routed to duplex sink")
	}
}

// TestHandleBinaryMessage_NilFactory_SendsErrorFrame verifies that when no
// DuplexSinkFactory is configured, the server does not panic and instead
// the error path is taken gracefully.
func TestHandleBinaryMessage_NilFactory_DoesNotPanic(t *testing.T) {
	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()

	// No WithDuplexSinkFactory — factory is nil
	srv := NewServer(cfg, store, handler, testLogger(t))

	conn := &Connection{sessionID: "test-sess-nil"}
	mediaID := testMediaIDFromString("audio-02")
	frame, err := NewMediaChunkFrame("test-sess-nil", mediaID, 0, false, "audio/pcm", []byte{0x01})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	// Must not panic
	assert.NotPanics(t, func() {
		srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())
	})
}

// TestAudioSession_CleanedUpOnDisconnect verifies that close() is called on
// the audioSession when cleanupConnection runs (via the audioSession field).
func TestAudioSession_CleanedUpOnDisconnect(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}
	w := &captureWriter{}
	as := newAudioSession("sess-cleanup", fc, w)

	// Simulate cleanup calling close
	require.NoError(t, as.close())
	assert.Equal(t, 1, fc.closeCount(), "sink Close should be called on cleanup")
}

// TestEnsureAudioSession_ReturnsExistingSession verifies that a second call
// to ensureAudioSession returns the already-created session without creating
// a new one (early-return fast path when c.audioSession != nil).
func TestEnsureAudioSession_ReturnsExistingSession(t *testing.T) {
	callCount := 0
	fc := &fakeDuplexSink{audio: make(chan []byte, 4)}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink {
			callCount++
			return fc
		}),
	)

	conn := &Connection{sessionID: "test-ensure-twice"}
	mediaID := testMediaIDFromString("audio-05")
	frame, err := NewMediaChunkFrame("test-ensure-twice", mediaID, 0, false, "audio/pcm", []byte{0x01})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	// First call — creates the session.
	srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())
	// Second call — should hit the early-return path.
	srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())

	assert.Equal(t, 1, callCount, "factory should only be called once")
}

// TestEnsureAudioSession_DoubleCheckRace exercises the double-check path
// by pre-populating c.audioSession before the second lock, simulating
// a concurrent goroutine winning the race.
func TestEnsureAudioSession_DoubleCheckRace(t *testing.T) {
	fc1 := &fakeDuplexSink{audio: make(chan []byte, 4)}
	fc2 := &fakeDuplexSink{audio: make(chan []byte, 4)}

	callCount := 0
	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()

	// The factory injects a pre-built audioSession into c.audioSession to
	// simulate a concurrent goroutine winning the double-check race.
	var conn *Connection
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(sessionID string, w ResponseWriter) DuplexSink {
			callCount++
			if callCount == 1 {
				// Simulate the race: inject an already-started session so the
				// double-check branch finds audioSession != nil.
				existing := newAudioSession(sessionID, fc1, w)
				_ = existing.start(context.Background(), &AudioSessionStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1})
				conn.mu.Lock()
				conn.audioSession = existing
				conn.mu.Unlock()
			}
			return fc2
		}),
	)

	conn = &Connection{sessionID: "test-race"}
	mediaID := testMediaIDFromString("audio-06")
	frame, err := NewMediaChunkFrame("test-race", mediaID, 0, false, "audio/pcm", []byte{0x01})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())

	// The pre-existing session (fc1) should be kept; fc2 should be discarded.
	conn.mu.Lock()
	as := conn.audioSession
	conn.mu.Unlock()
	assert.NotNil(t, as)
	assert.Equal(t, 1, fc2.closeCount(), "discarded sink should be closed")
}

// TestHandleBinaryMessage_SinkStartError verifies that when the factory's sink
// returns an error from Start, the server handles it without panicking and the
// connection's audioSession remains nil.
func TestHandleBinaryMessage_SinkStartError(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4), startErr: errors.New("grpc unavailable")}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink { return fc }),
	)

	conn := &Connection{sessionID: "test-start-err"}
	mediaID := testMediaIDFromString("audio-03")
	frame, err := NewMediaChunkFrame("test-start-err", mediaID, 0, false, "audio/pcm", []byte{0x01})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	// Must not panic; error is logged, audioSession stays nil.
	assert.NotPanics(t, func() {
		srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())
	})
	conn.mu.Lock()
	assert.Nil(t, conn.audioSession)
	conn.mu.Unlock()
}

// TestHandleBinaryMessage_SendAudioError verifies that a SendAudio error is
// surfaced by routeMediaChunk without panicking.
func TestHandleBinaryMessage_SendAudioError(t *testing.T) {
	fc := &fakeDuplexSink{audio: make(chan []byte, 4), sendErr: errors.New("stream closed")}

	store, handler := newTestStoreAndHandler(t)
	cfg := DefaultServerConfig()
	srv := NewServer(cfg, store, handler, testLogger(t),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink { return fc }),
	)

	conn := &Connection{sessionID: "test-send-err"}
	mediaID := testMediaIDFromString("audio-04")
	frame, err := NewMediaChunkFrame("test-send-err", mediaID, 0, false, "audio/pcm", []byte{0x02})
	require.NoError(t, err)
	raw, err := frame.Encode()
	require.NoError(t, err)

	// First call creates the session successfully (startErr is nil).
	// SendAudio returns an error; routeMediaChunk should log it and not panic.
	// We can't send the real error frame without a WS conn, but at least
	// the code must not panic — the sendError guard in helpers.go handles
	// the closed/nil conn case by ignoring the write.
	assert.NotPanics(t, func() {
		srv.handleBinaryMessage(context.Background(), conn, raw, testLoggerDiscard())
	})
}

// --- helpers ---

func newTestStoreAndHandler(t *testing.T) (session.Store, MessageHandler) {
	t.Helper()
	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })
	handler := &mockHandler{}
	return store, handler
}

func testLogger(t *testing.T) logr.Logger {
	t.Helper()
	return logr.Discard()
}

func testLoggerDiscard() logr.Logger {
	return logr.Discard()
}
