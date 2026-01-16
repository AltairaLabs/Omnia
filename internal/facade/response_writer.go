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
	"encoding/base64"
)

// connResponseWriter implements ResponseWriter for a connection.
type connResponseWriter struct {
	conn      *Connection
	sessionID string
	server    *Server
}

// WriteChunk sends a chunk of the response.
func (w *connResponseWriter) WriteChunk(content string) error {
	return w.server.sendMessage(w.conn, NewChunkMessage(w.sessionID, content))
}

// WriteChunkWithParts sends a chunk with multi-modal content parts.
func (w *connResponseWriter) WriteChunkWithParts(parts []ContentPart) error {
	return w.server.sendMessage(w.conn, NewChunkMessageWithParts(w.sessionID, parts))
}

// WriteDone signals the response is complete.
func (w *connResponseWriter) WriteDone(content string) error {
	return w.server.sendMessage(w.conn, NewDoneMessage(w.sessionID, content))
}

// WriteDoneWithParts signals completion with multi-modal content parts.
func (w *connResponseWriter) WriteDoneWithParts(parts []ContentPart) error {
	return w.server.sendMessage(w.conn, NewDoneMessageWithParts(w.sessionID, parts))
}

// WriteToolCall notifies of a tool call.
func (w *connResponseWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	return w.server.sendMessage(w.conn, NewToolCallMessage(w.sessionID, toolCall))
}

// WriteToolResult sends a tool result.
func (w *connResponseWriter) WriteToolResult(result *ToolResultInfo) error {
	return w.server.sendMessage(w.conn, NewToolResultMessage(w.sessionID, result))
}

// WriteError sends an error message.
func (w *connResponseWriter) WriteError(code, message string) error {
	return w.server.sendMessage(w.conn, NewErrorMessage(w.sessionID, code, message))
}

// WriteUploadReady sends upload URL information to the client.
func (w *connResponseWriter) WriteUploadReady(uploadReady *UploadReadyInfo) error {
	return w.server.sendMessage(w.conn, NewUploadReadyMessage(w.sessionID, uploadReady))
}

// WriteUploadComplete notifies the client that an upload is complete.
func (w *connResponseWriter) WriteUploadComplete(uploadComplete *UploadCompleteInfo) error {
	return w.server.sendMessage(w.conn, NewUploadCompleteMessage(w.sessionID, uploadComplete))
}

// WriteMediaChunk sends a streaming media chunk to the client.
func (w *connResponseWriter) WriteMediaChunk(mediaChunk *MediaChunkInfo) error {
	err := w.server.sendMessage(w.conn, NewMediaChunkMessage(w.sessionID, mediaChunk))
	if err == nil {
		w.server.metrics.MediaChunkSent(false, len(mediaChunk.Data))
	}
	return err
}

// SupportsBinary returns true if the client supports binary WebSocket frames.
func (w *connResponseWriter) SupportsBinary() bool {
	return w.conn.binaryCapable
}

// WriteBinaryMediaChunk sends a streaming media chunk as a binary frame.
// Falls back to base64 JSON if the client doesn't support binary frames.
func (w *connResponseWriter) WriteBinaryMediaChunk(mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) error {
	if !w.SupportsBinary() {
		// Fallback to base64 JSON for clients that don't support binary
		return w.WriteMediaChunk(&MediaChunkInfo{
			MediaID:  MediaIDToString(mediaID),
			Sequence: int(sequence),
			IsLast:   isLast,
			Data:     base64.StdEncoding.EncodeToString(payload),
			MimeType: mimeType,
		})
	}

	frame, err := NewMediaChunkFrame(w.sessionID, mediaID, sequence, isLast, mimeType, payload)
	if err != nil {
		return err
	}

	err = w.server.sendBinaryFrame(w.conn, frame)
	if err == nil {
		w.server.metrics.MediaChunkSent(true, len(payload))
	}
	return err
}
