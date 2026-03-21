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

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// uid generates a deterministic UUID from a numeric index.
func uid(n int) string {
	return fmt.Sprintf("00000000-0000-4000-a000-%012d", n)
}

// TestConversationStats_FullConversationWithTools simulates a realistic 2-turn
// conversation between a user and an agent that uses tools. It exercises the
// exact code paths that the runtime event store and facade recording writer hit
// in production, and verifies that session-level counters (messageCount,
// toolCallCount, totalInputTokens, totalOutputTokens, estimatedCostUSD) are
// correct after the conversation completes.
//
// Conversation being simulated:
//
//	Turn 1: User asks "What's the weather in NYC?"
//	  → Provider call 1 (LLM decides to call weather tool)
//	  → Tool call: get_weather(location="NYC")  [server-side]
//	  → Tool result recorded as a message
//	  → Provider call 2 (LLM generates final answer with tool result)
//	  → Assistant replies "It's 72°F in NYC"
//
//	Turn 2: User asks "Convert that to Celsius"
//	  → Provider call 3 (LLM answers directly, no tools)
//	  → Assistant replies "That's about 22°C"
//
// Expected final counters:
//
//	messageCount:      4   (2 user + 2 assistant messages; tool_call/tool_result don't count)
//	toolCallCount:     1   (1 tool call via RecordToolCall)
//	totalInputTokens:  850 (200 + 350 + 300 from 3 provider calls)
//	totalOutputTokens: 190 (50 + 80 + 60 from 3 provider calls)
//	estimatedCostUSD:  0.01250 (0.003 + 0.005 + 0.0045 from 3 provider calls)
func TestConversationStats_FullConversationWithTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// --- Create session ---
	sess := makeSession("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c01", now)
	require.NoError(t, p.CreateSession(ctx, sess))

	// Verify initial counters are all zero.
	got, err := p.GetSession(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(0), got.MessageCount, "initial messageCount")
	assert.Equal(t, int32(0), got.ToolCallCount, "initial toolCallCount")
	assert.Equal(t, int64(0), got.TotalInputTokens, "initial totalInputTokens")
	assert.Equal(t, int64(0), got.TotalOutputTokens, "initial totalOutputTokens")
	assert.InDelta(t, 0.0, got.EstimatedCostUSD, 1e-9, "initial estimatedCostUSD")

	// =====================================================================
	// TURN 1: "What's the weather in NYC?"
	// =====================================================================

	// 1a. User message (written by facade processRegularMessage)
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:        uid(10),
		Role:      session.RoleUser,
		Content:   "What's the weather in NYC?",
		Timestamp: now.Add(1 * time.Second),
	}))

	// 1b. Provider call 1: LLM decides to invoke the weather tool
	//     (written by runtime event store: convertProviderCallCompleted)
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:            uid(20),
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-20250514",
		Status:        session.ProviderCallStatusCompleted,
		InputTokens:   200,
		OutputTokens:  50,
		CostUSD:       0.003,
		DurationMs:    450,
		FinishReason:  "tool_use",
		ToolCallCount: 1,
		CreatedAt:     now.Add(2 * time.Second),
	}))

	// 1c. Tool call started: get_weather (runtime: convertToolCallStarted)
	//     Status=pending → increments tool_call_count.
	require.NoError(t, p.RecordToolCall(ctx, sess.ID, &session.ToolCall{
		ID:        uid(30),
		CallID:    "call_abc123",
		Name:      "get_weather",
		Arguments: map[string]any{"location": "NYC"},
		Status:    session.ToolCallStatusPending,
		CreatedAt: now.Add(3 * time.Second),
	}))

	// 1d. Tool call completed (runtime: convertToolCallCompleted)
	//     Separate row (new ID), same CallID. Status=success → does NOT increment.
	require.NoError(t, p.RecordToolCall(ctx, sess.ID, &session.ToolCall{
		ID:         uid(31),
		CallID:     "call_abc123",
		Name:       "get_weather",
		Result:     "72°F, sunny",
		Status:     session.ToolCallStatusSuccess,
		DurationMs: 320,
		CreatedAt:  now.Add(3500 * time.Millisecond),
	}))

	// 1e. Tool result message (written by runtime event store: convertMessageCreated
	//     with ToolResult data — has ToolCallID so messageCount is NOT incremented)
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:         uid(11),
		Role:       session.RoleSystem,
		Content:    "72°F, sunny",
		ToolCallID: "call_abc123",
		Timestamp:  now.Add(4 * time.Second),
		Metadata:   map[string]string{"type": "tool_result", "tool_name": "get_weather"},
	}))

	// 1f. Provider call 2: LLM generates final answer incorporating tool result
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(21),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  350,
		OutputTokens: 80,
		CostUSD:      0.005,
		DurationMs:   620,
		FinishReason: "end_turn",
		CreatedAt:    now.Add(5 * time.Second),
	}))

	// 1g. Assistant message — "It's 72°F in NYC"
	//     (written by both facade recordDone and runtime convertMessageCreated;
	//      tokens on the message are for historical queries only — session counters
	//      come from RecordProviderCall above)
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:           uid(12),
		Role:         session.RoleAssistant,
		Content:      "It's currently 72°F and sunny in New York City.",
		Timestamp:    now.Add(6 * time.Second),
		InputTokens:  350,
		OutputTokens: 80,
		CostUSD:      0.005,
	}))

	// --- Verify counters after turn 1 ---
	got, err = p.GetSession(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), got.MessageCount,
		"turn 1 messageCount: 1 user + 1 assistant (tool_result has ToolCallID, doesn't count)")
	assert.Equal(t, int32(1), got.ToolCallCount,
		"turn 1 toolCallCount: 1 tool call (only pending event counted)")
	assert.Equal(t, int64(550), got.TotalInputTokens,
		"turn 1 totalInputTokens: 200 (pc-1) + 350 (pc-2)")
	assert.Equal(t, int64(130), got.TotalOutputTokens,
		"turn 1 totalOutputTokens: 50 (pc-1) + 80 (pc-2)")
	assert.InDelta(t, 0.008, got.EstimatedCostUSD, 1e-9,
		"turn 1 estimatedCostUSD: 0.003 (pc-1) + 0.005 (pc-2)")

	// =====================================================================
	// TURN 2: "Convert that to Celsius" (no tool call)
	// =====================================================================

	// 2a. User message
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:        uid(13),
		Role:      session.RoleUser,
		Content:   "Convert that to Celsius",
		Timestamp: now.Add(10 * time.Second),
	}))

	// 2b. Provider call 3: LLM answers directly
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(22),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  300,
		OutputTokens: 60,
		CostUSD:      0.0045,
		DurationMs:   380,
		FinishReason: "end_turn",
		CreatedAt:    now.Add(11 * time.Second),
	}))

	// 2c. Assistant message
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:           uid(14),
		Role:         session.RoleAssistant,
		Content:      "72°F is approximately 22.2°C.",
		Timestamp:    now.Add(12 * time.Second),
		InputTokens:  300,
		OutputTokens: 60,
		CostUSD:      0.0045,
	}))

	// =====================================================================
	// FINAL VERIFICATION
	// =====================================================================
	got, err = p.GetSession(ctx, sess.ID)
	require.NoError(t, err)

	// --- Message count ---
	// 2 user messages + 2 assistant messages = 4
	// (tool_result message has ToolCallID → messageIncr=0, not counted)
	assert.Equal(t, int32(4), got.MessageCount,
		"final messageCount: 2 user + 2 assistant = 4")

	// --- Tool call count ---
	// 1 tool call: get_weather (started + completed are separate rows, only pending counted)
	assert.Equal(t, int32(1), got.ToolCallCount,
		"final toolCallCount: 1 tool call (only pending event counted)")

	// --- Input tokens ---
	// Derived exclusively from provider_calls: 200 + 350 + 300 = 850
	// (message-level tokens are stored for history but do NOT affect session counters)
	assert.Equal(t, int64(850), got.TotalInputTokens,
		"final totalInputTokens: 200 (pc-1) + 350 (pc-2) + 300 (pc-3) = 850")

	// --- Output tokens ---
	// Derived exclusively from provider_calls: 50 + 80 + 60 = 190
	assert.Equal(t, int64(190), got.TotalOutputTokens,
		"final totalOutputTokens: 50 (pc-1) + 80 (pc-2) + 60 (pc-3) = 190")

	// --- Cost ---
	// Derived exclusively from provider_calls: 0.003 + 0.005 + 0.0045 = 0.0125
	assert.InDelta(t, 0.0125, got.EstimatedCostUSD, 1e-9,
		"final estimatedCostUSD: 0.003 (pc-1) + 0.005 (pc-2) + 0.0045 (pc-3) = 0.0125")

	// --- Cross-check: verify the child tables have the right data ---

	// Tool calls — 2 rows for 1 logical call (started + completed), linked by CallID
	toolCalls, err := p.GetToolCalls(ctx, sess.ID, providers.PaginationOpts{})
	require.NoError(t, err)
	require.Len(t, toolCalls, 2, "2 tool call rows: started + completed")
	assert.Equal(t, "get_weather", toolCalls[0].Name)
	assert.Equal(t, session.ToolCallStatusPending, toolCalls[0].Status, "first row is started")
	assert.Equal(t, session.ToolCallStatusSuccess, toolCalls[1].Status, "second row is completed")
	assert.Equal(t, toolCalls[0].CallID, toolCalls[1].CallID, "same CallID links both rows")
	assert.Equal(t, int64(320), toolCalls[1].DurationMs)

	// Provider calls
	providerCalls, err := p.GetProviderCalls(ctx, sess.ID, providers.PaginationOpts{})
	require.NoError(t, err)
	require.Len(t, providerCalls, 3, "should have exactly 3 provider call records")

	var totalInput, totalOutput int64
	var totalCost float64
	for _, pc := range providerCalls {
		totalInput += pc.InputTokens
		totalOutput += pc.OutputTokens
		totalCost += pc.CostUSD
	}
	assert.Equal(t, int64(850), totalInput,
		"sum of provider_calls.input_tokens matches session.total_input_tokens")
	assert.Equal(t, int64(190), totalOutput,
		"sum of provider_calls.output_tokens matches session.total_output_tokens")
	assert.InDelta(t, 0.0125, totalCost, 1e-9,
		"sum of provider_calls.cost_usd matches session.estimated_cost_usd")

	// Messages (all 5: 2 user + 1 tool_result + 2 assistant)
	msgs, err := p.GetMessages(ctx, sess.ID, defaultMsgOpts)
	require.NoError(t, err)
	require.Len(t, msgs, 5, "should have 5 message records total")

	var userCount, assistantCount, toolResultCount int
	for _, m := range msgs {
		switch {
		case m.Role == session.RoleUser:
			userCount++
		case m.Role == session.RoleAssistant:
			assistantCount++
		case m.Metadata["type"] == "tool_result":
			toolResultCount++
		}
	}
	assert.Equal(t, 2, userCount, "2 user messages")
	assert.Equal(t, 2, assistantCount, "2 assistant messages")
	assert.Equal(t, 1, toolResultCount, "1 tool_result message")
}

// TestConversationStats_MultipleToolCalls verifies that multiple tool calls in
// a single turn each increment tool_call_count exactly once (on the pending
// event), and that completed events are separate rows that don't re-count.
func TestConversationStats_MultipleToolCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	sess := makeSession("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c02", now)
	require.NoError(t, p.CreateSession(ctx, sess))

	// 3 tool calls. Each gets started + completed. ClientToolRequest for tool C
	// goes to runtime_events (not tool_calls), so it doesn't affect the count.
	type toolDef struct {
		startedID string
		doneID    string
		callID    string
		name      string
	}
	tools := []toolDef{
		{uid(40), uid(43), "call_a", "search"},
		{uid(41), uid(44), "call_b", "calculator"},
		{uid(42), uid(45), "call_c", "get_location"},
	}

	for i, tc := range tools {
		ts := now.Add(time.Duration(i) * time.Second)
		// Started (pending) → increments tool_call_count
		require.NoError(t, p.RecordToolCall(ctx, sess.ID, &session.ToolCall{
			ID:        tc.startedID,
			CallID:    tc.callID,
			Name:      tc.name,
			Status:    session.ToolCallStatusPending,
			CreatedAt: ts,
		}))
		// Completed → separate row, does NOT increment
		require.NoError(t, p.RecordToolCall(ctx, sess.ID, &session.ToolCall{
			ID:         tc.doneID,
			CallID:     tc.callID,
			Name:       tc.name,
			Status:     session.ToolCallStatusSuccess,
			DurationMs: 100,
			CreatedAt:  ts.Add(500 * time.Millisecond),
		}))
	}

	got, err := p.GetSession(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(3), got.ToolCallCount,
		"3 logical tool calls: only pending events counted")

	// 6 rows total: 3 started + 3 completed
	toolCalls, err := p.GetToolCalls(ctx, sess.ID, providers.PaginationOpts{})
	require.NoError(t, err)
	require.Len(t, toolCalls, 6, "6 rows: 3 started + 3 completed")
}

// TestConversationStats_FailedProviderCallDoesNotAddTokens verifies that a
// failed provider call is recorded but does NOT add tokens/cost to the session.
func TestConversationStats_FailedProviderCallDoesNotAddTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	sess := makeSession("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c03", now)
	require.NoError(t, p.CreateSession(ctx, sess))

	// Completed call — adds tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(50),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  100,
		OutputTokens: 40,
		CostUSD:      0.002,
		CreatedAt:    now,
	}))

	// Failed call — should NOT add tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(51),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusFailed,
		ErrorMessage: "rate limit exceeded",
		CreatedAt:    now.Add(time.Second),
	}))

	got, err := p.GetSession(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), got.TotalInputTokens,
		"only completed call's tokens counted")
	assert.Equal(t, int64(40), got.TotalOutputTokens,
		"only completed call's tokens counted")
	assert.InDelta(t, 0.002, got.EstimatedCostUSD, 1e-9,
		"only completed call's cost counted")

	// Both rows exist.
	calls, err := p.GetProviderCalls(ctx, sess.ID, providers.PaginationOpts{})
	require.NoError(t, err)
	require.Len(t, calls, 2, "2 rows: 1 completed + 1 failed")
}

// TestConversationStats_NonAgentSourceDoesNotAddTokens verifies that provider
// calls with source "judge" or "selfplay" are recorded but do NOT increment
// the session-level token/cost counters. Only agent calls (source="" or "agent")
// should inflate session totals.
func TestConversationStats_NonAgentSourceDoesNotAddTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	sess := makeSession("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c10", now)
	require.NoError(t, p.CreateSession(ctx, sess))

	// Agent call (empty source) — adds tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(60),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  200,
		OutputTokens: 80,
		CostUSD:      0.004,
		CreatedAt:    now,
	}))

	// Judge call — should NOT add tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(61),
		Provider:     "openai",
		Model:        "gpt-4o",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  50,
		OutputTokens: 100,
		CostUSD:      0.003,
		Source:       "judge",
		CreatedAt:    now.Add(time.Second),
	}))

	// Self-play call — should NOT add tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(62),
		Provider:     "openai",
		Model:        "gpt-4o",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  75,
		OutputTokens: 120,
		CostUSD:      0.005,
		Source:       "selfplay",
		CreatedAt:    now.Add(2 * time.Second),
	}))

	// Explicit "agent" source — SHOULD add tokens/cost.
	require.NoError(t, p.RecordProviderCall(ctx, sess.ID, &session.ProviderCall{
		ID:           uid(63),
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		Status:       session.ProviderCallStatusCompleted,
		InputTokens:  100,
		OutputTokens: 40,
		CostUSD:      0.002,
		Source:       "agent",
		CreatedAt:    now.Add(3 * time.Second),
	}))

	got, err := p.GetSession(ctx, sess.ID)
	require.NoError(t, err)

	// Only agent calls: 200 + 100 = 300 input, 80 + 40 = 120 output, 0.004 + 0.002 = 0.006 cost
	assert.Equal(t, int64(300), got.TotalInputTokens,
		"only agent source calls counted: 200 + 100")
	assert.Equal(t, int64(120), got.TotalOutputTokens,
		"only agent source calls counted: 80 + 40")
	assert.InDelta(t, 0.006, got.EstimatedCostUSD, 1e-9,
		"only agent source calls counted: 0.004 + 0.002")

	// All 4 rows exist.
	calls, err := p.GetProviderCalls(ctx, sess.ID, providers.PaginationOpts{})
	require.NoError(t, err)
	require.Len(t, calls, 4, "4 rows: 2 agent + 1 judge + 1 selfplay")

	// Verify source field is persisted and readable.
	assert.Equal(t, "", calls[0].Source, "first call: empty source (agent)")
	assert.Equal(t, "judge", calls[1].Source, "second call: judge")
	assert.Equal(t, "selfplay", calls[2].Source, "third call: selfplay")
	assert.Equal(t, "agent", calls[3].Source, "fourth call: explicit agent")
}

// TestConversationStats_AppendMessageDoesNotAffectTokenCounters verifies that
// messages with token/cost data (written by both facade and runtime) do NOT
// inflate session-level counters. Only RecordProviderCall updates those.
func TestConversationStats_AppendMessageDoesNotAffectTokenCounters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	sess := makeSession("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c04", now)
	require.NoError(t, p.CreateSession(ctx, sess))

	// Append a message_updated system message with token data (runtime event store path)
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:           uid(60),
		Role:         session.RoleSystem,
		Content:      `{"inputTokens":500,"outputTokens":100,"totalCost":0.01}`,
		Timestamp:    now,
		InputTokens:  500,
		OutputTokens: 100,
		CostUSD:      0.01,
		Metadata:     map[string]string{"type": "message_updated", "source": "runtime"},
	}))

	// Append an assistant message with token data (facade recordDone path)
	require.NoError(t, p.AppendMessage(ctx, sess.ID, &session.Message{
		ID:           uid(61),
		Role:         session.RoleAssistant,
		Content:      "Hello!",
		Timestamp:    now.Add(time.Second),
		InputTokens:  500,
		OutputTokens: 100,
		CostUSD:      0.01,
	}))

	got, err := p.GetSession(ctx, sess.ID)
	require.NoError(t, err)

	// Session counters must be 0 — token/cost comes from RecordProviderCall only.
	assert.Equal(t, int64(0), got.TotalInputTokens,
		"AppendMessage must not update totalInputTokens")
	assert.Equal(t, int64(0), got.TotalOutputTokens,
		"AppendMessage must not update totalOutputTokens")
	assert.InDelta(t, 0.0, got.EstimatedCostUSD, 1e-9,
		"AppendMessage must not update estimatedCostUSD")

	// But the message rows themselves should still have the data.
	msgs, err := p.GetMessages(ctx, sess.ID, defaultMsgOpts)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, int32(500), msgs[0].InputTokens, "message row preserves inputTokens")
	assert.Equal(t, int32(100), msgs[0].OutputTokens, "message row preserves outputTokens")
}

// defaultMsgOpts is a zero-value query opts for GetMessages (no filtering).
var defaultMsgOpts = providers.MessageQueryOpts{}
