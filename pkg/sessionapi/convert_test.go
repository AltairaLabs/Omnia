package sessionapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// --- Pointer helper tests ---

func TestPtr(t *testing.T) {
	s := "hello"
	p := ptr(s)
	require.NotNil(t, p)
	assert.Equal(t, s, *p)
}

func TestDeref(t *testing.T) {
	s := "hello"
	assert.Equal(t, "hello", deref(&s))
	assert.Equal(t, "", deref[string](nil))
}

func TestDerefSlice(t *testing.T) {
	s := []string{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, derefSlice(&s))
	assert.Nil(t, derefSlice[string](nil))
}

func TestDerefMap(t *testing.T) {
	m := map[string]string{"k": "v"}
	assert.Equal(t, map[string]string{"k": "v"}, derefMap(&m))
	assert.Nil(t, derefMap[string, string](nil))
}

func TestTimePtr(t *testing.T) {
	now := time.Now()
	assert.Equal(t, &now, timePtr(now))
	assert.Nil(t, timePtr(time.Time{}))
}

func TestUuidPtr(t *testing.T) {
	id := uuid.New().String()
	p := uuidPtr(id)
	require.NotNil(t, p)
	assert.Equal(t, id, p.String())

	assert.Nil(t, uuidPtr("not-a-uuid"))
}

func TestUuidToString(t *testing.T) {
	id := uuid.New()
	u := id
	assert.Equal(t, id.String(), uuidToString(&u))
	assert.Equal(t, "", uuidToString(nil))
}

// --- Request conversion tests ---

func TestSessionToAPI(t *testing.T) {
	id := uuid.New().String()
	opts := session.CreateSessionOptions{
		AgentName:         "agent-1",
		Namespace:         "default",
		WorkspaceName:     "ws-1",
		TTL:               30 * time.Minute,
		PromptPackName:    "pp-1",
		PromptPackVersion: "v1",
	}

	result := SessionToAPI(id, opts)

	assert.Equal(t, id, result.Id.String())
	assert.Equal(t, "agent-1", deref(result.AgentName))
	assert.Equal(t, "default", deref(result.Namespace))
	assert.Equal(t, "ws-1", deref(result.WorkspaceName))
	assert.Equal(t, 1800, deref(result.TtlSeconds))
	assert.Equal(t, "pp-1", deref(result.PromptPackName))
	assert.Equal(t, "v1", deref(result.PromptPackVersion))
}

func TestSessionToAPI_Minimal(t *testing.T) {
	id := uuid.New().String()
	result := SessionToAPI(id, session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})

	assert.Equal(t, "a", deref(result.AgentName))
	assert.Nil(t, result.WorkspaceName)
	assert.Nil(t, result.TtlSeconds)
}

func TestMessageToAPI(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	msg := session.Message{
		ID:           "m1",
		Role:         session.RoleUser,
		Content:      "hello",
		Timestamp:    now,
		Metadata:     map[string]string{"key": "val"},
		InputTokens:  100,
		OutputTokens: 50,
		ToolCallID:   "tc1",
		SequenceNum:  3,
	}

	result := MessageToAPI(msg)

	assert.Equal(t, "m1", deref(result.Id))
	assert.Equal(t, User, *result.Role)
	assert.Equal(t, "hello", deref(result.Content))
	assert.Equal(t, now, *result.Timestamp)
	assert.Equal(t, map[string]string{"key": "val"}, derefMap(result.Metadata))
	assert.Equal(t, int32(100), deref(result.InputTokens))
	assert.Equal(t, int32(50), deref(result.OutputTokens))
	assert.Equal(t, "tc1", deref(result.ToolCallId))
	assert.Equal(t, int32(3), deref(result.SequenceNum))
}

func TestMessageToAPI_Minimal(t *testing.T) {
	result := MessageToAPI(session.Message{
		ID:   "m1",
		Role: session.RoleAssistant,
	})

	assert.Equal(t, "m1", deref(result.Id))
	assert.Equal(t, Assistant, *result.Role)
	assert.Nil(t, result.Metadata)
	assert.Nil(t, result.InputTokens)
	assert.Nil(t, result.ToolCallId)
}

func TestToolCallToAPI(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	tc := session.ToolCall{
		ID:           "tc1",
		SessionID:    uuid.New().String(),
		CallID:       "call-1",
		Name:         "search",
		Arguments:    map[string]any{"q": "test"},
		Result:       "found it",
		Status:       session.ToolCallStatusSuccess,
		DurationMs:   150,
		ErrorMessage: "",
		Labels:       map[string]string{"env": "test"},
		CreatedAt:    now,
	}

	result := ToolCallToAPI(tc)

	assert.Equal(t, "tc1", deref(result.Id))
	assert.Equal(t, tc.SessionID, result.SessionId.String())
	assert.Equal(t, "call-1", deref(result.CallId))
	assert.Equal(t, "search", deref(result.Name))
	assert.Equal(t, Success, *result.Status)
	assert.Equal(t, int64(150), deref(result.DurationMs))
	assert.Nil(t, result.ErrorMessage)
	assert.Equal(t, map[string]string{"env": "test"}, derefMap(result.Labels))
	require.NotNil(t, result.Arguments)
	assert.Equal(t, "test", (*result.Arguments)["q"])
	require.NotNil(t, result.Result)
	assert.Equal(t, "found it", *result.Result)
}

func TestProviderCallToAPI(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	pc := session.ProviderCall{
		ID:            "pc1",
		SessionID:     uuid.New().String(),
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-20250514",
		Status:        session.ProviderCallStatusCompleted,
		InputTokens:   1000,
		OutputTokens:  500,
		CachedTokens:  200,
		CostUSD:       0.05,
		DurationMs:    2000,
		FinishReason:  "end_turn",
		ToolCallCount: 2,
		Labels:        map[string]string{"tier": "hot"},
		CreatedAt:     now,
	}

	result := ProviderCallToAPI(pc)

	assert.Equal(t, "pc1", deref(result.Id))
	assert.Equal(t, ProviderCallStatusCompleted, *result.Status)
	assert.Equal(t, int64(1000), deref(result.InputTokens))
	assert.Equal(t, 0.05, deref(result.CostUsd))
	assert.Equal(t, "end_turn", deref(result.FinishReason))
	assert.Equal(t, int32(2), deref(result.ToolCallCount))
}

func TestStatsUpdateToAPI(t *testing.T) {
	endedAt := time.Now().Truncate(time.Second)
	u := session.SessionStatsUpdate{
		SetStatus:  session.SessionStatusCompleted,
		SetEndedAt: endedAt,
	}

	result := StatsUpdateToAPI(u)

	assert.Equal(t, SessionStatusCompleted, *result.SetStatus)
	assert.Equal(t, endedAt, *result.SetEndedAt)
}

func TestStatsUpdateToAPI_NoChange(t *testing.T) {
	result := StatsUpdateToAPI(session.SessionStatsUpdate{})

	assert.Nil(t, result.SetStatus)
	assert.Nil(t, result.SetEndedAt)
}

func TestEvalResultToAPI(t *testing.T) {
	score := 0.9
	dur := 150
	tokens := 500
	cost := 0.02
	details := json.RawMessage(`{"key":"value"}`)

	r := &api.EvalResult{
		ID:                "er1",
		SessionID:         uuid.New().String(),
		MessageID:         "m1",
		AgentName:         "agent",
		Namespace:         "ns",
		PromptPackName:    "pp",
		PromptPackVersion: "v1",
		EvalID:            "eval-1",
		EvalType:          "contains",
		Trigger:           "post_message",
		Passed:            true,
		Score:             &score,
		Details:           details,
		DurationMs:        &dur,
		JudgeTokens:       &tokens,
		JudgeCostUSD:      &cost,
		Source:            "worker",
		CreatedAt:         time.Now().Truncate(time.Second),
	}

	result := EvalResultToAPI(r)

	assert.Equal(t, "er1", deref(result.Id))
	assert.Equal(t, r.SessionID, result.SessionId.String())
	assert.Equal(t, "m1", deref(result.MessageId))
	assert.Equal(t, true, deref(result.Passed))
	assert.Equal(t, 0.9, deref(result.Score))
	assert.Equal(t, 150, deref(result.DurationMs))
	assert.Equal(t, "worker", deref(result.Source))

	// Details should have been unmarshaled
	require.NotNil(t, result.Details)
	detailsMap, ok := (*result.Details).(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", detailsMap["key"])
}

func TestEvalResultToAPI_Nil(t *testing.T) {
	result := EvalResultToAPI(nil)
	assert.Nil(t, result.Id)
}

func TestEvalResultsToAPI(t *testing.T) {
	results := []*api.EvalResult{
		{ID: "er1", EvalType: "contains"},
		{ID: "er2", EvalType: "tone"},
	}
	out := EvalResultsToAPI(results)
	require.Len(t, out, 2)
	assert.Equal(t, "er1", deref(out[0].Id))
	assert.Equal(t, "er2", deref(out[1].Id))

	assert.Nil(t, EvalResultsToAPI(nil))
}

func TestEvalListOptsToParams(t *testing.T) {
	passed := true
	opts := api.EvalResultListOpts{
		Limit:     10,
		Offset:    5,
		AgentName: "agent",
		Namespace: "ns",
		EvalID:    "eval-1",
		EvalType:  "contains",
		Passed:    &passed,
	}

	params := EvalListOptsToParams(opts)

	assert.Equal(t, 10, deref(params.Limit))
	assert.Equal(t, 5, deref(params.Offset))
	assert.Equal(t, "agent", deref(params.AgentName))
	assert.Equal(t, "ns", deref(params.Namespace))
	assert.Equal(t, "eval-1", deref(params.EvalId))
	assert.Equal(t, "contains", deref(params.EvalType))
	assert.Equal(t, true, deref(params.Passed))
}

func TestEvalListOptsToParams_Minimal(t *testing.T) {
	params := EvalListOptsToParams(api.EvalResultListOpts{})

	assert.Nil(t, params.Limit)
	assert.Nil(t, params.Offset)
	assert.Nil(t, params.AgentName)
	assert.Nil(t, params.Passed)
}

// --- Response conversion tests ---

func TestSessionFromAPI(t *testing.T) {
	id := uuid.New()
	u := id
	now := time.Now().Truncate(time.Second)
	status := SessionStatusActive

	s := &Session{
		Id:                 &u,
		AgentName:          ptr("agent"),
		Namespace:          ptr("ns"),
		CreatedAt:          &now,
		UpdatedAt:          &now,
		Status:             &status,
		MessageCount:       ptr(int32(5)),
		TotalInputTokens:   ptr(int64(1000)),
		TotalOutputTokens:  ptr(int64(500)),
		EstimatedCostUSD:   ptr(0.05),
		PromptPackName:     ptr("pp"),
		PromptPackVersion:  ptr("v1"),
		Tags:               &[]string{"tag1"},
		State:              &map[string]string{"k": "v"},
		LastMessagePreview: ptr("hello..."),
	}

	result := SessionFromAPI(s)

	require.NotNil(t, result)
	assert.Equal(t, id.String(), result.ID)
	assert.Equal(t, "agent", result.AgentName)
	assert.Equal(t, "ns", result.Namespace)
	assert.Equal(t, now, result.CreatedAt)
	assert.Equal(t, session.SessionStatusActive, result.Status)
	assert.Equal(t, int32(5), result.MessageCount)
	assert.Equal(t, int64(1000), result.TotalInputTokens)
	assert.Equal(t, 0.05, result.EstimatedCostUSD)
	assert.Equal(t, []string{"tag1"}, result.Tags)
	assert.Equal(t, map[string]string{"k": "v"}, result.State)
}

func TestSessionFromAPI_Nil(t *testing.T) {
	assert.Nil(t, SessionFromAPI(nil))
}

func TestMessageFromAPI(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	role := User
	m := Message{
		Id:           ptr("m1"),
		Role:         &role,
		Content:      ptr("hello"),
		Timestamp:    &now,
		InputTokens:  ptr(int32(100)),
		OutputTokens: ptr(int32(50)),
		ToolCallId:   ptr("tc1"),
		SequenceNum:  ptr(int32(1)),
		Metadata:     &map[string]string{"k": "v"},
	}

	result := MessageFromAPI(m)

	assert.Equal(t, "m1", result.ID)
	assert.Equal(t, session.RoleUser, result.Role)
	assert.Equal(t, "hello", result.Content)
	assert.Equal(t, now, result.Timestamp)
	assert.Equal(t, int32(100), result.InputTokens)
	assert.Equal(t, "tc1", result.ToolCallID)
	assert.Equal(t, map[string]string{"k": "v"}, result.Metadata)
}

func TestMessagesFromAPI(t *testing.T) {
	role := User
	msgs := &[]Message{
		{Id: ptr("m1"), Role: &role, Content: ptr("hi")},
		{Id: ptr("m2"), Role: &role, Content: ptr("there")},
	}

	result := MessagesFromAPI(msgs)

	require.Len(t, result, 2)
	assert.Equal(t, "m1", result[0].ID)
	assert.Equal(t, "m2", result[1].ID)

	assert.Nil(t, MessagesFromAPI(nil))
}

func TestToolCallFromAPI(t *testing.T) {
	id := uuid.New()
	u := id
	now := time.Now().Truncate(time.Second)
	status := Success
	args := map[string]any{"q": "test"}
	var result any = "found"

	tc := ToolCall{
		Id:         ptr("tc1"),
		SessionId:  &u,
		CallId:     ptr("call-1"),
		Name:       ptr("search"),
		Arguments:  &args,
		Result:     &result,
		Status:     &status,
		DurationMs: ptr(int64(100)),
		Labels:     &map[string]string{"env": "test"},
		CreatedAt:  &now,
	}

	out := ToolCallFromAPI(tc)

	assert.Equal(t, "tc1", out.ID)
	assert.Equal(t, id.String(), out.SessionID)
	assert.Equal(t, "search", out.Name)
	assert.Equal(t, session.ToolCallStatusSuccess, out.Status)
	assert.Equal(t, "test", out.Arguments["q"])
	assert.Equal(t, "found", out.Result)
	assert.Equal(t, int64(100), out.DurationMs)
}

func TestToolCallsFromAPI(t *testing.T) {
	status := Pending
	tcs := []ToolCall{
		{Id: ptr("tc1"), Name: ptr("search"), Status: &status},
		{Id: ptr("tc2"), Name: ptr("calc"), Status: &status},
	}

	result := ToolCallsFromAPI(tcs)
	require.Len(t, result, 2)
	assert.Equal(t, "tc1", result[0].ID)
	assert.Equal(t, "tc2", result[1].ID)

	assert.Nil(t, ToolCallsFromAPI(nil))
}

func TestProviderCallFromAPI(t *testing.T) {
	id := uuid.New()
	u := id
	now := time.Now().Truncate(time.Second)
	status := ProviderCallStatusCompleted

	pc := ProviderCall{
		Id:            ptr("pc1"),
		SessionId:     &u,
		Provider:      ptr("anthropic"),
		Model:         ptr("claude-sonnet-4-20250514"),
		Status:        &status,
		InputTokens:   ptr(int64(1000)),
		OutputTokens:  ptr(int64(500)),
		CostUsd:       ptr(0.05),
		DurationMs:    ptr(int64(2000)),
		FinishReason:  ptr("end_turn"),
		ToolCallCount: ptr(int32(2)),
		Labels:        &map[string]string{"tier": "hot"},
		CreatedAt:     &now,
	}

	out := ProviderCallFromAPI(pc)

	assert.Equal(t, "pc1", out.ID)
	assert.Equal(t, id.String(), out.SessionID)
	assert.Equal(t, "anthropic", out.Provider)
	assert.Equal(t, session.ProviderCallStatusCompleted, out.Status)
	assert.Equal(t, int64(1000), out.InputTokens)
	assert.Equal(t, 0.05, out.CostUSD)
	assert.Equal(t, "end_turn", out.FinishReason)
}

func TestProviderCallsFromAPI(t *testing.T) {
	status := ProviderCallStatusPending
	pcs := []ProviderCall{
		{Id: ptr("pc1"), Status: &status},
		{Id: ptr("pc2"), Status: &status},
	}

	result := ProviderCallsFromAPI(pcs)
	require.Len(t, result, 2)
	assert.Nil(t, ProviderCallsFromAPI(nil))
}

func TestEvalResultFromAPI(t *testing.T) {
	id := uuid.New()
	u := id
	now := time.Now().Truncate(time.Second)

	var details any = map[string]any{"key": "value"}

	r := EvalResult{
		Id:                ptr("er1"),
		SessionId:         &u,
		MessageId:         ptr("m1"),
		AgentName:         ptr("agent"),
		Namespace:         ptr("ns"),
		PromptpackName:    ptr("pp"),
		PromptpackVersion: ptr("v1"),
		EvalId:            ptr("eval-1"),
		EvalType:          ptr("contains"),
		Trigger:           ptr("post_message"),
		Passed:            ptr(true),
		Score:             ptr(0.9),
		Details:           &details,
		DurationMs:        ptr(150),
		JudgeTokens:       ptr(500),
		JudgeCostUsd:      ptr(0.02),
		Source:            ptr("worker"),
		CreatedAt:         &now,
	}

	out := EvalResultFromAPI(r)

	require.NotNil(t, out)
	assert.Equal(t, "er1", out.ID)
	assert.Equal(t, id.String(), out.SessionID)
	assert.Equal(t, "m1", out.MessageID)
	assert.True(t, out.Passed)
	assert.Equal(t, 0.9, *out.Score)
	assert.Equal(t, "worker", out.Source)
	assert.Equal(t, now, out.CreatedAt)

	// Details should round-trip through json.Marshal
	require.NotNil(t, out.Details)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out.Details, &parsed))
	assert.Equal(t, "value", parsed["key"])
}

func TestEvalResultsFromAPI(t *testing.T) {
	results := &[]EvalResult{
		{Id: ptr("er1")},
		{Id: ptr("er2")},
	}

	out := EvalResultsFromAPI(results)
	require.Len(t, out, 2)
	assert.Equal(t, "er1", out[0].ID)

	assert.Nil(t, EvalResultsFromAPI(nil))
}

// --- Round-trip tests ---

func TestMessageRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := session.Message{
		ID:           "m1",
		Role:         session.RoleUser,
		Content:      "hello world",
		Timestamp:    now,
		Metadata:     map[string]string{"key": "val"},
		InputTokens:  100,
		OutputTokens: 50,
		ToolCallID:   "tc1",
		SequenceNum:  3,
	}

	apiMsg := MessageToAPI(original)
	roundTripped := MessageFromAPI(apiMsg)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.Role, roundTripped.Role)
	assert.Equal(t, original.Content, roundTripped.Content)
	assert.Equal(t, original.Timestamp, roundTripped.Timestamp)
	assert.Equal(t, original.Metadata, roundTripped.Metadata)
	assert.Equal(t, original.InputTokens, roundTripped.InputTokens)
	assert.Equal(t, original.OutputTokens, roundTripped.OutputTokens)
	assert.Equal(t, original.ToolCallID, roundTripped.ToolCallID)
	assert.Equal(t, original.SequenceNum, roundTripped.SequenceNum)
}

func TestToolCallRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := session.ToolCall{
		ID:         "tc1",
		SessionID:  uuid.New().String(),
		CallID:     "call-1",
		Name:       "search",
		Arguments:  map[string]any{"q": "test"},
		Status:     session.ToolCallStatusSuccess,
		DurationMs: 150,
		Labels:     map[string]string{"env": "test"},
		CreatedAt:  now,
	}

	apiTC := ToolCallToAPI(original)
	roundTripped := ToolCallFromAPI(apiTC)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SessionID, roundTripped.SessionID)
	assert.Equal(t, original.CallID, roundTripped.CallID)
	assert.Equal(t, original.Name, roundTripped.Name)
	assert.Equal(t, original.Status, roundTripped.Status)
	assert.Equal(t, original.DurationMs, roundTripped.DurationMs)
	assert.Equal(t, original.Labels, roundTripped.Labels)
	assert.Equal(t, original.CreatedAt, roundTripped.CreatedAt)
	assert.Equal(t, "test", roundTripped.Arguments["q"])
}

func TestProviderCallRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := session.ProviderCall{
		ID:            "pc1",
		SessionID:     uuid.New().String(),
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-20250514",
		Status:        session.ProviderCallStatusCompleted,
		InputTokens:   1000,
		OutputTokens:  500,
		CachedTokens:  200,
		CostUSD:       0.05,
		DurationMs:    2000,
		FinishReason:  "end_turn",
		ToolCallCount: 2,
		Labels:        map[string]string{"tier": "hot"},
		CreatedAt:     now,
	}

	apiPC := ProviderCallToAPI(original)
	roundTripped := ProviderCallFromAPI(apiPC)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SessionID, roundTripped.SessionID)
	assert.Equal(t, original.Provider, roundTripped.Provider)
	assert.Equal(t, original.Model, roundTripped.Model)
	assert.Equal(t, original.Status, roundTripped.Status)
	assert.Equal(t, original.InputTokens, roundTripped.InputTokens)
	assert.Equal(t, original.OutputTokens, roundTripped.OutputTokens)
	assert.Equal(t, original.CachedTokens, roundTripped.CachedTokens)
	assert.Equal(t, original.CostUSD, roundTripped.CostUSD)
	assert.Equal(t, original.DurationMs, roundTripped.DurationMs)
	assert.Equal(t, original.FinishReason, roundTripped.FinishReason)
	assert.Equal(t, original.ToolCallCount, roundTripped.ToolCallCount)
	assert.Equal(t, original.Labels, roundTripped.Labels)
}

func TestEvalResultDetailsRoundTrip(t *testing.T) {
	details := json.RawMessage(`{"nested":{"key":"value"},"count":42}`)
	score := 0.8
	original := &api.EvalResult{
		ID:        "er1",
		SessionID: uuid.New().String(),
		EvalID:    "eval-1",
		EvalType:  "contains",
		Passed:    true,
		Score:     &score,
		Details:   details,
		Source:    "worker",
	}

	apiResult := EvalResultToAPI(original)
	roundTripped := EvalResultFromAPI(apiResult)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.Passed, roundTripped.Passed)
	assert.Equal(t, *original.Score, *roundTripped.Score)
	assert.Equal(t, original.Source, roundTripped.Source)

	// Verify details round-trip
	require.NotNil(t, roundTripped.Details)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(roundTripped.Details, &parsed))
	assert.Equal(t, float64(42), parsed["count"])
	nested, ok := parsed["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", nested["key"])
}

func TestStatsUpdateRoundTrip_EmptyStatus(t *testing.T) {
	u := session.SessionStatsUpdate{}

	result := StatsUpdateToAPI(u)

	assert.Nil(t, result.SetStatus, "empty status should produce nil")
	assert.Nil(t, result.SetEndedAt, "zero time should produce nil")
}
