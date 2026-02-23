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

package otlp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// SessionWriter is the subset of SessionService used by the transformer.
type SessionWriter interface {
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)
	CreateSession(ctx context.Context, sess *session.Session) error
	AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error
	UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error
}

// Transformer converts OTLP GenAI spans into session data.
type Transformer struct {
	writer SessionWriter
	log    logr.Logger
}

// NewTransformer creates a new Transformer.
func NewTransformer(writer SessionWriter, log logr.Logger) *Transformer {
	return &Transformer{
		writer: writer,
		log:    log.WithName("otlp-transformer"),
	}
}

// spanContext holds resource-level attributes extracted once per ResourceSpans.
type spanContext struct {
	namespace     string
	agentName     string
	resourceAttrs []*commonpb.KeyValue
}

// ProcessExport processes an OTLP export request and returns the number of
// spans that were successfully processed.
func (t *Transformer) ProcessExport(ctx context.Context, resourceSpans []*tracepb.ResourceSpans) (int, error) {
	var processed int
	var firstErr error

	for _, rs := range resourceSpans {
		n, err := t.processResourceSpans(ctx, rs)
		processed += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return processed, firstErr
}

// processResourceSpans extracts resource attributes and iterates over scope spans.
func (t *Transformer) processResourceSpans(ctx context.Context, rs *tracepb.ResourceSpans) (int, error) {
	var resourceAttrs []*commonpb.KeyValue
	if rs.GetResource() != nil {
		resourceAttrs = rs.GetResource().GetAttributes()
	}

	sc := spanContext{
		namespace:     getStringAttr(resourceAttrs, AttrServiceNamespace),
		agentName:     getStringAttr(resourceAttrs, AttrServiceName),
		resourceAttrs: resourceAttrs,
	}

	var processed int
	var firstErr error

	for _, scopeSpans := range rs.GetScopeSpans() {
		n, err := t.processScopeSpans(ctx, sc, scopeSpans)
		processed += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return processed, firstErr
}

// processScopeSpans iterates over spans within a scope.
func (t *Transformer) processScopeSpans(ctx context.Context, sc spanContext, ss *tracepb.ScopeSpans) (int, error) {
	spans := ss.GetSpans()
	sortSpansByTime(spans)

	var processed int
	var firstErr error

	for _, span := range spans {
		if err := t.processSpan(ctx, sc, span); err != nil {
			t.log.Error(err, "failed to process span", "spanID", fmt.Sprintf("%x", span.GetSpanId()))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		processed++
	}

	return processed, firstErr
}

// processSpan extracts conversation ID, ensures the session exists, and
// appends output messages and token usage.
func (t *Transformer) processSpan(ctx context.Context, sc spanContext, span *tracepb.Span) error {
	attrs := span.GetAttributes()
	spanName := span.GetName()

	sessionID := extractSessionID(attrs, sc.resourceAttrs, span.GetTraceId())
	if sessionID == "" {
		return nil // no way to identify a session — skip
	}

	if err := t.ensureSession(ctx, sessionID, sc, attrs); err != nil {
		return fmt.Errorf("ensuring session %s: %w", sessionID, err)
	}

	timestamp := spanTimestamp(span)

	// Route enriched spans by name prefix.
	switch {
	case strings.HasPrefix(spanName, "tool."):
		return t.processToolSpan(ctx, sessionID, attrs, timestamp)
	case spanName == "workflow.transition":
		return t.processWorkflowTransition(ctx, sessionID, attrs, timestamp)
	case spanName == "workflow.completed":
		return t.processWorkflowCompleted(ctx, sessionID, attrs, timestamp)
	}

	// Default: GenAI conversation messages + token usage.
	model := extractModel(attrs)
	msgs := t.resolveMessages(attrs, span.GetEvents(), timestamp, model)

	for _, msg := range msgs {
		if err := t.writer.AppendMessage(ctx, sessionID, msg); err != nil {
			return fmt.Errorf("appending message to session %s: %w", sessionID, err)
		}
	}

	return t.updateTokenUsage(ctx, sessionID, attrs)
}

// ensureSession creates the session if it does not already exist.
func (t *Transformer) ensureSession(ctx context.Context, sessionID string, sc spanContext, spanAttrs []*commonpb.KeyValue) error {
	_, err := t.writer.GetSession(ctx, sessionID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		return err
	}

	now := time.Now()
	sess := &session.Session{
		ID:        sessionID,
		AgentName: sc.agentName,
		Namespace: sc.namespace,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    session.SessionStatusActive,
		State:     buildSessionState(spanAttrs),
	}

	return t.writer.CreateSession(ctx, sess)
}

// buildSessionState extracts metadata for the session state map.
func buildSessionState(attrs []*commonpb.KeyValue) map[string]string {
	state := make(map[string]string)
	if provider := extractProviderName(attrs); provider != "" {
		state["gen_ai.provider"] = provider
	}
	if model := extractModel(attrs); model != "" {
		state["gen_ai.model"] = model
	}
	if len(state) == 0 {
		return nil
	}
	return state
}

// resolveMessages extracts messages using multiple strategies in priority order:
// 1. Current OTel: gen_ai.input.messages / gen_ai.output.messages (structured)
// 2. Span events: gen_ai.client.inference.operation.details event
// 3. Legacy OpenLLMetry: gen_ai.prompt.{i}.* / gen_ai.completion.{i}.*
func (t *Transformer) resolveMessages(attrs []*commonpb.KeyValue, events []*tracepb.Span_Event, timestamp time.Time, model string) []*session.Message {
	// Strategy 1: current OTel structured attributes.
	msgs := extractStructuredMessages(attrs, timestamp)

	// Strategy 2: span events.
	if len(msgs) == 0 {
		msgs = extractMessagesFromEvents(events, timestamp)
	}

	// Strategy 3: legacy OpenLLMetry indexed attributes.
	if len(msgs) == 0 {
		msgs = extractLegacyMessages(attrs, timestamp)
	}

	applyMessageDefaults(msgs, model)
	return msgs
}

// extractStructuredMessages parses gen_ai.input.messages and gen_ai.output.messages.
func extractStructuredMessages(attrs []*commonpb.KeyValue, timestamp time.Time) []*session.Message {
	var msgs []*session.Message

	inputValues := getArrayAttr(attrs, AttrGenAIInputMessages)
	for _, v := range inputValues {
		if msg := parseMessageValue(v, timestamp); msg != nil {
			msgs = append(msgs, msg)
		}
	}

	outputValues := getArrayAttr(attrs, AttrGenAIOutputMessages)
	for _, v := range outputValues {
		msg := parseMessageValueWithDefault(v, session.RoleAssistant, timestamp)
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}

// extractMessagesFromEvents checks span events for the consolidated
// gen_ai.client.inference.operation.details event.
func extractMessagesFromEvents(events []*tracepb.Span_Event, timestamp time.Time) []*session.Message {
	for _, event := range events {
		if event.GetName() != "gen_ai.client.inference.operation.details" {
			continue
		}
		return extractStructuredMessages(event.GetAttributes(), timestamp)
	}
	return nil
}

// extractLegacyMessages parses OpenLLMetry indexed attributes.
func extractLegacyMessages(attrs []*commonpb.KeyValue, timestamp time.Time) []*session.Message {
	// Input: gen_ai.prompt.{i}.role / gen_ai.prompt.{i}.content
	inputIndexed := extractIndexedMessages(attrs, AttrGenAIPromptPrefix)
	outputIndexed := extractIndexedMessages(attrs, AttrGenAICompletionPrefix)
	msgs := make([]*session.Message, 0, len(inputIndexed)+len(outputIndexed))
	for _, im := range inputIndexed {
		msg := indexedToSingleMessage(im, timestamp)
		msgs = append(msgs, msg)
	}

	// Output: gen_ai.completion.{i}.role / gen_ai.completion.{i}.content
	for _, im := range outputIndexed {
		msg := indexedToSingleMessage(im, timestamp)
		if msg.Role == "" {
			msg.Role = session.RoleAssistant
		}
		msgs = append(msgs, msg)
	}

	return msgs
}

// indexedToSingleMessage converts a single indexedMessage to a session.Message.
func indexedToSingleMessage(im indexedMessage, timestamp time.Time) *session.Message {
	role := toMessageRole(im.role)
	if role == "" {
		role = session.RoleAssistant
	}
	return &session.Message{
		ID:        uuid.New().String(),
		Role:      role,
		Content:   im.content,
		Timestamp: timestamp,
	}
}

// applyMessageDefaults sets IDs and model metadata on messages that lack them.
func applyMessageDefaults(msgs []*session.Message, model string) {
	for _, msg := range msgs {
		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}
		if model != "" {
			if msg.Metadata == nil {
				msg.Metadata = make(map[string]string)
			}
			msg.Metadata["gen_ai.model"] = model
		}
	}
}

// updateTokenUsage extracts token counts and applies them as a stats update.
func (t *Transformer) updateTokenUsage(ctx context.Context, sessionID string, attrs []*commonpb.KeyValue) error {
	inputTokens, outputTokens := extractTokenUsage(attrs)
	if inputTokens == 0 && outputTokens == 0 {
		return nil
	}

	update := session.SessionStatsUpdate{
		AddInputTokens:  int32(inputTokens),
		AddOutputTokens: int32(outputTokens),
	}

	return t.writer.UpdateSessionStats(ctx, sessionID, update)
}

// processToolSpan converts a tool.* span into a session message matching
// the metadata format used by event_store.go handleToolCallCompleted.
func (t *Transformer) processToolSpan(ctx context.Context, sessionID string, attrs []*commonpb.KeyValue, ts time.Time) error {
	msg := &session.Message{
		ID:         uuid.New().String(),
		Role:       session.RoleSystem,
		Timestamp:  ts,
		ToolCallID: getStringAttr(attrs, AttrToolCallID),
		Metadata: map[string]string{
			"type":        "tool.call.completed",
			"tool_name":   getStringAttr(attrs, AttrToolName),
			"tool_args":   getStringAttr(attrs, AttrToolArgs),
			"status":      getStringAttr(attrs, AttrToolStatus),
			"duration_ms": strconv.FormatInt(getIntAttr(attrs, AttrToolDurationMs), 10),
		},
	}
	return t.writer.AppendMessage(ctx, sessionID, msg)
}

// processWorkflowTransition converts a workflow.transition span into a session
// message matching the metadata format used by event_store.go handleWorkflowTransitioned.
func (t *Transformer) processWorkflowTransition(ctx context.Context, sessionID string, attrs []*commonpb.KeyValue, ts time.Time) error {
	msg := &session.Message{
		ID:        uuid.New().String(),
		Role:      session.RoleSystem,
		Timestamp: ts,
		Metadata: map[string]string{
			"type":        "workflow.transitioned",
			"from_state":  getStringAttr(attrs, AttrWorkflowFromState),
			"to_state":    getStringAttr(attrs, AttrWorkflowToState),
			"event":       getStringAttr(attrs, AttrWorkflowEvent),
			"prompt_task": getStringAttr(attrs, AttrWorkflowPromptTask),
		},
	}
	return t.writer.AppendMessage(ctx, sessionID, msg)
}

// processWorkflowCompleted converts a workflow.completed span into a session
// message matching the metadata format used by event_store.go handleWorkflowCompleted.
func (t *Transformer) processWorkflowCompleted(ctx context.Context, sessionID string, attrs []*commonpb.KeyValue, ts time.Time) error {
	msg := &session.Message{
		ID:        uuid.New().String(),
		Role:      session.RoleSystem,
		Timestamp: ts,
		Metadata: map[string]string{
			"type":             "workflow.completed",
			"final_state":      getStringAttr(attrs, AttrWorkflowFinalState),
			"transition_count": strconv.FormatInt(getIntAttr(attrs, AttrWorkflowTransitionCount), 10),
		},
	}
	return t.writer.AppendMessage(ctx, sessionID, msg)
}

// parseMessageValue extracts role and content from a kvlist AnyValue.
func parseMessageValue(v *commonpb.AnyValue, timestamp time.Time) *session.Message {
	return parseMessageValueWithDefault(v, "", timestamp)
}

// parseMessageValueWithDefault extracts role and content from a kvlist AnyValue,
// using the provided default role when the message lacks an explicit role.
func parseMessageValueWithDefault(v *commonpb.AnyValue, defaultRole session.MessageRole, timestamp time.Time) *session.Message {
	kvl := v.GetKvlistValue()
	if kvl == nil {
		return nil
	}

	role, content := extractRoleAndContent(kvl.GetValues())
	if content == "" {
		return nil
	}

	msgRole := toMessageRole(role)
	if msgRole == "" {
		msgRole = defaultRole
	}
	if msgRole == "" {
		return nil
	}

	return &session.Message{
		ID:        uuid.New().String(),
		Role:      msgRole,
		Content:   content,
		Timestamp: timestamp,
	}
}

// extractRoleAndContent reads role and content from a kvlist's key-value pairs.
// Content can come from a "content" key or from "parts" (new spec format).
func extractRoleAndContent(kvs []*commonpb.KeyValue) (role, content string) {
	for _, kv := range kvs {
		switch kv.GetKey() {
		case "role":
			role = kv.GetValue().GetStringValue()
		case "content":
			content = kv.GetValue().GetStringValue()
		case "parts":
			if content == "" {
				content = extractContentFromParts(kv.GetValue())
			}
		}
	}
	return
}

// extractContentFromParts extracts text content from the "parts" array format
// used by the current OTel GenAI spec.
func extractContentFromParts(v *commonpb.AnyValue) string {
	arr := v.GetArrayValue()
	if arr == nil {
		return ""
	}
	for _, part := range arr.GetValues() {
		kvl := part.GetKvlistValue()
		if kvl == nil {
			continue
		}
		partType := getStringAttr(kvl.GetValues(), "type")
		if partType == "text" || partType == "" {
			if c := getStringAttr(kvl.GetValues(), "content"); c != "" {
				return c
			}
		}
	}
	return ""
}

// spanTimestamp converts the span's StartTimeUnixNano to a time.Time.
func spanTimestamp(span *tracepb.Span) time.Time {
	nanos := span.GetStartTimeUnixNano()
	if nanos == 0 {
		return time.Now()
	}
	return time.Unix(0, int64(nanos))
}

// sortSpansByTime sorts spans by start time ascending.
func sortSpansByTime(spans []*tracepb.Span) {
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].GetStartTimeUnixNano() < spans[j].GetStartTimeUnixNano()
	})
}
