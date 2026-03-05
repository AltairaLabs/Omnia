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

package facade

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDeadlineResetOnTextMessage(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			// Send two messages in sequence. If the deadline is not
			// reset after the first write, the second write could
			// fail with a deadline-exceeded error on slow CI.
			if err := writer.WriteChunk("first"); err != nil {
				return err
			}
			return writer.WriteDone("second")
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	// Send message
	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "hi"}))

	// Read connected
	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))
	assert.Equal(t, MessageTypeConnected, connMsg.Type)

	// Read chunk
	var chunk ServerMessage
	require.NoError(t, ws.ReadJSON(&chunk))
	assert.Equal(t, MessageTypeChunk, chunk.Type)
	assert.Equal(t, "first", chunk.Content)

	// Read done — proves deadline was cleared between writes
	var done ServerMessage
	require.NoError(t, ws.ReadJSON(&done))
	assert.Equal(t, MessageTypeDone, done.Type)
	assert.Equal(t, "second", done.Content)
}

func TestWriteDeadlineResetOnBinaryFrame(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			mediaID := testMediaIDFromString("audio-01")
			payload := []byte("frame-one")
			// Send two binary frames in sequence
			if err := writer.WriteBinaryMediaChunk(mediaID, 0, false, "audio/mp3", payload); err != nil {
				return err
			}
			return writer.WriteBinaryMediaChunk(mediaID, 1, true, "audio/mp3", []byte("frame-two"))
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "go"}))

	// Read connected (JSON)
	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))
	assert.Equal(t, MessageTypeConnected, connMsg.Type)

	// Read first binary frame
	msgType, data, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, msgType)
	frame1, err := DecodeBinaryFrame(data)
	require.NoError(t, err)
	assert.Equal(t, []byte("frame-one"), frame1.Payload)
	assert.False(t, frame1.Header.Flags.IsLast())

	// Read second binary frame — proves deadline was cleared
	msgType, data, err = ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, msgType)
	frame2, err := DecodeBinaryFrame(data)
	require.NoError(t, err)
	assert.Equal(t, []byte("frame-two"), frame2.Payload)
	assert.True(t, frame2.Header.Flags.IsLast())
}

func TestChunkedMediaStreamingLargePayload(t *testing.T) {
	// Create a payload larger than ChunkThreshold (1 MB)
	payloadSize := ChunkThreshold + MaxChunkSize/2 // 1 MB + 32 KB
	largePayload := make([]byte, payloadSize)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			mediaID := testMediaIDFromString("big-media")
			return writer.WriteBinaryMediaChunk(mediaID, 0, true, "video/mp4", largePayload)
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "stream"}))

	// Read connected
	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))
	assert.Equal(t, MessageTypeConnected, connMsg.Type)

	// Calculate expected chunk count
	expectedChunks := (payloadSize + MaxChunkSize - 1) / MaxChunkSize

	// Read all chunked binary frames
	var reassembled []byte
	for i := 0; i < expectedChunks; i++ {
		msgType, data, err := ws.ReadMessage()
		require.NoError(t, err, "chunk %d", i)
		assert.Equal(t, websocket.BinaryMessage, msgType, "chunk %d", i)

		frame, err := DecodeBinaryFrame(data)
		require.NoError(t, err, "chunk %d", i)

		// Verify chunked flag is set
		assert.True(t, frame.Header.Flags.IsChunked(), "chunk %d should have FlagChunked", i)
		assert.Equal(t, uint32(i), frame.Header.Sequence, "chunk %d sequence", i)

		// Verify metadata contains total_chunks
		var meta BinaryMediaChunkMetadata
		require.NoError(t, json.Unmarshal(frame.Metadata, &meta), "chunk %d metadata", i)
		assert.Equal(t, uint32(expectedChunks), meta.TotalChunks, "chunk %d total_chunks", i)
		assert.Equal(t, "video/mp4", meta.MimeType, "chunk %d mime_type", i)

		// Only the last chunk should have IsLast
		if i == expectedChunks-1 {
			assert.True(t, frame.Header.Flags.IsLast(), "last chunk should have FlagIsLast")
		} else {
			assert.False(t, frame.Header.Flags.IsLast(), "non-last chunk %d should not have FlagIsLast", i)
		}

		reassembled = append(reassembled, frame.Payload...)
	}

	// Verify reassembled payload matches original
	assert.Equal(t, largePayload, reassembled)
}

func TestSmallPayloadNotChunked(t *testing.T) {
	// Payload under ChunkThreshold — should be sent as a single frame
	smallPayload := make([]byte, ChunkThreshold-1)
	for i := range smallPayload {
		smallPayload[i] = byte(i % 256)
	}

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			mediaID := testMediaIDFromString("small")
			return writer.WriteBinaryMediaChunk(mediaID, 42, true, "image/png", smallPayload)
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "go"}))

	// Read connected
	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))

	// Read single binary frame — should NOT be chunked
	msgType, data, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, msgType)

	frame, err := DecodeBinaryFrame(data)
	require.NoError(t, err)

	assert.False(t, frame.Header.Flags.IsChunked(), "small payload should not be chunked")
	assert.True(t, frame.Header.Flags.IsLast())
	assert.Equal(t, uint32(42), frame.Header.Sequence, "original sequence preserved")
	assert.Equal(t, smallPayload, frame.Payload)
}

func TestExactThresholdPayloadNotChunked(t *testing.T) {
	// Payload exactly at ChunkThreshold — should still be sent as single frame
	payload := make([]byte, ChunkThreshold)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			mediaID := testMediaIDFromString("exact")
			return writer.WriteBinaryMediaChunk(mediaID, 0, true, "image/png", payload)
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "go"}))

	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))

	msgType, data, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, msgType)

	frame, err := DecodeBinaryFrame(data)
	require.NoError(t, err)
	assert.False(t, frame.Header.Flags.IsChunked(), "threshold-size payload should not be chunked")
	assert.True(t, frame.Header.Flags.IsLast())
}

func TestNewChunkedMediaFrame(t *testing.T) {
	mediaID := testMediaIDFromString("chunk-test")
	payload := []byte("chunk payload data")

	t.Run("middle chunk", func(t *testing.T) {
		frame, err := NewChunkedMediaFrame("session-1", mediaID, 2, 10, false, "audio/wav", payload)
		require.NoError(t, err)

		assert.Equal(t, [4]byte{'O', 'M', 'N', 'I'}, frame.Header.Magic)
		assert.Equal(t, uint8(BinaryVersion), frame.Header.Version)
		assert.True(t, frame.Header.Flags.IsChunked())
		assert.False(t, frame.Header.Flags.IsLast())
		assert.Equal(t, BinaryMessageTypeMediaChunk, frame.Header.MessageType)
		assert.Equal(t, uint32(2), frame.Header.Sequence)
		assert.Equal(t, mediaID, frame.Header.MediaID)
		assert.Equal(t, payload, frame.Payload)

		var meta BinaryMediaChunkMetadata
		require.NoError(t, json.Unmarshal(frame.Metadata, &meta))
		assert.Equal(t, "session-1", meta.SessionID)
		assert.Equal(t, "audio/wav", meta.MimeType)
		assert.Equal(t, uint32(10), meta.TotalChunks)
	})

	t.Run("last chunk", func(t *testing.T) {
		frame, err := NewChunkedMediaFrame("session-1", mediaID, 9, 10, true, "audio/wav", payload)
		require.NoError(t, err)

		assert.True(t, frame.Header.Flags.IsChunked())
		assert.True(t, frame.Header.Flags.IsLast())
		assert.Equal(t, uint32(9), frame.Header.Sequence)
	})
}

func TestChunkedMediaFallbackToJSON(t *testing.T) {
	// Large payload without binary support should fallback to JSON (no chunking)
	largePayload := make([]byte, ChunkThreshold+1)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			mediaID := testMediaIDFromString("fallback")
			return writer.WriteBinaryMediaChunk(mediaID, 0, true, "video/mp4", largePayload)
		},
	}

	_, ts := newTestServer(t, handler)

	// Connect WITHOUT binary=true
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	require.NoError(t, ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "go"}))

	// Read connected
	var connMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&connMsg))

	// Should receive a single JSON media_chunk message (base64 fallback)
	var mediaMsg ServerMessage
	require.NoError(t, ws.ReadJSON(&mediaMsg))
	assert.Equal(t, MessageTypeMediaChunk, mediaMsg.Type)
	assert.NotNil(t, mediaMsg.MediaChunk)
	assert.True(t, mediaMsg.MediaChunk.IsLast)
}
