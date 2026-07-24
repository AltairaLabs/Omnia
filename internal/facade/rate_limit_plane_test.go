/*
Copyright 2026.

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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

// TestAdmitMessage_BinaryBypassesCountLimiter is the core regression for the
// wrong-language bug: an exhausted per-message COUNT limiter must NOT drop
// binary media frames. Audio streams at ~187 frames/s; gating it on message
// count dropped ~70% of frames, time-compressing the audio the provider heard.
func TestAdmitMessage_BinaryBypassesCountLimiter(t *testing.T) {
	c := &Connection{
		// Count limiter is fully drained; media limiter is nil (unlimited).
		rateLimiter: rate.NewLimiter(rate.Limit(1), 1),
	}
	now := time.Now()
	require.True(t, c.rateLimiter.AllowN(now, 1)) // drain the single burst token

	for i := 0; i < 500; i++ {
		admitted, _ := c.admitMessage(websocket.BinaryMessage, 256, now)
		require.Truef(t, admitted, "binary frame %d must not be shed by the count limiter", i)
	}
}

// TestAdmitMessage_BinaryBoundedByByteRate verifies the media plane still has a
// bound: with the byte-rate limiter set, sustained binary beyond the burst is
// shed (defense-in-depth against a flooding client).
func TestAdmitMessage_BinaryBoundedByByteRate(t *testing.T) {
	c := &Connection{
		mediaRateLimiter: rate.NewLimiter(rate.Limit(1000), 2000), // 1000 B/s, 2000 B burst
	}
	now := time.Now() // fixed clock → only the burst is available, no refill

	admitted, shed := 0, 0
	for i := 0; i < 100; i++ {
		ok, reason := c.admitMessage(websocket.BinaryMessage, 256, now)
		if ok {
			admitted++
		} else {
			shed++
			assert.Equal(t, "media rate limit exceeded", reason)
		}
	}
	// Burst 2000 / 256 ≈ 7 frames pass at a fixed instant; the rest are shed.
	assert.Positive(t, admitted, "burst frames should pass")
	assert.Positive(t, shed, "sustained binary beyond burst must be byte-rate limited")
}

// TestAdmitMessage_TextBoundedByCountLimiter verifies text/control frames keep
// the per-message count limit (the JSON-flood abuse vector).
func TestAdmitMessage_TextBoundedByCountLimiter(t *testing.T) {
	c := &Connection{
		rateLimiter: rate.NewLimiter(rate.Limit(5), 3), // burst 3
	}
	now := time.Now() // fixed clock → exactly the burst passes

	admitted, shed := 0, 0
	for i := 0; i < 20; i++ {
		ok, reason := c.admitMessage(websocket.TextMessage, 10, now)
		if ok {
			admitted++
		} else {
			shed++
			assert.Equal(t, "rate limit exceeded", reason)
		}
	}
	assert.Equal(t, 3, admitted, "only the burst of text messages should pass at a fixed instant")
	assert.Equal(t, 17, shed)
}

// TestAdmitMessage_NilLimitersAdmitAll verifies both planes are unlimited when
// their limiters are nil.
func TestAdmitMessage_NilLimitersAdmitAll(t *testing.T) {
	c := &Connection{} // both limiters nil
	now := time.Now()
	for i := 0; i < 50; i++ {
		okBin, _ := c.admitMessage(websocket.BinaryMessage, 1<<20, now)
		okTxt, _ := c.admitMessage(websocket.TextMessage, 10, now)
		require.True(t, okBin)
		require.True(t, okTxt)
	}
}

// TestReadMessageLoop_BinaryAudioReachesSinkDespiteLowCountLimit is the wiring
// test: it drives real binary media frames through the dialed WebSocket and the
// actual readMessageLoop with a count limit far below the frame count, and
// asserts EVERY frame reaches the runtime sink — proving audio is not gated by
// the per-message count limiter end-to-end.
func TestReadMessageLoop_BinaryAudioReachesSinkDespiteLowCountLimit(t *testing.T) {
	const frames = 60
	sink := &fakeDuplexSink{audio: make(chan []byte, frames+16)}

	store := sessiontest.NewStore()
	cfg := DefaultServerConfig()
	cfg.MessageRateLimit = 5 // far below `frames`; would drop ~90% if it gated audio
	cfg.MessageRateBurst = 5
	cfg.PingInterval = 100 * time.Millisecond
	server := NewServer(cfg, store, &mockHandler{}, logr.Discard(),
		WithDuplexSinkFactory(func(_ string, _ ResponseWriter) DuplexSink { return sink }),
	)
	ts := httptest.NewServer(server)
	t.Cleanup(func() { ts.Close(); _ = store.Close() })

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	sid := readConnected(t, ws)
	mediaID := testMediaIDFromString("audio-rl")
	for i := 0; i < frames; i++ {
		frame, err := NewMediaChunkFrame(sid, mediaID, uint32(i), false, "audio/pcm", []byte{0xAA, 0xBB, 0xCC, 0xDD}) //nolint:gosec // loop index, bounded by `frames`
		require.NoError(t, err)
		raw, err := frame.Encode()
		require.NoError(t, err)
		require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, raw))
	}

	received := 0
	timeout := time.After(5 * time.Second)
	for received < frames {
		select {
		case <-sink.audio:
			received++
		case <-timeout:
			t.Fatalf("only %d/%d audio frames reached the sink — audio is being count-rate-limited", received, frames)
		}
	}
}
