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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// eventAction holds the outputs of converting a single PromptKit event.
// For tool/provider events, only the first-class record is set (no legacy message).
// For runtime lifecycle events, the event field carries the data.
// For message events, the message field carries the data.
type eventAction struct {
	message      *session.Message
	toolCall     *session.ToolCall
	providerCall *session.ProviderCall
	event        *session.RuntimeEvent
	stats        session.SessionStatsUpdate
}

// callKey tracks a started call so that completed/failed events can upsert
// the same row in the tool_calls or provider_calls table.
type callKey struct {
	id        string
	createdAt time.Time
}

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
	sessionStore     session.Store
	log              logr.Logger
	toolMetaFn       func(string) (tools.ToolMeta, bool)
	sem              chan struct{} // bounded concurrency for async writes
	sessionID        string        // fallback sessionID for events missing it (PromptKit bug workaround)
	toolCallKeys     sync.Map      // callID → callKey (for tool call upsert correlation)
	providerCallKeys sync.Map      // callID → callKey (for provider call upsert correlation)
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

	action, ok := s.convertEvent(event)
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
		s.writeAction(traceCtx, event.SessionID, action)
	}()
	return nil
}

// OnEvent is a Listener-compatible method for wiring the store as a bus subscriber.
// Events without a SessionID are silently skipped.
func (s *OmniaEventStore) OnEvent(event *events.Event) {
	if event.SessionID == "" {
		if s.sessionID != "" {
			event.SessionID = s.sessionID
		} else {
			return
		}
	}
	_ = s.Append(context.Background(), event)
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

// convertEvent maps a PromptKit event to an eventAction containing the legacy
// message (backward compat) plus optional first-class tool/provider call records.
func (s *OmniaEventStore) convertEvent(event *events.Event) (eventAction, bool) {
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

	// Client tool calls
	case events.EventClientToolRequest:
		return s.convertClientToolRequest(event)
	// NOTE: EventClientToolResolved will be added when the published SDK includes it.

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
func (s *OmniaEventStore) convertMessageCreated(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.MessageCreatedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	role := session.MessageRole(data.Role)
	content := data.Content

	metadata := map[string]string{
		metaKeySource: metaValueSource,
		"index":       strconv.Itoa(data.Index),
	}

	// Tool calls on assistant messages are recorded via the first-class tool_calls table
	// (EventToolCallStarted events), so we only count them in stats here.
	if len(data.ToolCalls) > 0 {
		msg := s.buildMessage(role, content, event.Timestamp, metadata)
		return eventAction{
			message: &msg,
			stats:   session.SessionStatsUpdate{AddMessages: 1, AddToolCalls: int32(len(data.ToolCalls))},
		}, true
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
		return eventAction{message: &msg}, true
	}

	// Enrich with multimodal content metadata (not the blob data itself)
	var hasMedia bool
	var mediaTypes []string
	if len(data.Parts) > 0 {
		partsMeta := extractPartsMetadata(data.Parts)
		if len(partsMeta) > 0 {
			partsJSON, err := json.Marshal(partsMeta)
			if err == nil {
				metadata["parts"] = string(partsJSON)
				metadata["multimodal"] = "true"
				metadata["part_count"] = strconv.Itoa(len(data.Parts))
			}
			hasMedia = true
			mediaTypes = extractMediaTypes(partsMeta)
		}
	}

	msg := s.buildMessage(role, content, event.Timestamp, metadata)
	msg.HasMedia = hasMedia
	msg.MediaTypes = mediaTypes
	return eventAction{
		message: &msg,
		stats:   session.SessionStatsUpdate{AddMessages: 1},
	}, true
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

// extractMediaTypes returns distinct media types from part metadata.
func extractMediaTypes(metas []partMetadata) []string {
	seen := make(map[string]struct{})
	var types []string
	for _, m := range metas {
		if _, ok := seen[m.Type]; !ok {
			seen[m.Type] = struct{}{}
			types = append(types, m.Type)
		}
	}
	return types
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
func (s *OmniaEventStore) convertMessageUpdated(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.MessageUpdatedData](event.Data)
	if !ok {
		return eventAction{}, false
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

	return eventAction{
		message: &msg,
		stats: session.SessionStatsUpdate{
			AddInputTokens:  int32(data.InputTokens),
			AddOutputTokens: int32(data.OutputTokens),
			AddCostUSD:      data.TotalCost,
		},
	}, true
}

// convertConversationStarted records the system prompt.
func (s *OmniaEventStore) convertConversationStarted(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ConversationStartedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	metadata := map[string]string{
		metaKeyType:   "conversation_started",
		metaKeySource: metaValueSource,
	}

	msg := s.buildMessage(session.RoleSystem, data.SystemPrompt, event.Timestamp, metadata)
	return eventAction{message: &msg}, true
}

// Metadata key constants for tool registry enrichment.
const (
	metaKeyHandlerName       = "handler_name"
	metaKeyHandlerType       = "handler_type"
	metaKeyRegistryName      = "registry_name"
	metaKeyRegistryNamespace = "registry_namespace"
)

// --- Tool call events ---

// convertToolCallStarted creates a first-class ToolCall record.
func (s *OmniaEventStore) convertToolCallStarted(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ToolCallStartedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	// Build first-class ToolCall record.
	tcID := uuid.New().String()
	tc := session.ToolCall{
		ID:        tcID,
		CallID:    data.CallID,
		Name:      data.ToolName,
		Arguments: data.Args,
		Status:    session.ToolCallStatusPending,
		Execution: session.ToolCallExecutionServer,
		CreatedAt: event.Timestamp,
	}
	s.enrichToolCallLabels(&tc, data.ToolName)

	// Store the key so completed/failed events can upsert the same row.
	s.toolCallKeys.Store(data.CallID, callKey{id: tcID, createdAt: event.Timestamp})

	return eventAction{
		toolCall: &tc,
	}, true
}

// convertToolCallCompleted creates an updated ToolCall record (upsert).
func (s *OmniaEventStore) convertToolCallCompleted(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ToolCallCompletedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	resultBody := textFromParts(data.Parts)

	// Build ToolCall upsert — reuse the ID from the started event.
	tc := s.buildToolCallUpsert(data.CallID, data.ToolName, event.Timestamp)
	tc.Status = session.ToolCallStatusSuccess
	tc.DurationMs = data.Duration.Milliseconds()
	if resultBody != "" {
		tc.Result = resultBody
	}

	return eventAction{toolCall: &tc}, true
}

// convertToolCallFailed creates an errored ToolCall record.
func (s *OmniaEventStore) convertToolCallFailed(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ToolCallFailedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	errMsg := "unknown error"
	if data.Error != nil {
		errMsg = data.Error.Error()
	}

	tc := s.buildToolCallUpsert(data.CallID, data.ToolName, event.Timestamp)
	tc.Status = session.ToolCallStatusError
	tc.DurationMs = data.Duration.Milliseconds()
	tc.ErrorMessage = errMsg

	return eventAction{toolCall: &tc}, true
}

// --- Client tool events ---

// convertClientToolRequest records a client-side tool request as a first-class ToolCall.
func (s *OmniaEventStore) convertClientToolRequest(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ClientToolRequestData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	tcID := uuid.New().String()
	tc := session.ToolCall{
		ID:        tcID,
		CallID:    data.CallID,
		Name:      data.ToolName,
		Arguments: data.Args,
		Status:    session.ToolCallStatusPending,
		Execution: session.ToolCallExecutionClient,
		CreatedAt: event.Timestamp,
	}
	s.enrichToolCallLabels(&tc, data.ToolName)
	s.toolCallKeys.Store(data.CallID, callKey{id: tcID, createdAt: event.Timestamp})

	return eventAction{toolCall: &tc}, true
}

// NOTE: convertClientToolResolved will be added when the published PromptKit SDK
// includes EventClientToolResolved and ClientToolResolvedData types.

// --- Provider call events ---

// convertProviderCallStarted records the start of a provider call.
func (s *OmniaEventStore) convertProviderCallStarted(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ProviderCallStartedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	pcID := uuid.New().String()
	pc := session.ProviderCall{
		ID:        pcID,
		Provider:  data.Provider,
		Model:     data.Model,
		Status:    session.ProviderCallStatusPending,
		CreatedAt: event.Timestamp,
	}

	// Build a stable call key. ProviderCallStartedData has no CallID,
	// so we key on "provider:model:timestamp" to correlate with completed/failed.
	key := data.Provider + ":" + data.Model + ":" + event.Timestamp.Format(time.RFC3339Nano)
	s.providerCallKeys.Store(key, callKey{id: pcID, createdAt: event.Timestamp})

	return eventAction{providerCall: &pc}, true
}

// convertProviderCallCompleted records provider call completion.
func (s *OmniaEventStore) convertProviderCallCompleted(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ProviderCallCompletedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	pc := s.buildProviderCallUpsert(data.Provider, data.Model, event.Timestamp)
	pc.Status = session.ProviderCallStatusCompleted
	pc.InputTokens = int64(data.InputTokens)
	pc.OutputTokens = int64(data.OutputTokens)
	pc.CachedTokens = int64(data.CachedTokens)
	pc.CostUSD = data.Cost
	pc.DurationMs = data.Duration.Milliseconds()
	pc.FinishReason = data.FinishReason

	return eventAction{
		providerCall: &pc,
		// Stats still needed — session counters for tokens/cost.
		stats: session.SessionStatsUpdate{
			AddInputTokens:  int32(data.InputTokens),
			AddOutputTokens: int32(data.OutputTokens),
			AddCostUSD:      data.Cost,
		},
	}, true
}

// convertProviderCallFailed records a provider call failure.
func (s *OmniaEventStore) convertProviderCallFailed(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.ProviderCallFailedData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	errMsg := ""
	if data.Error != nil {
		errMsg = data.Error.Error()
	}

	pc := s.buildProviderCallUpsert(data.Provider, data.Model, event.Timestamp)
	pc.Status = session.ProviderCallStatusFailed
	pc.DurationMs = data.Duration.Milliseconds()
	pc.ErrorMessage = errMsg

	return eventAction{providerCall: &pc}, true
}

// --- Eval events ---

// convertEvalEvent records an eval completed/failed event as a RuntimeEvent.
// The authoritative eval_results record is written by the arena eval worker;
// this RuntimeEvent preserves timeline visibility without polluting messages.
func (s *OmniaEventStore) convertEvalEvent(event *events.Event) (eventAction, bool) {
	data, ok := asPtr[events.EvalEventData](event.Data)
	if !ok {
		return eventAction{}, false
	}

	// Round-trip through JSON to convert typed struct to map[string]any.
	var evtData map[string]any
	raw, err := json.Marshal(data)
	if err == nil {
		_ = json.Unmarshal(raw, &evtData)
	}

	evtType := string(event.Type)
	var errMsg string
	if data.Error != "" {
		errMsg = data.Error
	}

	evt := session.RuntimeEvent{
		ID:           uuid.New().String(),
		EventType:    evtType,
		Data:         evtData,
		DurationMs:   data.DurationMs,
		ErrorMessage: errMsg,
		Timestamp:    event.Timestamp,
	}

	return eventAction{event: &evt}, true
}

// --- Generic event handler ---

// convertGenericEvent records any event type as a first-class RuntimeEvent.
// This ensures full recording fidelity — no events are silently dropped.
func (s *OmniaEventStore) convertGenericEvent(event *events.Event) (eventAction, bool) {
	var data map[string]any
	if event.Data != nil {
		// Round-trip through JSON to convert typed structs to map[string]any.
		raw, err := json.Marshal(event.Data)
		if err == nil {
			_ = json.Unmarshal(raw, &data)
		}
	}

	evt := session.RuntimeEvent{
		ID:        uuid.New().String(),
		EventType: string(event.Type),
		Data:      data,
		Timestamp: event.Timestamp,
	}

	return eventAction{event: &evt}, true
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

// writeAction persists an eventAction to session-api: first-class tool/provider call records,
// runtime events, and/or legacy messages.
// Errors are logged but never propagated, matching the facade's fire-and-forget pattern.
func (s *OmniaEventStore) writeAction(traceCtx context.Context, sessionID string, action eventAction) {
	if s.sessionStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(traceCtx, writeTimeout)
	defer cancel()
	log := logctx.LoggerWithContext(s.log, traceCtx)

	eventType := s.resolveEventType(action)

	log.V(1).Info("writing event to session-api",
		"sessionID", sessionID, "eventType", eventType)

	// Write first-class records first (they update session counters atomically).
	if action.toolCall != nil {
		action.toolCall.SessionID = sessionID
		if err := s.sessionStore.RecordToolCall(ctx, sessionID, *action.toolCall); err != nil {
			log.Error(err, "failed to record tool call",
				"sessionID", sessionID, "toolName", action.toolCall.Name)
		}
	}

	if action.providerCall != nil {
		action.providerCall.SessionID = sessionID
		if err := s.sessionStore.RecordProviderCall(ctx, sessionID, *action.providerCall); err != nil {
			log.Error(err, "failed to record provider call",
				"sessionID", sessionID, "provider", action.providerCall.Provider)
		}
	}

	// Write runtime event (for lifecycle events).
	if action.event != nil {
		action.event.SessionID = sessionID
		if err := s.sessionStore.RecordRuntimeEvent(ctx, sessionID, *action.event); err != nil {
			log.Error(err, "failed to record runtime event",
				"sessionID", sessionID, "eventType", action.event.EventType)
		}
	}

	// Write message (for message/conversation events).
	if action.message != nil {
		if err := s.sessionStore.AppendMessage(ctx, sessionID, *action.message); err != nil {
			log.Error(err, "failed to append event message",
				"sessionID", sessionID, "eventType", eventType)
			return
		}
	}

	// Update session stats (tokens, cost, message counts, etc.).
	if err := s.sessionStore.UpdateSessionStats(ctx, sessionID, action.stats); err != nil {
		log.Error(err, "failed to update session stats",
			"sessionID", sessionID, "eventType", eventType)
		return
	}

	log.V(1).Info("event written to session-api",
		"sessionID", sessionID, "eventType", eventType)
}

// resolveEventType returns a descriptive string for the action being written.
func (s *OmniaEventStore) resolveEventType(action eventAction) string {
	if action.event != nil {
		return action.event.EventType
	}
	if action.message != nil {
		return action.message.Metadata[metaKeyType]
	}
	if action.toolCall != nil {
		return "tool_call:" + action.toolCall.Name
	}
	if action.providerCall != nil {
		return "provider_call:" + action.providerCall.Provider
	}
	return "unknown"
}

// --- Dual-write helpers ---

// buildToolCallUpsert creates a ToolCall for an upsert (completed/failed),
// reusing the ID from the started event if available.
func (s *OmniaEventStore) buildToolCallUpsert(callID, toolName string, ts time.Time) session.ToolCall {
	tc := session.ToolCall{
		CallID:    callID,
		Name:      toolName,
		CreatedAt: ts,
		Execution: session.ToolCallExecutionServer,
	}

	if v, ok := s.toolCallKeys.LoadAndDelete(callID); ok {
		ck := v.(callKey)
		tc.ID = ck.id
		tc.CreatedAt = ck.createdAt
	} else {
		tc.ID = uuid.New().String()
	}

	s.enrichToolCallLabels(&tc, toolName)
	return tc
}

// buildProviderCallUpsert creates a ProviderCall for an upsert (completed/failed),
// reusing the ID from the started event if available.
func (s *OmniaEventStore) buildProviderCallUpsert(provider, model string, ts time.Time) session.ProviderCall {
	pc := session.ProviderCall{
		Provider:  provider,
		Model:     model,
		CreatedAt: ts,
	}

	key := provider + ":" + model + ":" + ts.Format(time.RFC3339Nano)
	if v, ok := s.providerCallKeys.LoadAndDelete(key); ok {
		ck := v.(callKey)
		pc.ID = ck.id
		pc.CreatedAt = ck.createdAt
	} else {
		pc.ID = uuid.New().String()
	}

	return pc
}

// enrichToolCallLabels adds tool registry metadata as labels on the ToolCall.
func (s *OmniaEventStore) enrichToolCallLabels(tc *session.ToolCall, toolName string) {
	if s.toolMetaFn == nil {
		return
	}
	meta, ok := s.toolMetaFn(toolName)
	if !ok {
		return
	}
	if tc.Labels == nil {
		tc.Labels = make(map[string]string)
	}
	tc.Labels[metaKeyHandlerName] = meta.HandlerName
	tc.Labels[metaKeyHandlerType] = meta.HandlerType
	tc.Labels[metaKeyRegistryName] = meta.RegistryName
	tc.Labels[metaKeyRegistryNamespace] = meta.RegistryNamespace
}

// detachedTraceContext returns a context that inherits all values (including
// span context and logctx trace_id) from ctx but does not inherit its
// cancellation or deadline. This is used for async fire-and-forget writes.
func detachedTraceContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// Verify interface compliance at compile time.
var _ events.EventStore = (*OmniaEventStore)(nil)
