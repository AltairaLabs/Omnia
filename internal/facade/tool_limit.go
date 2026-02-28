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
	"fmt"
	"sync/atomic"
)

// ErrorCodeToolCallLimitExceeded is sent when a session exceeds its tool call limit.
const ErrorCodeToolCallLimitExceeded = "TOOL_CALL_LIMIT_EXCEEDED"

// ToolCallLimiter tracks tool calls per session and enforces a maximum.
type ToolCallLimiter struct {
	count int32
	limit int32
}

// NewToolCallLimiter creates a limiter with the given max. If max <= 0, no limit is enforced.
func NewToolCallLimiter(max int32) *ToolCallLimiter {
	return &ToolCallLimiter{limit: max}
}

// Allow checks whether another tool call is permitted and increments the counter.
// Returns nil if allowed, or an error describing the limit breach.
func (l *ToolCallLimiter) Allow() error {
	if l == nil || l.limit <= 0 {
		return nil
	}
	newCount := atomic.AddInt32(&l.count, 1)
	if newCount > l.limit {
		return fmt.Errorf("tool call limit exceeded: %d/%d", newCount, l.limit)
	}
	return nil
}

// Count returns the current number of tool calls.
func (l *ToolCallLimiter) Count() int32 {
	if l == nil {
		return 0
	}
	return atomic.LoadInt32(&l.count)
}

// Limit returns the configured maximum.
func (l *ToolCallLimiter) Limit() int32 {
	if l == nil {
		return 0
	}
	return l.limit
}

// rateLimitedWriter wraps a ResponseWriter and enforces tool call limits.
type rateLimitedWriter struct {
	inner   ResponseWriter
	limiter *ToolCallLimiter
}

// newRateLimitedWriter creates a writer that checks the limiter before forwarding tool calls.
// If limiter is nil, all calls pass through unchanged.
func newRateLimitedWriter(inner ResponseWriter, limiter *ToolCallLimiter) ResponseWriter {
	if limiter == nil || limiter.limit <= 0 {
		return inner
	}
	return &rateLimitedWriter{inner: inner, limiter: limiter}
}

func (w *rateLimitedWriter) WriteChunk(content string) error {
	return w.inner.WriteChunk(content)
}

func (w *rateLimitedWriter) WriteChunkWithParts(parts []ContentPart) error {
	return w.inner.WriteChunkWithParts(parts)
}

func (w *rateLimitedWriter) WriteDone(content string) error {
	return w.inner.WriteDone(content)
}

func (w *rateLimitedWriter) WriteDoneWithParts(parts []ContentPart) error {
	return w.inner.WriteDoneWithParts(parts)
}

// WriteToolCall checks the limiter before forwarding. If the limit is exceeded,
// it sends an error message instead.
func (w *rateLimitedWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	if err := w.limiter.Allow(); err != nil {
		return w.inner.WriteError(ErrorCodeToolCallLimitExceeded, err.Error())
	}
	return w.inner.WriteToolCall(toolCall)
}

func (w *rateLimitedWriter) WriteToolResult(result *ToolResultInfo) error {
	return w.inner.WriteToolResult(result)
}

func (w *rateLimitedWriter) WriteError(code, message string) error {
	return w.inner.WriteError(code, message)
}

func (w *rateLimitedWriter) WriteUploadReady(uploadReady *UploadReadyInfo) error {
	return w.inner.WriteUploadReady(uploadReady)
}

func (w *rateLimitedWriter) WriteUploadComplete(uploadComplete *UploadCompleteInfo) error {
	return w.inner.WriteUploadComplete(uploadComplete)
}

func (w *rateLimitedWriter) WriteMediaChunk(mediaChunk *MediaChunkInfo) error {
	return w.inner.WriteMediaChunk(mediaChunk)
}

func (w *rateLimitedWriter) WriteBinaryMediaChunk(mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) error {
	return w.inner.WriteBinaryMediaChunk(mediaID, sequence, isLast, mimeType, payload)
}

func (w *rateLimitedWriter) SupportsBinary() bool {
	return w.inner.SupportsBinary()
}

// Verify interface compliance at compile time.
var _ ResponseWriter = (*rateLimitedWriter)(nil)
