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

package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// Metadata key constants to avoid string duplication (SonarCloud go:S1192).
const (
	metaKeyType       = "type"
	metaKeySource     = "source"
	metaKeyToolName   = "tool_name"
	metaKeyDurationMs = "duration_ms"

	// writeTimeout bounds how long a single writeMessage call can take.
	writeTimeout   = 30 * time.Second
	metaKeyIsError = "is_error"

	metaValueSource = "runtime"
)

// asPtr extracts event data as a pointer, handling both value and pointer types.
// The PromptKit emitter may pass either T or *T depending on the event method.
func asPtr[T any](data any) (*T, bool) {
	if p, ok := data.(*T); ok {
		return p, true
	}
	if v, ok := data.(T); ok {
		return &v, true
	}
	return nil, false
}

// OmniaEventStore implements PromptKit's events.EventStore interface by
// bridging events to Omnia's session-api via session.Store.AppendMessage().
// This is the Pattern C integration point: the SDK's EventBus publishes
// events here, and we persist them as session messages.
//
// Every event the SDK emits is recorded — matching the fidelity of
// PromptKit's FileEventStore.

// defaultWriteConcurrency is the maximum number of concurrent event store writes.
const defaultWriteConcurrency = 100

type OmniaEventStore struct {
	sessionStore session.Store
	log          logr.Logger
	toolMetaFn   func(string) (tools.ToolMeta, bool)
	sem          chan struct{} // bounded concurrency for async writes
	sessionID    string        // fallback sessionID for events missing it (PromptKit bug workaround)
}

// NewOmniaEventStore creates a new event store that bridges to session-api.
// Write concurrency is bounded by a semaphore to prevent unbounded goroutine spawning.
func NewOmniaEventStore(store session.Store, log logr.Logger) *OmniaEventStore {
	return &OmniaEventStore{
		sessionStore: store,
		log:          log.WithName("event-store"),
		sem:          make(chan struct{}, defaultWriteConcurrency),
	}
}

// SetSessionID sets the fallback session ID used when events arrive with an
// empty SessionID. This works around a PromptKit bug where the eval middleware
// emitter is created without a session ID (see PromptKit#705).
func (s *OmniaEventStore) SetSessionID(id string) {
	s.sessionID = id
}

// SetToolMetaFn sets the function used to look up registry/handler metadata for tools.
func (s *OmniaEventStore) SetToolMetaFn(fn func(string) (tools.ToolMeta, bool)) {
	s.toolMetaFn = fn
}

// Append adds an event to the store by converting it to a session message
// and writing it to session-api. Writes are fire-and-forget (goroutines with
// logged errors), matching the facade's async recording pattern.
func (s *OmniaEventStore) Append(ctx context.Context, event *events.Event) error {
	// Backfill empty SessionID from the fallback — works around PromptKit#705
	// where the eval middleware emitter is created without a session ID.
	if event.SessionID == "" {
		if s.sessionID != "" {
			event.SessionID = s.sessionID
		} else {
			return nil
		}
	}

	msg, stats, ok := s.convertEvent(event)
	if !ok {
		return nil
	}

	// Carry span context into the goroutine so session-api calls are
	// children of the conversation turn span, but detach cancellation
	// so the write is not aborted when the caller's context expires.
	traceCtx := detachedTraceContext(ctx)

	s.sem <- struct{}{}
	go func() {
		defer func() { <-s.sem }()
		s.writeMessage(traceCtx, event.SessionID, msg, stats)
	}()
	return nil
}

// Query is a no-op — OmniaEventStore is write-only (session-api is the read path).
func (s *OmniaEventStore) Query(_ context.Context, _ *events.EventFilter) ([]*events.Event, error) {
	return nil, nil
}

// QueryRaw is a no-op — OmniaEventStore is write-only.
func (s *OmniaEventStore) QueryRaw(_ context.Context, _ *events.EventFilter) ([]*events.StoredEvent, error) {
	return nil, nil
}

// Stream is a no-op — OmniaEventStore is write-only.
func (s *OmniaEventStore) Stream(_ context.Context, _ string) (<-chan *events.Event, error) {
	ch := make(chan *events.Event)
	close(ch)
	return ch, nil
}

// Close is a no-op — the session store lifecycle is managed externally.
func (s *OmniaEventStore) Close() error {
	return nil
}

// convertEvent maps a PromptKit event to a session.Message and stats update.
func (s *OmniaEventStore) convertEvent(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	switch event.Type {
	// Message lifecycle
	case events.EventMessageCreated:
		return s.convertMessageCreated(event)
	case events.EventMessageUpdated:
		return s.convertMessageUpdated(event)
	case events.EventConversationStarted:
		return s.convertConversationStarted(event)

	// Tool calls
	case events.EventToolCallStarted:
		return s.convertToolCallStarted(event)
	case events.EventToolCallCompleted:
		return s.convertToolCallCompleted(event)
	case events.EventToolCallFailed:
		return s.convertToolCallFailed(event)

	// Provider calls
	case events.EventProviderCallStarted:
		return s.convertProviderCallStarted(event)
	case events.EventProviderCallCompleted:
		return s.convertProviderCallCompleted(event)
	case events.EventProviderCallFailed:
		return s.convertProviderCallFailed(event)

	// Evals
	case events.EventEvalCompleted, events.EventEvalFailed:
		return s.convertEvalEvent(event)

	// Pipeline lifecycle
	case events.EventPipelineStarted,
		events.EventPipelineCompleted,
		events.EventPipelineFailed:
		return s.convertGenericEvent(event)

	// Stage lifecycle
	case events.EventStageStarted,
		events.EventStageCompleted,
		events.EventStageFailed:
		return s.convertGenericEvent(event)

	// Middleware lifecycle
	case events.EventMiddlewareStarted,
		events.EventMiddlewareCompleted,
		events.EventMiddlewareFailed:
		return s.convertGenericEvent(event)

	// Validation
	case events.EventValidationStarted,
		events.EventValidationPassed,
		events.EventValidationFailed:
		return s.convertGenericEvent(event)

	// Context/state
	case events.EventContextBuilt,
		events.EventTokenBudgetExceeded,
		events.EventStateLoaded,
		events.EventStateSaved,
		events.EventStreamInterrupted:
		return s.convertGenericEvent(event)

	// Workflow
	case events.EventWorkflowTransitioned,
		events.EventWorkflowCompleted:
		return s.convertGenericEvent(event)

	default:
		// Record unknown event types too — full fidelity
		return s.convertGenericEvent(event)
	}
}

// --- Message events ---

// convertMessageCreated creates a session message from EventMessageCreated.
// Records ALL roles (user, assistant, tool, system) with full content
// including embedded tool calls and tool results.
func (s *OmniaEventStore) convertMessageCreated(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.MessageCreatedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	role := session.MessageRole(data.Role)
	content := data.Content

	metadata := map[string]string{
		metaKeySource: metaValueSource,
		"index":       strconv.Itoa(data.Index),
	}

	// Enrich with tool call data if present on assistant messages
	if len(data.ToolCalls) > 0 {
		metadata[metaKeyType] = "tool_call"
		toolCallJSON, err := json.Marshal(data.ToolCalls)
		if err == nil {
			metadata["tool_calls"] = string(toolCallJSON)
		}
		// Use the first tool call ID for linking
		if data.ToolCalls[0].ID != "" {
			msg := s.buildMessage(role, content, event.Timestamp, metadata)
			msg.ToolCallID = data.ToolCalls[0].ID
			return msg, session.SessionStatsUpdate{AddMessages: 1, AddToolCalls: int32(len(data.ToolCalls))}, true
		}
	}

	// Enrich with tool result data if present on tool messages
	if data.ToolResult != nil {
		metadata[metaKeyType] = "tool_result"
		metadata[metaKeyToolName] = data.ToolResult.Name
		if data.ToolResult.Error != "" {
			metadata[metaKeyIsError] = "true"
			content = data.ToolResult.Error
		} else {
			content = textFromParts(data.ToolResult.Parts)
		}
		if data.ToolResult.LatencyMs > 0 {
			metadata[metaKeyDurationMs] = strconv.FormatInt(data.ToolResult.LatencyMs, 10)
		}
		msg := s.buildMessage(role, content, event.Timestamp, metadata)
		msg.ToolCallID = data.ToolResult.ID
		return msg, session.SessionStatsUpdate{}, true
	}

	// Enrich with multimodal content metadata (not the blob data itself)
	if len(data.Parts) > 0 {
		partsMeta := extractPartsMetadata(data.Parts)
		if len(partsMeta) > 0 {
			partsJSON, err := json.Marshal(partsMeta)
			if err == nil {
				metadata["parts"] = string(partsJSON)
				metadata["multimodal"] = "true"
				metadata["part_count"] = strconv.Itoa(len(data.Parts))
			}
		}
	}

	msg := s.buildMessage(role, content, event.Timestamp, metadata)
	return msg, session.SessionStatsUpdate{AddMessages: 1}, true
}

// partMetadata holds metadata about a content part without the actual blob data.
type partMetadata struct {
	Type     string `json:"type"`                // text, image, audio, video, document
	MIMEType string `json:"mime_type,omitempty"` // e.g., "image/jpeg"
	SizeKB   *int64 `json:"size_kb,omitempty"`   // Size in kilobytes
	Format   string `json:"format,omitempty"`    // Format hint (e.g., "png")
	Width    *int   `json:"width,omitempty"`     // Image/video width in pixels
	Height   *int   `json:"height,omitempty"`    // Image/video height in pixels
	Duration *int   `json:"duration,omitempty"`  // Audio/video duration in seconds
	Channels *int   `json:"channels,omitempty"`  // Audio channels
	BitRate  *int   `json:"bit_rate,omitempty"`  // Audio/video bit rate in kbps
	FPS      *int   `json:"fps,omitempty"`       // Video frames per second
	Caption  string `json:"caption,omitempty"`   // Optional caption
	Detail   string `json:"detail,omitempty"`    // Vision model detail level
	HasData  bool   `json:"has_data,omitempty"`  // Whether blob data was present (but stripped)
}

// textFromParts returns concatenated text content from content parts.
func textFromParts(parts []types.ContentPart) string {
	var s string
	for _, p := range parts {
		if p.Type == types.ContentTypeText && p.Text != nil {
			s += *p.Text
		}
	}
	return s
}

// extractPartsMetadata extracts metadata from content parts, stripping blob data.
// Only media parts produce metadata entries; text parts are already in the Content field.
func extractPartsMetadata(parts []types.ContentPart) []partMetadata {
	var metas []partMetadata
	for _, part := range parts {
		if part.Media == nil {
			continue // Text parts — content is already in the message Content field
		}
		meta := partMetadata{
			Type:     part.Type,
			MIMEType: part.Media.MIMEType,
			SizeKB:   part.Media.SizeKB,
			Width:    part.Media.Width,
			Height:   part.Media.Height,
			Duration: part.Media.Duration,
			Channels: part.Media.Channels,
			BitRate:  part.Media.BitRate,
			FPS:      part.Media.FPS,
		}
		if part.Media.Format != nil {
			meta.Format = *part.Media.Format
		}
		if part.Media.Caption != nil {
			meta.Caption = *part.Media.Caption
		}
		if part.Media.Detail != nil {
			meta.Detail = *part.Media.Detail
		}
		// Record whether data was present (but don't store it)
		meta.HasData = (part.Media.Data != nil) || (part.Media.FilePath != nil) || (part.Media.URL != nil) || (part.Media.StorageReference != nil)
		metas = append(metas, meta)
	}
	return metas
}

// convertMessageUpdated records token/cost/latency updates for a message.
func (s *OmniaEventStore) convertMessageUpdated(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.MessageUpdatedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	metadata := map[string]string{
		metaKeyType:   "message_updated",
		metaKeySource: metaValueSource,
		"index":       strconv.Itoa(data.Index),
		"latency_ms":  strconv.FormatInt(data.LatencyMs, 10),
	}

	content, _ := json.Marshal(map[string]interface{}{
		"index":        data.Index,
		"latencyMs":    data.LatencyMs,
		"inputTokens":  data.InputTokens,
		"outputTokens": data.OutputTokens,
		"totalCost":    data.TotalCost,
	})

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, metadata)
	msg.InputTokens = int32(data.InputTokens)
	msg.OutputTokens = int32(data.OutputTokens)

	stats := session.SessionStatsUpdate{
		AddInputTokens:  int32(data.InputTokens),
		AddOutputTokens: int32(data.OutputTokens),
		AddCostUSD:      data.TotalCost,
	}

	return msg, stats, true
}

// convertConversationStarted records the system prompt.
func (s *OmniaEventStore) convertConversationStarted(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ConversationStartedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	metadata := map[string]string{
		metaKeyType:   "conversation_started",
		metaKeySource: metaValueSource,
	}

	msg := s.buildMessage(session.RoleSystem, data.SystemPrompt, event.Timestamp, metadata)
	return msg, session.SessionStatsUpdate{}, true
}

// Metadata key constants for tool registry enrichment.
const (
	metaKeyHandlerName       = "handler_name"
	metaKeyHandlerType       = "handler_type"
	metaKeyRegistryName      = "registry_name"
	metaKeyRegistryNamespace = "registry_namespace"
)

// enrichToolMeta adds registry/handler metadata to the map if available.
func (s *OmniaEventStore) enrichToolMeta(metadata map[string]string, toolName string) {
	if s.toolMetaFn == nil {
		return
	}
	meta, ok := s.toolMetaFn(toolName)
	if !ok {
		return
	}
	metadata[metaKeyHandlerName] = meta.HandlerName
	metadata[metaKeyHandlerType] = meta.HandlerType
	metadata[metaKeyRegistryName] = meta.RegistryName
	metadata[metaKeyRegistryNamespace] = meta.RegistryNamespace
}

// --- Tool call events ---

// convertToolCallStarted creates a tool_call message matching the facade format.
func (s *OmniaEventStore) convertToolCallStarted(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ToolCallStartedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	content, err := json.Marshal(map[string]interface{}{
		"name":      data.ToolName,
		"arguments": data.Args,
	})
	if err != nil {
		s.log.Error(err, "failed to marshal tool call")
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	metadata := map[string]string{
		metaKeyType:   "tool_call",
		metaKeySource: metaValueSource,
	}
	s.enrichToolMeta(metadata, data.ToolName)

	msg := s.buildMessage(session.RoleAssistant, string(content), event.Timestamp, metadata)
	msg.ToolCallID = data.CallID

	return msg, session.SessionStatsUpdate{AddToolCalls: 1}, true
}

// convertToolCallCompleted creates a tool_result status message.
func (s *OmniaEventStore) convertToolCallCompleted(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ToolCallCompletedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	// Extract tool result content from Parts (text parts survive MetadataOnlyParts).
	resultBody := textFromParts(data.Parts)

	payload := map[string]interface{}{
		"toolName":   data.ToolName,
		"callID":     data.CallID,
		"status":     data.Status,
		"durationMs": data.Duration.Milliseconds(),
	}
	if resultBody != "" {
		payload["result"] = resultBody
	}
	content, _ := json.Marshal(payload)

	metadata := map[string]string{
		metaKeyType:       "tool_call_completed",
		metaKeySource:     metaValueSource,
		metaKeyToolName:   data.ToolName,
		metaKeyDurationMs: strconv.FormatInt(data.Duration.Milliseconds(), 10),
		"status":          data.Status,
	}
	s.enrichToolMeta(metadata, data.ToolName)

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, metadata)
	msg.ToolCallID = data.CallID

	return msg, session.SessionStatsUpdate{}, true
}

// convertToolCallFailed creates a tool_result message with is_error metadata.
func (s *OmniaEventStore) convertToolCallFailed(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ToolCallFailedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	errMsg := "unknown error"
	if data.Error != nil {
		errMsg = data.Error.Error()
	}

	metadata := map[string]string{
		metaKeyType:       "tool_result",
		metaKeyIsError:    "true",
		metaKeySource:     metaValueSource,
		metaKeyToolName:   data.ToolName,
		metaKeyDurationMs: strconv.FormatInt(data.Duration.Milliseconds(), 10),
	}
	s.enrichToolMeta(metadata, data.ToolName)

	msg := s.buildMessage(session.RoleSystem, errMsg, event.Timestamp, metadata)
	msg.ToolCallID = data.CallID

	return msg, session.SessionStatsUpdate{}, true
}

// --- Provider call events ---

// convertProviderCallStarted records the start of a provider call.
func (s *OmniaEventStore) convertProviderCallStarted(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ProviderCallStartedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	content, _ := json.Marshal(map[string]interface{}{
		"provider":     data.Provider,
		"model":        data.Model,
		"messageCount": data.MessageCount,
		"toolCount":    data.ToolCount,
	})

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, map[string]string{
		metaKeyType:   "provider_call_started",
		metaKeySource: metaValueSource,
		"provider":    data.Provider,
		"model":       data.Model,
	})

	return msg, session.SessionStatsUpdate{}, true
}

// convertProviderCallCompleted records provider call completion with tokens/cost.
func (s *OmniaEventStore) convertProviderCallCompleted(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ProviderCallCompletedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	content, err := json.Marshal(map[string]interface{}{
		"provider":     data.Provider,
		"model":        data.Model,
		"inputTokens":  data.InputTokens,
		"outputTokens": data.OutputTokens,
		"cachedTokens": data.CachedTokens,
		"cost":         data.Cost,
		"finishReason": data.FinishReason,
		"durationMs":   data.Duration.Milliseconds(),
	})
	if err != nil {
		s.log.Error(err, "failed to marshal provider call data")
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, map[string]string{
		metaKeyType:   "provider_call",
		metaKeySource: metaValueSource,
	})
	msg.InputTokens = int32(data.InputTokens)
	msg.OutputTokens = int32(data.OutputTokens)

	stats := session.SessionStatsUpdate{
		AddInputTokens:  int32(data.InputTokens),
		AddOutputTokens: int32(data.OutputTokens),
		AddCostUSD:      data.Cost,
	}

	return msg, stats, true
}

// convertProviderCallFailed records a provider call failure.
func (s *OmniaEventStore) convertProviderCallFailed(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.ProviderCallFailedData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	errMsg := ""
	if data.Error != nil {
		errMsg = data.Error.Error()
	}

	content, _ := json.Marshal(map[string]interface{}{
		"provider":   data.Provider,
		"model":      data.Model,
		"error":      errMsg,
		"durationMs": data.Duration.Milliseconds(),
	})

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, map[string]string{
		metaKeyType:    "provider_call_failed",
		metaKeyIsError: "true",
		metaKeySource:  metaValueSource,
		"provider":     data.Provider,
		"model":        data.Model,
	})

	return msg, session.SessionStatsUpdate{}, true
}

// --- Eval events ---

// convertEvalEvent creates a session message from an eval completed/failed event.
func (s *OmniaEventStore) convertEvalEvent(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	data, ok := asPtr[events.EvalEventData](event.Data)
	if !ok {
		return session.Message{}, session.SessionStatsUpdate{}, false
	}

	evtType := "eval_completed"
	if event.Type == events.EventEvalFailed {
		evtType = "eval_failed"
	}

	metadata := map[string]string{
		metaKeyType:   evtType,
		metaKeySource: metaValueSource,
		"eval_id":     data.EvalID,
		"eval_type":   data.EvalType,
		"trigger":     data.Trigger,
		"passed":      strconv.FormatBool(data.Passed),
	}
	if data.DurationMs > 0 {
		metadata[metaKeyDurationMs] = strconv.FormatInt(data.DurationMs, 10)
	}
	if data.Error != "" {
		metadata[metaKeyIsError] = "true"
	}
	if data.Skipped {
		metadata["skipped"] = "true"
		metadata["skip_reason"] = data.SkipReason
	}

	content, _ := json.Marshal(map[string]interface{}{
		"evalID":      data.EvalID,
		"evalType":    data.EvalType,
		"trigger":     data.Trigger,
		"passed":      data.Passed,
		"score":       data.Score,
		"durationMs":  data.DurationMs,
		"explanation": data.Explanation,
		"message":     data.Message,
		"violations":  data.Violations,
		"skipped":     data.Skipped,
		"skipReason":  data.SkipReason,
		"error":       data.Error,
	})

	msg := s.buildMessage(session.RoleSystem, string(content), event.Timestamp, metadata)
	return msg, session.SessionStatsUpdate{}, true
}

// --- Generic event handler ---

// convertGenericEvent records any event type by serializing its Data as JSON.
// This ensures full recording fidelity — no events are silently dropped.
func (s *OmniaEventStore) convertGenericEvent(event *events.Event) (session.Message, session.SessionStatsUpdate, bool) {
	content := "{}"
	if event.Data != nil {
		if data, err := json.Marshal(event.Data); err == nil {
			content = string(data)
		}
	}

	msg := s.buildMessage(session.RoleSystem, content, event.Timestamp, map[string]string{
		metaKeyType:   string(event.Type),
		metaKeySource: metaValueSource,
	})

	return msg, session.SessionStatsUpdate{}, true
}

// --- Helpers ---

// buildMessage creates a session.Message with common fields.
func (s *OmniaEventStore) buildMessage(
	role session.MessageRole,
	content string,
	ts time.Time,
	metadata map[string]string,
) session.Message {
	return session.Message{
		ID:        uuid.New().String(),
		Role:      role,
		Content:   content,
		Timestamp: ts,
		Metadata:  metadata,
	}
}

// writeMessage persists a message and stats update to session-api.
// Errors are logged but never propagated, matching the facade's fire-and-forget pattern.
// When sessionStore is nil (eval-metrics-only mode), this is a no-op.
// The traceCtx carries span context for trace propagation without cancellation.
func (s *OmniaEventStore) writeMessage(traceCtx context.Context, sessionID string, msg session.Message, stats session.SessionStatsUpdate) {
	if s.sessionStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(traceCtx, writeTimeout)
	defer cancel()
	log := logctx.LoggerWithContext(s.log, traceCtx)
	msgType := msg.Metadata[metaKeyType]

	log.V(1).Info("writing event to session-api",
		"sessionID", sessionID, "messageType", msgType, "messageID", msg.ID)

	if err := s.sessionStore.AppendMessage(ctx, sessionID, msg); err != nil {
		log.Error(err, "failed to append event message",
			"sessionID", sessionID,
			"messageType", msgType)
		return
	}

	if err := s.sessionStore.UpdateSessionStats(ctx, sessionID, stats); err != nil {
		log.Error(err, "failed to update session stats",
			"sessionID", sessionID,
			"messageType", msgType)
		return
	}

	log.V(1).Info("event written to session-api",
		"sessionID", sessionID, "messageType", msgType)
}

// detachedTraceContext returns a context that inherits all values (including
// span context and logctx trace_id) from ctx but does not inherit its
// cancellation or deadline. This is used for async fire-and-forget writes.
func detachedTraceContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// Verify interface compliance at compile time.
var _ events.EventStore = (*OmniaEventStore)(nil)
