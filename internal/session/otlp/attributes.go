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

// Package otlp provides an OTLP ingestion endpoint that converts OpenTelemetry
// GenAI traces into Omnia session data.
package otlp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/altairalabs/omnia/internal/session"
)

// Current OTel GenAI semantic convention attribute keys (v1.37+).
// See: https://opentelemetry.io/docs/specs/semconv/gen-ai/
const (
	AttrGenAIConversationID = "gen_ai.conversation.id"
	AttrGenAIOperationName  = "gen_ai.operation.name"
	AttrGenAIRequestModel   = "gen_ai.request.model"
	AttrGenAIResponseModel  = "gen_ai.response.model"
	AttrGenAIProviderName   = "gen_ai.provider.name"
	AttrGenAIOutputMessages = "gen_ai.output.messages"
	AttrGenAIInputMessages  = "gen_ai.input.messages"
	AttrGenAIUsageInput     = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutput    = "gen_ai.usage.output_tokens"
)

// Deprecated attribute keys still widely emitted by OpenLLMetry and others.
const (
	AttrGenAISystem              = "gen_ai.system"
	AttrGenAIUsagePromptTokens   = "gen_ai.usage.prompt_tokens"
	AttrGenAIUsageComplTokens    = "gen_ai.usage.completion_tokens"
	AttrGenAIPromptPrefix        = "gen_ai.prompt."
	AttrGenAICompletionPrefix    = "gen_ai.completion."
	AttrLLMUsageTotalTokens      = "llm.usage.total_tokens"
	AttrTraceloopAssocProperties = "traceloop.association.properties"
)

// Session ID fallback attribute keys used by various tools.
const (
	AttrSessionID         = "session.id"
	AttrLangfuseSessionID = "langfuse.session.id"
)

// Resource attribute keys.
const (
	AttrServiceName      = "service.name"
	AttrServiceNamespace = "service.namespace"
)

// Omnia custom attribute keys.
const (
	AttrOmniaAgentName           = "omnia.agent.name"
	AttrOmniaAgentNamespace      = "omnia.agent.namespace"
	AttrOmniaPromptPackName      = "omnia.promptpack.name"
	AttrOmniaPromptPackVersion   = "omnia.promptpack.version"
	AttrOmniaPromptPackNamespace = "omnia.promptpack.namespace"
)

// PromptKit tool span attribute keys.
const (
	AttrToolName       = "tool.name"
	AttrToolCallID     = "tool.call_id"
	AttrToolArgs       = "tool.args"
	AttrToolStatus     = "tool.status"
	AttrToolDurationMs = "tool.duration_ms"
)

// PromptKit workflow span attribute keys.
const (
	AttrWorkflowFromState       = "workflow.from_state"
	AttrWorkflowToState         = "workflow.to_state"
	AttrWorkflowEvent           = "workflow.event"
	AttrWorkflowPromptTask      = "workflow.prompt_task"
	AttrWorkflowFinalState      = "workflow.final_state"
	AttrWorkflowTransitionCount = "workflow.transition_count"
)

// getStringAttr retrieves a string attribute value from a KeyValue slice.
func getStringAttr(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			if sv := kv.GetValue().GetStringValue(); sv != "" {
				return sv
			}
		}
	}
	return ""
}

// getIntAttr retrieves an integer attribute value from a KeyValue slice.
func getIntAttr(attrs []*commonpb.KeyValue, key string) int64 {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			return kv.GetValue().GetIntValue()
		}
	}
	return 0
}

// getArrayAttr retrieves an array attribute value from a KeyValue slice.
func getArrayAttr(attrs []*commonpb.KeyValue, key string) []*commonpb.AnyValue {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			if arr := kv.GetValue().GetArrayValue(); arr != nil {
				return arr.GetValues()
			}
		}
	}
	return nil
}

// getStringAttrMulti tries multiple keys in priority order, returning the first non-empty value.
func getStringAttrMulti(attrs []*commonpb.KeyValue, keys ...string) string {
	for _, key := range keys {
		if v := getStringAttr(attrs, key); v != "" {
			return v
		}
	}
	return ""
}

// getIntAttrMulti tries multiple keys in priority order, returning the first non-zero value.
func getIntAttrMulti(attrs []*commonpb.KeyValue, keys ...string) int64 {
	for _, key := range keys {
		if v := getIntAttr(attrs, key); v != 0 {
			return v
		}
	}
	return 0
}

// extractSessionID resolves a session identifier from span and resource attributes.
// Priority: gen_ai.conversation.id → session.id → langfuse.session.id → traceID hex.
func extractSessionID(spanAttrs, resourceAttrs []*commonpb.KeyValue, traceID []byte) string {
	if id := getStringAttrMulti(spanAttrs, AttrGenAIConversationID, AttrSessionID, AttrLangfuseSessionID); id != "" {
		return id
	}
	if id := getStringAttrMulti(resourceAttrs, AttrSessionID, AttrLangfuseSessionID); id != "" {
		return id
	}
	if len(traceID) > 0 {
		return fmt.Sprintf("%x", traceID)
	}
	return ""
}

// extractProviderName returns the GenAI provider, checking both current and deprecated keys.
func extractProviderName(attrs []*commonpb.KeyValue) string {
	return getStringAttrMulti(attrs, AttrGenAIProviderName, AttrGenAISystem)
}

// extractModel returns the model name, preferring response model over request model.
func extractModel(attrs []*commonpb.KeyValue) string {
	return getStringAttrMulti(attrs, AttrGenAIResponseModel, AttrGenAIRequestModel)
}

// extractTokenUsage retrieves input and output token counts, checking both
// current and deprecated attribute names.
func extractTokenUsage(attrs []*commonpb.KeyValue) (inputTokens, outputTokens int64) {
	inputTokens = getIntAttrMulti(attrs, AttrGenAIUsageInput, AttrGenAIUsagePromptTokens)
	outputTokens = getIntAttrMulti(attrs, AttrGenAIUsageOutput, AttrGenAIUsageComplTokens)
	return
}

// indexedMessage is a single message extracted from OpenLLMetry indexed attributes.
type indexedMessage struct {
	index   int
	role    string
	content string
}

// extractIndexedMessages parses OpenLLMetry legacy indexed attributes.
// It handles the gen_ai.prompt.{i}.role / gen_ai.prompt.{i}.content format
// and gen_ai.completion.{i}.role / gen_ai.completion.{i}.content format.
func extractIndexedMessages(attrs []*commonpb.KeyValue, prefix string) []indexedMessage {
	roleMap := make(map[int]string)
	contentMap := make(map[int]string)

	for _, kv := range attrs {
		key := kv.GetKey()
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := key[len(prefix):]
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		switch parts[1] {
		case "role":
			roleMap[idx] = kv.GetValue().GetStringValue()
		case "content":
			contentMap[idx] = kv.GetValue().GetStringValue()
		}
	}

	return buildIndexedMessages(roleMap, contentMap)
}

// buildIndexedMessages combines role and content maps into sorted messages.
func buildIndexedMessages(roleMap, contentMap map[int]string) []indexedMessage {
	indices := make(map[int]struct{})
	for idx := range roleMap {
		indices[idx] = struct{}{}
	}
	for idx := range contentMap {
		indices[idx] = struct{}{}
	}

	msgs := make([]indexedMessage, 0, len(indices))
	for idx := range indices {
		content := contentMap[idx]
		if content == "" {
			continue
		}
		msgs = append(msgs, indexedMessage{
			index:   idx,
			role:    roleMap[idx],
			content: content,
		})
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].index < msgs[j].index })
	return msgs
}

// indexedToSessionMessages converts indexed messages to session Messages.
func indexedToSessionMessages(indexed []indexedMessage) []*session.Message {
	if len(indexed) == 0 {
		return nil
	}
	msgs := make([]*session.Message, 0, len(indexed))
	for _, im := range indexed {
		role := toMessageRole(im.role)
		if role == "" {
			role = session.RoleAssistant
		}
		msgs = append(msgs, &session.Message{
			Role:    role,
			Content: im.content,
		})
	}
	return msgs
}

// toMessageRole maps a GenAI role string to a session.MessageRole.
func toMessageRole(role string) session.MessageRole {
	switch role {
	case "user":
		return session.RoleUser
	case "assistant":
		return session.RoleAssistant
	case "system":
		return session.RoleSystem
	default:
		return ""
	}
}
