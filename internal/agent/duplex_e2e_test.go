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

package agent

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/runtime/duplexmock"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

// wsURLFromHTTP converts an http:// URL to a ws:// URL.
func wsURLFromHTTP(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// connectedMsg is the minimal shape of the "connected" server message.
type connectedMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
}

// readConnectedE2E reads the initial "connected" JSON message and returns the sessionID.
func readConnectedE2E(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("readConnected: ReadMessage: %v", err)
	}
	var msg connectedMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("readConnected: unmarshal: %v (raw: %q)", err, data)
	}
	if msg.SessionID == "" {
		t.Fatalf("readConnected: empty session_id in message: %s", data)
	}
	return msg.SessionID
}

// TestDuplexAudio_EndToEnd_WSToRuntimeEcho proves the complete duplex audio
// wiring path:
//
//	WebSocket client → facade (binary frame) → grpcDuplexSink → runtime Converse
//	→ duplexmock echo → MediaChunk → relayOut → WriteBinaryMediaChunk → WS binary frame.
//
// This is a wiring test per the repo convention: it starts real in-process
// servers (runtime gRPC, facade HTTP/WS) and drives them end-to-end.
func TestDuplexAudio_EndToEnd_WSToRuntimeEcho(t *testing.T) {
	// --- 1. Start the real runtime gRPC server backed by duplexmock ---
	runtimeAddr, runtimeCleanup := startRealRuntimeServer(t, duplexmock.New())
	defer runtimeCleanup()

	runtimeClient, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     runtimeAddr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntimeClient: %v", err)
	}
	defer func() { _ = runtimeClient.Close() }()

	// --- 2. Build a real facade.Server with WithDuplexSinkFactory ---
	store := sessiontest.NewStore()
	defer func() { _ = store.Close() }()

	cfg := facade.DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	// The factory creates a grpcDuplexSink per connection, wired to the
	// real runtime client. This is exactly what cmd/agent does at binary
	// wiring time.
	factory := func(sessionID string, w facade.ResponseWriter) facade.DuplexSink {
		return NewGRPCDuplexSink(sessionID, runtimeClient, w)
	}

	server := facade.NewServer(
		cfg,
		store,
		nil, // no text handler needed for this duplex test
		logr.Discard(),
		facade.WithDuplexSinkFactory(factory),
	)

	ts := httptest.NewServer(server)
	defer ts.Close()

	// --- 3. Dial with a binary-capable WebSocket client ---
	// ?binary=true sets Connection.binaryCapable = true so WriteBinaryMediaChunk
	// sends real binary frames rather than falling back to base64 JSON.
	dialURL := wsURLFromHTTP(ts.URL) + "?agent=test-agent&binary=true"
	ws, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read the eagerly-sent "connected" message to obtain the session ID.
	sessionID := readConnectedE2E(t, ws)

	// --- 4. Build and send a BinaryMessageTypeMediaChunk frame ---
	audioPayload := []byte{0xAA, 0xBB, 0xCC}
	var mediaID [facade.MediaIDSize]byte
	copy(mediaID[:], "test-media-id")

	frame, err := facade.NewMediaChunkFrame(sessionID, mediaID, 0, false, "audio/pcm", audioPayload)
	if err != nil {
		t.Fatalf("NewMediaChunkFrame: %v", err)
	}
	encoded, err := frame.Encode()
	if err != nil {
		t.Fatalf("frame.Encode: %v", err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, encoded); err != nil {
		t.Fatalf("WriteMessage (audio frame): %v", err)
	}

	// --- 5. Wait for the echoed binary frame from the facade ---
	// The duplexmock echoes every audio chunk as a MediaChunk; relayOut
	// forwards it via WriteBinaryMediaChunk → sendBinaryFrame → WS binary message.
	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout: no binary MediaChunk echo received from facade within 5s")
		default:
		}

		ws.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) //nolint:errcheck
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			// Timeout on this particular read is fine — keep looping.
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}
			t.Fatalf("ReadMessage: %v", err)
		}
		if msgType != websocket.BinaryMessage {
			// JSON text frames (e.g. errors, metrics) — skip and keep polling.
			continue
		}

		// Decode and assert the echoed payload.
		decoded, decErr := facade.DecodeBinaryFrame(data)
		if decErr != nil {
			t.Fatalf("DecodeBinaryFrame: %v", decErr)
		}
		if string(decoded.Payload) != string(audioPayload) {
			t.Fatalf("echo payload = %v, want %v", decoded.Payload, audioPayload)
		}
		// Success — the full path is wired end-to-end.
		return
	}
}
