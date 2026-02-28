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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func TestGetStringAttr(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "foo", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "bar"}}},
		{Key: "empty", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: ""}}},
		{Key: "int", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
	}

	assert.Equal(t, "bar", getStringAttr(attrs, "foo"))
	assert.Equal(t, "", getStringAttr(attrs, "empty"))
	assert.Equal(t, "", getStringAttr(attrs, "int"))
	assert.Equal(t, "", getStringAttr(attrs, "missing"))
	assert.Equal(t, "", getStringAttr(nil, "foo"))
}

func TestGetIntAttr(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "count", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 100}}},
		{Key: "str", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello"}}},
	}

	assert.Equal(t, int64(100), getIntAttr(attrs, "count"))
	assert.Equal(t, int64(0), getIntAttr(attrs, "str"))
	assert.Equal(t, int64(0), getIntAttr(attrs, "missing"))
	assert.Equal(t, int64(0), getIntAttr(nil, "count"))
}

func TestGetArrayAttr(t *testing.T) {
	items := []*commonpb.AnyValue{
		{Value: &commonpb.AnyValue_StringValue{StringValue: "a"}},
		{Value: &commonpb.AnyValue_StringValue{StringValue: "b"}},
	}
	attrs := []*commonpb.KeyValue{
		{Key: "list", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: items},
		}}},
		{Key: "str", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello"}}},
	}

	result := getArrayAttr(attrs, "list")
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0].GetStringValue())

	assert.Nil(t, getArrayAttr(attrs, "str"))
	assert.Nil(t, getArrayAttr(attrs, "missing"))
	assert.Nil(t, getArrayAttr(nil, "list"))
}

func TestGetStringAttrMulti(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "b", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "second"}}},
	}

	assert.Equal(t, "second", getStringAttrMulti(attrs, "a", "b"))
	assert.Equal(t, "", getStringAttrMulti(attrs, "x", "y"))
}

func TestGetIntAttrMulti(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "old", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 50}}},
	}

	assert.Equal(t, int64(50), getIntAttrMulti(attrs, "new", "old"))
	assert.Equal(t, int64(0), getIntAttrMulti(attrs, "x", "y"))
}

func TestExtractSessionID(t *testing.T) {
	t.Run("from conversation id", func(t *testing.T) {
		spanAttrs := []*commonpb.KeyValue{
			{Key: AttrGenAIConversationID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "conv-123"}}},
		}
		assert.Equal(t, "conv-123", extractSessionID(spanAttrs, nil, nil))
	})

	t.Run("from session.id", func(t *testing.T) {
		spanAttrs := []*commonpb.KeyValue{
			{Key: AttrSessionID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess-456"}}},
		}
		assert.Equal(t, "sess-456", extractSessionID(spanAttrs, nil, nil))
	})

	t.Run("from langfuse.session.id", func(t *testing.T) {
		spanAttrs := []*commonpb.KeyValue{
			{Key: AttrLangfuseSessionID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "lf-789"}}},
		}
		assert.Equal(t, "lf-789", extractSessionID(spanAttrs, nil, nil))
	})

	t.Run("from resource session.id", func(t *testing.T) {
		resAttrs := []*commonpb.KeyValue{
			{Key: AttrSessionID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "res-sess"}}},
		}
		assert.Equal(t, "res-sess", extractSessionID(nil, resAttrs, nil))
	})

	t.Run("fallback to trace ID", func(t *testing.T) {
		traceID := []byte{0xab, 0xcd, 0xef, 0x01}
		assert.Equal(t, "abcdef01", extractSessionID(nil, nil, traceID))
	})

	t.Run("empty when nothing available", func(t *testing.T) {
		assert.Equal(t, "", extractSessionID(nil, nil, nil))
	})

	t.Run("priority order", func(t *testing.T) {
		spanAttrs := []*commonpb.KeyValue{
			{Key: AttrGenAIConversationID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "conv"}}},
			{Key: AttrSessionID, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "sess"}}},
		}
		assert.Equal(t, "conv", extractSessionID(spanAttrs, nil, []byte{0x01}))
	})
}

func TestExtractProviderName(t *testing.T) {
	t.Run("current attr", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAIProviderName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
		}
		assert.Equal(t, "openai", extractProviderName(attrs))
	})

	t.Run("deprecated gen_ai.system", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAISystem, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "anthropic"}}},
		}
		assert.Equal(t, "anthropic", extractProviderName(attrs))
	})
}

func TestExtractModel(t *testing.T) {
	t.Run("response model preferred", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAIRequestModel, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4"}}},
			{Key: AttrGenAIResponseModel, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4-0613"}}},
		}
		assert.Equal(t, "gpt-4-0613", extractModel(attrs))
	})

	t.Run("falls back to request model", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAIRequestModel, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4"}}},
		}
		assert.Equal(t, "gpt-4", extractModel(attrs))
	})
}

func TestExtractTokenUsage(t *testing.T) {
	t.Run("current attribute names", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAIUsageInput, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 150}}},
			{Key: AttrGenAIUsageOutput, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 75}}},
		}
		input, output := extractTokenUsage(attrs)
		assert.Equal(t, int64(150), input)
		assert.Equal(t, int64(75), output)
	})

	t.Run("deprecated attribute names", func(t *testing.T) {
		attrs := []*commonpb.KeyValue{
			{Key: AttrGenAIUsagePromptTokens, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 200}}},
			{Key: AttrGenAIUsageComplTokens, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 100}}},
		}
		input, output := extractTokenUsage(attrs)
		assert.Equal(t, int64(200), input)
		assert.Equal(t, int64(100), output)
	})

	t.Run("empty", func(t *testing.T) {
		input, output := extractTokenUsage(nil)
		assert.Equal(t, int64(0), input)
		assert.Equal(t, int64(0), output)
	})
}

func TestExtractIndexedMessages(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "gen_ai.prompt.0.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "system"}}},
		{Key: "gen_ai.prompt.0.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "You are helpful."}}},
		{Key: "gen_ai.prompt.1.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "user"}}},
		{Key: "gen_ai.prompt.1.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "Hello!"}}},
		{Key: "gen_ai.prompt.2.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "assistant"}}},
		// index 2 has no content — should be skipped.
		{Key: "unrelated.attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "ignored"}}},
	}

	msgs := extractIndexedMessages(attrs, AttrGenAIPromptPrefix)
	require.Len(t, msgs, 2)
	assert.Equal(t, 0, msgs[0].index)
	assert.Equal(t, "system", msgs[0].role)
	assert.Equal(t, "You are helpful.", msgs[0].content)
	assert.Equal(t, 1, msgs[1].index)
	assert.Equal(t, "user", msgs[1].role)
	assert.Equal(t, "Hello!", msgs[1].content)
}

func TestExtractIndexedMessages_Completion(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: "gen_ai.completion.0.role", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "assistant"}}},
		{Key: "gen_ai.completion.0.content", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "Here is the answer."}}},
	}

	msgs := extractIndexedMessages(attrs, AttrGenAICompletionPrefix)
	require.Len(t, msgs, 1)
	assert.Equal(t, "assistant", msgs[0].role)
	assert.Equal(t, "Here is the answer.", msgs[0].content)
}

func TestIndexedToSessionMessages(t *testing.T) {
	indexed := []indexedMessage{
		{index: 0, role: "user", content: "Hi"},
		{index: 1, role: "", content: "Response"},
	}

	msgs := indexedToSessionMessages(indexed)
	require.Len(t, msgs, 2)
	assert.Equal(t, session.RoleUser, msgs[0].Role)
	assert.Equal(t, session.RoleAssistant, msgs[1].Role) // default
}

func TestIndexedToSessionMessages_Empty(t *testing.T) {
	assert.Nil(t, indexedToSessionMessages(nil))
}

func TestOmniaAttributeConstants(t *testing.T) {
	assert.Equal(t, "omnia.agent.name", AttrOmniaAgentName)
	assert.Equal(t, "omnia.agent.namespace", AttrOmniaAgentNamespace)
	assert.Equal(t, "omnia.promptpack.name", AttrOmniaPromptPackName)
	assert.Equal(t, "omnia.promptpack.version", AttrOmniaPromptPackVersion)
	assert.Equal(t, "omnia.promptpack.namespace", AttrOmniaPromptPackNamespace)
}

func TestToMessageRole(t *testing.T) {
	assert.Equal(t, session.RoleUser, toMessageRole("user"))
	assert.Equal(t, session.RoleAssistant, toMessageRole("assistant"))
	assert.Equal(t, session.RoleSystem, toMessageRole("system"))
	assert.Equal(t, session.MessageRole(""), toMessageRole("unknown"))
	assert.Equal(t, session.MessageRole(""), toMessageRole(""))
}
