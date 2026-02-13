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
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// UsageInfo carries token/cost data from the runtime Done message.
type UsageInfo struct {
	InputTokens  int32
	OutputTokens int32
	CostUSD      float64
}

// UsageReporter allows setting usage data on a writer.
type UsageReporter interface {
	ReportUsage(usage *UsageInfo)
}

// recordingResponseWriter wraps a ResponseWriter and asynchronously records
// messages to the session store. It delegates all calls to the inner writer
// first (so client latency is unaffected), then fires off goroutines to
// persist data. Store errors are logged but never propagated.
type recordingResponseWriter struct {
	inner     ResponseWriter
	store     session.Store
	sessionID string
	log       logr.Logger
	startTime time.Time
	usage     *UsageInfo
}

// newRecordingWriter creates a recordingResponseWriter that wraps inner.
func newRecordingWriter(inner ResponseWriter, store session.Store, sessionID string, log logr.Logger) *recordingResponseWriter {
	return &recordingResponseWriter{
		inner:     inner,
		store:     store,
		sessionID: sessionID,
		log:       log.WithName("recording-writer"),
		startTime: time.Now(),
	}
}

// ReportUsage stores usage info for the next WriteDone call.
func (w *recordingResponseWriter) ReportUsage(usage *UsageInfo) {
	w.usage = usage
}

// WriteChunk delegates to inner without recording (chunks are intermediate).
func (w *recordingResponseWriter) WriteChunk(content string) error {
	return w.inner.WriteChunk(content)
}

// WriteChunkWithParts delegates to inner without recording.
func (w *recordingResponseWriter) WriteChunkWithParts(parts []ContentPart) error {
	return w.inner.WriteChunkWithParts(parts)
}

// WriteDone delegates to inner, then async-records the assistant message.
func (w *recordingResponseWriter) WriteDone(content string) error {
	err := w.inner.WriteDone(content)
	w.recordDone(content)
	return err
}

// WriteDoneWithParts delegates to inner, then async-records the assistant message.
func (w *recordingResponseWriter) WriteDoneWithParts(parts []ContentPart) error {
	err := w.inner.WriteDoneWithParts(parts)
	// Extract text content from parts for recording
	text := extractTextFromParts(parts)
	w.recordDone(text)
	return err
}

// WriteToolCall delegates to inner, then async-records the tool call.
func (w *recordingResponseWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	err := w.inner.WriteToolCall(toolCall)

	go func() {
		content, marshalErr := json.Marshal(map[string]interface{}{
			"name":      toolCall.Name,
			"arguments": toolCall.Arguments,
		})
		if marshalErr != nil {
			w.log.Error(marshalErr, "failed to marshal tool call")
			return
		}

		msg := session.Message{
			ID:         uuid.New().String(),
			Role:       session.RoleAssistant,
			Content:    string(content),
			ToolCallID: toolCall.ID,
			Timestamp:  time.Now(),
			Metadata: map[string]string{
				"type": "tool_call",
			},
		}
		if storeErr := w.store.AppendMessage(context.Background(), w.sessionID, msg); storeErr != nil {
			w.log.Error(storeErr, "failed to record tool call")
		}

		if storeErr := w.store.UpdateSessionStats(context.Background(), w.sessionID, session.SessionStatsUpdate{
			AddToolCalls: 1,
			AddMessages:  1,
		}); storeErr != nil {
			w.log.Error(storeErr, "failed to update session stats for tool call")
		}
	}()

	return err
}

// WriteToolResult delegates to inner, then async-records the tool result.
func (w *recordingResponseWriter) WriteToolResult(result *ToolResultInfo) error {
	err := w.inner.WriteToolResult(result)

	go func() {
		var content string
		if result.Error != "" {
			content = result.Error
		} else {
			data, marshalErr := json.Marshal(result.Result)
			if marshalErr != nil {
				w.log.Error(marshalErr, "failed to marshal tool result")
				content = fmt.Sprintf("%v", result.Result)
			} else {
				content = string(data)
			}
		}

		metadata := map[string]string{
			"type": "tool_result",
		}
		if result.Error != "" {
			metadata["is_error"] = "true"
		}

		msg := session.Message{
			ID:         uuid.New().String(),
			Role:       session.RoleSystem,
			Content:    content,
			ToolCallID: result.ID,
			Timestamp:  time.Now(),
			Metadata:   metadata,
		}
		if storeErr := w.store.AppendMessage(context.Background(), w.sessionID, msg); storeErr != nil {
			w.log.Error(storeErr, "failed to record tool result")
		}

		if storeErr := w.store.UpdateSessionStats(context.Background(), w.sessionID, session.SessionStatsUpdate{
			AddMessages: 1,
		}); storeErr != nil {
			w.log.Error(storeErr, "failed to update session stats for tool result")
		}
	}()

	return err
}

// WriteError delegates to inner, then async-records the error.
func (w *recordingResponseWriter) WriteError(code, message string) error {
	err := w.inner.WriteError(code, message)

	go func() {
		msg := session.Message{
			ID:        uuid.New().String(),
			Role:      session.RoleSystem,
			Content:   fmt.Sprintf("%s: %s", code, message),
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"type": "error",
			},
		}
		if storeErr := w.store.AppendMessage(context.Background(), w.sessionID, msg); storeErr != nil {
			w.log.Error(storeErr, "failed to record error message")
		}

		if storeErr := w.store.UpdateSessionStats(context.Background(), w.sessionID, session.SessionStatsUpdate{
			AddMessages: 1,
			SetStatus:   session.SessionStatusError,
		}); storeErr != nil {
			w.log.Error(storeErr, "failed to update session stats for error")
		}
	}()

	return err
}

// WriteUploadReady delegates to inner without recording.
func (w *recordingResponseWriter) WriteUploadReady(uploadReady *UploadReadyInfo) error {
	return w.inner.WriteUploadReady(uploadReady)
}

// WriteUploadComplete delegates to inner without recording.
func (w *recordingResponseWriter) WriteUploadComplete(uploadComplete *UploadCompleteInfo) error {
	return w.inner.WriteUploadComplete(uploadComplete)
}

// WriteMediaChunk delegates to inner without recording.
func (w *recordingResponseWriter) WriteMediaChunk(mediaChunk *MediaChunkInfo) error {
	return w.inner.WriteMediaChunk(mediaChunk)
}

// WriteBinaryMediaChunk delegates to inner without recording.
func (w *recordingResponseWriter) WriteBinaryMediaChunk(mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) error {
	return w.inner.WriteBinaryMediaChunk(mediaID, sequence, isLast, mimeType, payload)
}

// SupportsBinary delegates to inner.
func (w *recordingResponseWriter) SupportsBinary() bool {
	return w.inner.SupportsBinary()
}

// recordDone records the final assistant message with usage and latency metadata.
func (w *recordingResponseWriter) recordDone(content string) {
	// Capture values before goroutine
	usage := w.usage
	latencyMs := time.Since(w.startTime).Milliseconds()

	go func() {
		metadata := map[string]string{
			"latency_ms": strconv.FormatInt(latencyMs, 10),
		}

		var inputTokens, outputTokens int32
		if usage != nil {
			inputTokens = usage.InputTokens
			outputTokens = usage.OutputTokens
			metadata["cost_usd"] = strconv.FormatFloat(usage.CostUSD, 'f', -1, 64)
		}

		msg := session.Message{
			ID:           uuid.New().String(),
			Role:         session.RoleAssistant,
			Content:      content,
			Timestamp:    time.Now(),
			Metadata:     metadata,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}
		if storeErr := w.store.AppendMessage(context.Background(), w.sessionID, msg); storeErr != nil {
			w.log.Error(storeErr, "failed to record assistant message")
		}

		statsUpdate := session.SessionStatsUpdate{
			AddMessages: 1,
		}
		if usage != nil {
			statsUpdate.AddInputTokens = usage.InputTokens
			statsUpdate.AddOutputTokens = usage.OutputTokens
			statsUpdate.AddCostUSD = usage.CostUSD
		}
		if storeErr := w.store.UpdateSessionStats(context.Background(), w.sessionID, statsUpdate); storeErr != nil {
			w.log.Error(storeErr, "failed to update session stats for done")
		}
	}()
}

// extractTextFromParts extracts text content from ContentParts.
func extractTextFromParts(parts []ContentPart) string {
	for _, part := range parts {
		if part.Type == ContentPartTypeText && part.Text != "" {
			return part.Text
		}
	}
	return ""
}

// Verify interface compliance at compile time.
var _ ResponseWriter = (*recordingResponseWriter)(nil)
var _ UsageReporter = (*recordingResponseWriter)(nil)
