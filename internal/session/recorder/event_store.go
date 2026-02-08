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

package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// streamChanSize is the buffer size for event stream channels.
const streamChanSize = 100

// OmniaEventStore implements the PromptKit events.EventStore interface backed by
// Omnia's warm store. All write operations are asynchronous — errors are logged
// but never propagated to callers, following the graceful degradation pattern.
type OmniaEventStore struct {
	warmStore providers.WarmStoreProvider
	blobStore *OmniaBlobStore
	log       logr.Logger
	sequence  atomic.Int64
}

// NewOmniaEventStore creates a new event store bridge.
func NewOmniaEventStore(
	warmStore providers.WarmStoreProvider,
	blobStore *OmniaBlobStore,
	log logr.Logger,
) *OmniaEventStore {
	return &OmniaEventStore{
		warmStore: warmStore,
		blobStore: blobStore,
		log:       log.WithName("omnia-event-store"),
	}
}

// Append converts a PromptKit event into an Omnia session.Message and persists
// it asynchronously via the warm store. Binary payloads (audio/image/video) are
// stored via the blob store. Provider completion events also update session stats.
func (s *OmniaEventStore) Append(ctx context.Context, event *events.Event) error {
	if event.SessionID == "" {
		return fmt.Errorf("event has no session ID")
	}

	seq := s.sequence.Add(1)

	// Detach from caller's cancellation so the goroutine can finish after the
	// request returns, while preserving any values attached to the context.
	detached := context.WithoutCancel(ctx)

	go func() {
		msg := s.eventToMessage(detached, event, seq)
		if msg == nil {
			return
		}

		if err := s.warmStore.AppendMessage(detached, event.SessionID, msg); err != nil {
			s.log.Error(err, "failed to append message",
				"sessionID", event.SessionID,
				"eventType", string(event.Type))
		}

		// Update session stats for provider completion events
		s.maybeUpdateSessionStats(detached, event)
	}()

	return nil
}

// eventToMessage converts a PromptKit event to an Omnia session.Message.
func (s *OmniaEventStore) eventToMessage(ctx context.Context, event *events.Event, seq int64) *session.Message {
	msg := &session.Message{
		ID:          uuid.New().String(),
		Timestamp:   event.Timestamp,
		SequenceNum: int32(seq),
		Metadata:    map[string]string{"type": string(event.Type)},
	}

	if event.ConversationID != "" {
		msg.Metadata["conversation_id"] = event.ConversationID
	}
	if event.RunID != "" {
		msg.Metadata["run_id"] = event.RunID
	}

	s.populateMessageFromEvent(ctx, msg, event)
	return msg
}

// populateMessageFromEvent fills message fields based on event type.
// Binary events (audio/image/video/screenshot) are dispatched via extractBinaryPayload.
func (s *OmniaEventStore) populateMessageFromEvent(ctx context.Context, msg *session.Message, event *events.Event) {
	// Try binary payload extraction first (audio, image, video, screenshot)
	if payload, mimeType, category, ok := extractBinaryPayload(event.Data); ok {
		s.handleBinaryEvent(ctx, msg, event.SessionID, payload, mimeType, category)
		return
	}

	switch data := event.Data.(type) {
	case *events.MessageCreatedData:
		s.handleMessageCreated(msg, data)
	case events.MessageCreatedData:
		s.handleMessageCreated(msg, &data)
	case *events.ToolCallStartedData:
		s.handleToolCallStarted(msg, data)
	case events.ToolCallStartedData:
		s.handleToolCallStarted(msg, &data)
	case *events.ToolCallCompletedData:
		s.handleToolCallCompleted(msg, data)
	case events.ToolCallCompletedData:
		s.handleToolCallCompleted(msg, &data)
	case *events.ToolCallFailedData:
		s.handleToolCallFailed(msg, data)
	case events.ToolCallFailedData:
		s.handleToolCallFailed(msg, &data)
	case *events.ProviderCallCompletedData:
		s.handleProviderCallCompleted(msg, data)
	case events.ProviderCallCompletedData:
		s.handleProviderCallCompleted(msg, &data)
	default:
		msg.Role = session.RoleSystem
		if event.Data != nil {
			content, _ := json.Marshal(event.Data)
			msg.Content = string(content)
		}
	}
}

// extractBinaryPayload returns the binary payload, mimeType, and category for
// audio/image/video/screenshot events. Returns ok=false for non-binary events.
func extractBinaryPayload(data events.EventData) (payload *events.BinaryPayload, mimeType, category string, ok bool) {
	switch d := data.(type) {
	case *events.AudioInputData:
		return &d.Payload, d.Payload.MIMEType, categoryAudio, true
	case events.AudioInputData:
		return &d.Payload, d.Payload.MIMEType, categoryAudio, true
	case *events.AudioOutputData:
		return &d.Payload, d.Payload.MIMEType, categoryAudio, true
	case events.AudioOutputData:
		return &d.Payload, d.Payload.MIMEType, categoryAudio, true
	case *events.ImageInputData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	case events.ImageInputData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	case *events.ImageOutputData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	case events.ImageOutputData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	case *events.VideoFrameData:
		return &d.Payload, d.Payload.MIMEType, categoryVideo, true
	case events.VideoFrameData:
		return &d.Payload, d.Payload.MIMEType, categoryVideo, true
	case *events.ScreenshotData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	case events.ScreenshotData:
		return &d.Payload, d.Payload.MIMEType, categoryImage, true
	default:
		return nil, "", "", false
	}
}

// handleToolCallStarted converts a tool call started event to a message.
func (s *OmniaEventStore) handleToolCallStarted(msg *session.Message, data *events.ToolCallStartedData) {
	msg.Role = session.RoleAssistant
	msg.ToolCallID = data.CallID
	content, _ := json.Marshal(map[string]any{
		"name": data.ToolName,
		"args": data.Args,
	})
	msg.Content = string(content)
}

// handleToolCallCompleted converts a tool call completed event to a message.
func (s *OmniaEventStore) handleToolCallCompleted(msg *session.Message, data *events.ToolCallCompletedData) {
	msg.Role = session.RoleSystem
	msg.ToolCallID = data.CallID
	msg.Metadata["tool_name"] = data.ToolName
	msg.Metadata["status"] = data.Status
	msg.Metadata["duration_ms"] = strconv.FormatInt(data.Duration.Milliseconds(), 10)
}

// handleToolCallFailed converts a tool call failed event to a message.
func (s *OmniaEventStore) handleToolCallFailed(msg *session.Message, data *events.ToolCallFailedData) {
	msg.Role = session.RoleSystem
	msg.ToolCallID = data.CallID
	if data.Error != nil {
		msg.Content = data.Error.Error()
	}
	msg.Metadata["tool_name"] = data.ToolName
}

// handleMessageCreated converts a MessageCreated event to the appropriate role and content.
func (s *OmniaEventStore) handleMessageCreated(msg *session.Message, data *events.MessageCreatedData) {
	switch data.Role {
	case "user":
		msg.Role = session.RoleUser
	case "assistant":
		msg.Role = session.RoleAssistant
	default:
		msg.Role = session.RoleSystem
	}
	msg.Content = data.Content
}

// handleProviderCallCompleted stores provider metrics as message metadata.
func (s *OmniaEventStore) handleProviderCallCompleted(msg *session.Message, data *events.ProviderCallCompletedData) {
	msg.Role = session.RoleSystem
	msg.InputTokens = int32(data.InputTokens)
	msg.OutputTokens = int32(data.OutputTokens)
	msg.Metadata["provider"] = data.Provider
	msg.Metadata["model"] = data.Model
	msg.Metadata["duration_ms"] = strconv.FormatInt(data.Duration.Milliseconds(), 10)
	msg.Metadata["cost_usd"] = strconv.FormatFloat(data.Cost, 'f', -1, 64)
	msg.Metadata["finish_reason"] = data.FinishReason
	msg.Metadata["input_tokens"] = strconv.Itoa(data.InputTokens)
	msg.Metadata["output_tokens"] = strconv.Itoa(data.OutputTokens)
}

// handleBinaryEvent stores binary payload via the blob store and records the
// storage reference in the message.
func (s *OmniaEventStore) handleBinaryEvent(
	ctx context.Context,
	msg *session.Message,
	sessionID string,
	payload *events.BinaryPayload,
	mimeType string,
	category string,
) {
	msg.Role = session.RoleSystem
	msg.Metadata["mime_type"] = mimeType
	msg.Metadata["category"] = category

	// If there's inline data, store it via the blob store
	if len(payload.InlineData) > 0 && s.blobStore != nil {
		result, err := s.blobStore.Store(ctx, sessionID, payload.InlineData, mimeType)
		if err != nil {
			s.log.Error(err, "failed to store binary payload",
				"sessionID", sessionID, "category", category)
			return
		}
		msg.Content = result.StorageRef
		msg.Metadata["size_bytes"] = strconv.FormatInt(result.Size, 10)
		msg.Metadata["checksum"] = result.Checksum
	} else if payload.StorageRef != "" {
		// Already has a storage ref — just record it
		msg.Content = payload.StorageRef
		msg.Metadata["size_bytes"] = strconv.FormatInt(payload.Size, 10)
		if payload.Checksum != "" {
			msg.Metadata["checksum"] = payload.Checksum
		}
	}
}

// maybeUpdateSessionStats updates session-level counters for provider completion events.
func (s *OmniaEventStore) maybeUpdateSessionStats(ctx context.Context, event *events.Event) {
	var data *events.ProviderCallCompletedData

	switch d := event.Data.(type) {
	case *events.ProviderCallCompletedData:
		data = d
	case events.ProviderCallCompletedData:
		data = &d
	default:
		return
	}

	// Fetch the session, update its stats, and save it back
	sess, err := s.warmStore.GetSession(ctx, event.SessionID)
	if err != nil {
		s.log.Error(err, "failed to get session for stats update", "sessionID", event.SessionID)
		return
	}

	sess.TotalInputTokens += int64(data.InputTokens)
	sess.TotalOutputTokens += int64(data.OutputTokens)
	sess.EstimatedCostUSD += data.Cost
	sess.UpdatedAt = time.Now()

	if err := s.warmStore.UpdateSession(ctx, sess); err != nil {
		s.log.Error(err, "failed to update session stats", "sessionID", event.SessionID)
	}
}

// Query returns events matching the filter by reading messages from the warm store
// and reconstructing PromptKit events.
func (s *OmniaEventStore) Query(ctx context.Context, filter *events.EventFilter) ([]*events.Event, error) {
	if filter.SessionID == "" {
		return nil, fmt.Errorf("session ID required for query")
	}

	opts := providers.MessageQueryOpts{
		SortOrder: providers.SortAsc,
	}
	if filter.Limit > 0 {
		opts.Limit = filter.Limit
	}

	messages, err := s.warmStore.GetMessages(ctx, filter.SessionID, opts)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}

	var result []*events.Event
	for _, msg := range messages {
		event := s.messageToEvent(msg, filter.SessionID)
		if s.matchesFilter(event, filter) {
			result = append(result, event)
		}
	}
	return result, nil
}

// QueryRaw returns stored events with raw data preserved.
func (s *OmniaEventStore) QueryRaw(ctx context.Context, filter *events.EventFilter) ([]*events.StoredEvent, error) {
	evts, err := s.Query(ctx, filter)
	if err != nil {
		return nil, err
	}

	stored := make([]*events.StoredEvent, 0, len(evts))
	for i, evt := range evts {
		se, err := marshalSerializableEvent(evt)
		if err != nil {
			s.log.Error(err, "failed to serialize event for QueryRaw")
			continue
		}
		stored = append(stored, &events.StoredEvent{
			Sequence: int64(i + 1),
			Event:    se,
		})
	}
	return stored, nil
}

// marshalSerializableEvent converts an Event to a SerializableEvent.
func marshalSerializableEvent(e *events.Event) (*events.SerializableEvent, error) {
	se := &events.SerializableEvent{
		Type:           e.Type,
		Timestamp:      e.Timestamp,
		RunID:          e.RunID,
		SessionID:      e.SessionID,
		ConversationID: e.ConversationID,
	}
	if e.Data != nil {
		se.DataType = fmt.Sprintf("%T", e.Data)
		data, err := json.Marshal(e.Data)
		if err != nil {
			return nil, err
		}
		se.Data = data
	}
	return se, nil
}

// Stream returns a channel of events for a session.
func (s *OmniaEventStore) Stream(ctx context.Context, sessionID string) (<-chan *events.Event, error) {
	evts, err := s.Query(ctx, &events.EventFilter{SessionID: sessionID})
	if err != nil {
		return nil, err
	}

	ch := make(chan *events.Event, streamChanSize)
	go func() {
		defer close(ch)
		for _, evt := range evts {
			select {
			case <-ctx.Done():
				return
			case ch <- evt:
			}
		}
	}()
	return ch, nil
}

// Close is a no-op; the underlying providers are managed by the Registry.
func (s *OmniaEventStore) Close() error {
	return nil
}

// messageToEvent reconstructs a PromptKit event from an Omnia session.Message.
func (s *OmniaEventStore) messageToEvent(msg *session.Message, sessionID string) *events.Event {
	eventType := events.EventType(msg.Metadata["type"])
	if eventType == "" {
		eventType = events.EventMessageCreated
	}

	event := &events.Event{
		Type:      eventType,
		Timestamp: msg.Timestamp,
		SessionID: sessionID,
	}

	if cid, ok := msg.Metadata["conversation_id"]; ok {
		event.ConversationID = cid
	}
	if rid, ok := msg.Metadata["run_id"]; ok {
		event.RunID = rid
	}

	// Reconstruct event data based on type
	switch eventType {
	case events.EventMessageCreated:
		role := string(msg.Role)
		event.Data = &events.MessageCreatedData{
			Role:    role,
			Content: msg.Content,
		}

	case events.EventToolCallStarted:
		event.Data = &events.ToolCallStartedData{
			ToolName: msg.Metadata["tool_name"],
			CallID:   msg.ToolCallID,
		}

	case events.EventToolCallCompleted:
		event.Data = &events.ToolCallCompletedData{
			ToolName: msg.Metadata["tool_name"],
			CallID:   msg.ToolCallID,
			Status:   msg.Metadata["status"],
		}

	case events.EventToolCallFailed:
		event.Data = &events.ToolCallFailedData{
			ToolName: msg.Metadata["tool_name"],
			CallID:   msg.ToolCallID,
		}

	case events.EventProviderCallCompleted:
		inputTokens, _ := strconv.Atoi(msg.Metadata["input_tokens"])
		outputTokens, _ := strconv.Atoi(msg.Metadata["output_tokens"])
		cost, _ := strconv.ParseFloat(msg.Metadata["cost_usd"], 64)
		event.Data = &events.ProviderCallCompletedData{
			Provider:     msg.Metadata["provider"],
			Model:        msg.Metadata["model"],
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			Cost:         cost,
			FinishReason: msg.Metadata["finish_reason"],
		}
	}

	return event
}

// matchesFilter checks if an event matches the query filter.
func (s *OmniaEventStore) matchesFilter(event *events.Event, filter *events.EventFilter) bool {
	if filter.ConversationID != "" && event.ConversationID != filter.ConversationID {
		return false
	}
	if filter.RunID != "" && event.RunID != filter.RunID {
		return false
	}
	if !filter.Since.IsZero() && event.Timestamp.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && event.Timestamp.After(filter.Until) {
		return false
	}
	if len(filter.Types) > 0 && !slices.Contains(filter.Types, event.Type) {
		return false
	}
	return true
}

// Ensure OmniaEventStore implements events.EventStore.
var _ events.EventStore = (*OmniaEventStore)(nil)
