package sessionapi

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// --- Pointer helpers ---

func ptr[T any](v T) *T { return &v }

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func derefMap[K comparable, V any](p *map[K]V) map[K]V {
	if p == nil {
		return nil
	}
	return *p
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func uuidPtr(s string) *openapi_types.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &u
}

func uuidToString(u *openapi_types.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

// --- Request conversions (internal → generated) ---

// SessionToAPI converts internal CreateSessionOptions to a generated CreateSessionRequest.
func SessionToAPI(id string, opts session.CreateSessionOptions) CreateSessionRequest {
	req := CreateSessionRequest{
		Id:                uuidPtr(id),
		AgentName:         ptr(opts.AgentName),
		Namespace:         ptr(opts.Namespace),
		PromptPackName:    ptr(opts.PromptPackName),
		PromptPackVersion: ptr(opts.PromptPackVersion),
	}
	if opts.WorkspaceName != "" {
		req.WorkspaceName = ptr(opts.WorkspaceName)
	}
	if opts.TTL > 0 {
		req.TtlSeconds = ptr(int(opts.TTL.Seconds()))
	}
	return req
}

// MessageToAPI converts an internal Message to a generated Message.
func MessageToAPI(m session.Message) Message {
	role := MessageRole(m.Role)
	msg := Message{
		Id:           ptr(m.ID),
		Role:         &role,
		Content:      ptr(m.Content),
		Timestamp:    timePtr(m.Timestamp),
		InputTokens:  ptrNonZero(m.InputTokens),
		OutputTokens: ptrNonZero(m.OutputTokens),
		ToolCallId:   ptrNonEmpty(m.ToolCallID),
		SequenceNum:  ptrNonZero(m.SequenceNum),
	}
	if m.Metadata != nil {
		msg.Metadata = &m.Metadata
	}
	return msg
}

// ToolCallToAPI converts an internal ToolCall to a generated ToolCall.
func ToolCallToAPI(tc session.ToolCall) ToolCall {
	status := ToolCallStatus(tc.Status)
	exec := ToolCallExecution(tc.Execution)
	out := ToolCall{
		Id:           ptr(tc.ID),
		SessionId:    uuidPtr(tc.SessionID),
		CallId:       ptr(tc.CallID),
		Name:         ptr(tc.Name),
		Status:       &status,
		DurationMs:   ptrNonZeroInt64(tc.DurationMs),
		ErrorMessage: ptrNonEmpty(tc.ErrorMessage),
		CreatedAt:    timePtr(tc.CreatedAt),
	}
	if tc.Execution != "" {
		out.Execution = &exec
	}
	if tc.Arguments != nil {
		args := map[string]any{}
		maps.Copy(args, tc.Arguments)
		out.Arguments = &args
	}
	if tc.Result != nil {
		out.Result = &tc.Result
	}
	if tc.Labels != nil {
		out.Labels = &tc.Labels
	}
	return out
}

// ProviderCallToAPI converts an internal ProviderCall to a generated ProviderCall.
func ProviderCallToAPI(pc session.ProviderCall) ProviderCall {
	status := ProviderCallStatus(pc.Status)
	out := ProviderCall{
		Id:            ptr(pc.ID),
		SessionId:     uuidPtr(pc.SessionID),
		Provider:      ptr(pc.Provider),
		Model:         ptr(pc.Model),
		Status:        &status,
		InputTokens:   ptrNonZeroInt64(pc.InputTokens),
		OutputTokens:  ptrNonZeroInt64(pc.OutputTokens),
		CachedTokens:  ptrNonZeroInt64(pc.CachedTokens),
		DurationMs:    ptrNonZeroInt64(pc.DurationMs),
		FinishReason:  ptrNonEmpty(pc.FinishReason),
		ToolCallCount: ptrNonZero(pc.ToolCallCount),
		ErrorMessage:  ptrNonEmpty(pc.ErrorMessage),
		CreatedAt:     timePtr(pc.CreatedAt),
	}
	if pc.CostUSD != 0 {
		out.CostUsd = &pc.CostUSD
	}
	if pc.Labels != nil {
		out.Labels = &pc.Labels
	}
	return out
}

// StatsUpdateToAPI converts an internal SessionStatsUpdate to a generated SessionStatsUpdate.
// Only SetStatus and SetEndedAt are populated — counter fields (Add*) are auto-derived
// from AppendMessage and are no longer set externally.
func StatsUpdateToAPI(u session.SessionStatsUpdate) SessionStatsUpdate {
	out := SessionStatsUpdate{}
	if u.SetStatus != "" {
		status := SessionStatus(u.SetStatus)
		out.SetStatus = &status
	}
	if !u.SetEndedAt.IsZero() {
		out.SetEndedAt = &u.SetEndedAt
	}
	return out
}

// EvalResultToAPI converts an internal EvalResult to a generated EvalResult.
func EvalResultToAPI(r *api.EvalResult) EvalResult {
	if r == nil {
		return EvalResult{}
	}
	out := EvalResult{
		Id:                ptrNonEmpty(r.ID),
		SessionId:         uuidPtr(r.SessionID),
		MessageId:         ptrNonEmpty(r.MessageID),
		AgentName:         ptrNonEmpty(r.AgentName),
		Namespace:         ptrNonEmpty(r.Namespace),
		PromptpackName:    ptrNonEmpty(r.PromptPackName),
		PromptpackVersion: ptrNonEmpty(r.PromptPackVersion),
		EvalId:            ptrNonEmpty(r.EvalID),
		EvalType:          ptrNonEmpty(r.EvalType),
		Trigger:           ptrNonEmpty(r.Trigger),
		Passed:            &r.Passed,
		Score:             r.Score,
		DurationMs:        r.DurationMs,
		JudgeTokens:       r.JudgeTokens,
		JudgeCostUsd:      r.JudgeCostUSD,
		Source:            ptrNonEmpty(r.Source),
		CreatedAt:         timePtr(r.CreatedAt),
	}
	if r.Details != nil {
		var v any
		if err := json.Unmarshal(r.Details, &v); err == nil {
			out.Details = &v
		}
	}
	return out
}

// EvalResultsToAPI converts a slice of internal EvalResults to generated EvalResults.
func EvalResultsToAPI(results []*api.EvalResult) []EvalResult {
	if results == nil {
		return nil
	}
	out := make([]EvalResult, len(results))
	for i, r := range results {
		out[i] = EvalResultToAPI(r)
	}
	return out
}

// EvalListOptsToParams converts internal EvalResultListOpts to generated ListEvalResultsParams.
func EvalListOptsToParams(opts api.EvalResultListOpts) ListEvalResultsParams {
	params := ListEvalResultsParams{
		Passed: opts.Passed,
	}
	if opts.Limit > 0 {
		params.Limit = &opts.Limit
	}
	if opts.Offset > 0 {
		params.Offset = &opts.Offset
	}
	if opts.AgentName != "" {
		params.AgentName = &opts.AgentName
	}
	if opts.Namespace != "" {
		params.Namespace = &opts.Namespace
	}
	if opts.EvalID != "" {
		params.EvalId = &opts.EvalID
	}
	if opts.EvalType != "" {
		params.EvalType = &opts.EvalType
	}
	return params
}

// --- Response conversions (generated → internal) ---

// SessionFromAPI converts a generated Session to an internal Session.
func SessionFromAPI(s *Session) *session.Session {
	if s == nil {
		return nil
	}
	out := &session.Session{
		ID:                 uuidToString(s.Id),
		AgentName:          deref(s.AgentName),
		Namespace:          deref(s.Namespace),
		CreatedAt:          deref(s.CreatedAt),
		UpdatedAt:          deref(s.UpdatedAt),
		ExpiresAt:          deref(s.ExpiresAt),
		WorkspaceName:      deref(s.WorkspaceName),
		EndedAt:            deref(s.EndedAt),
		MessageCount:       deref(s.MessageCount),
		ToolCallCount:      deref(s.ToolCallCount),
		TotalInputTokens:   deref(s.TotalInputTokens),
		TotalOutputTokens:  deref(s.TotalOutputTokens),
		EstimatedCostUSD:   deref(s.EstimatedCostUSD),
		LastMessagePreview: deref(s.LastMessagePreview),
		PromptPackName:     deref(s.PromptPackName),
		PromptPackVersion:  deref(s.PromptPackVersion),
		Tags:               derefSlice(s.Tags),
		State:              derefMap(s.State),
		Messages:           MessagesFromAPI(s.Messages),
	}
	if s.Status != nil {
		out.Status = session.SessionStatus(*s.Status)
	}
	return out
}

// MessageFromAPI converts a generated Message to an internal Message.
func MessageFromAPI(m Message) session.Message {
	out := session.Message{
		ID:           deref(m.Id),
		Content:      deref(m.Content),
		Timestamp:    deref(m.Timestamp),
		InputTokens:  deref(m.InputTokens),
		OutputTokens: deref(m.OutputTokens),
		ToolCallID:   deref(m.ToolCallId),
		SequenceNum:  deref(m.SequenceNum),
		Metadata:     derefMap(m.Metadata),
	}
	if m.Role != nil {
		out.Role = session.MessageRole(*m.Role)
	}
	return out
}

// MessagesFromAPI converts a pointer to a slice of generated Messages to internal Messages.
func MessagesFromAPI(msgs *[]Message) []session.Message {
	if msgs == nil {
		return nil
	}
	out := make([]session.Message, len(*msgs))
	for i, m := range *msgs {
		out[i] = MessageFromAPI(m)
	}
	return out
}

// ToolCallFromAPI converts a generated ToolCall to an internal ToolCall.
func ToolCallFromAPI(tc ToolCall) session.ToolCall {
	out := session.ToolCall{
		ID:           deref(tc.Id),
		SessionID:    uuidToString(tc.SessionId),
		CallID:       deref(tc.CallId),
		Name:         deref(tc.Name),
		DurationMs:   deref(tc.DurationMs),
		ErrorMessage: deref(tc.ErrorMessage),
		CreatedAt:    deref(tc.CreatedAt),
		Labels:       derefMap(tc.Labels),
	}
	if tc.Status != nil {
		out.Status = session.ToolCallStatus(*tc.Status)
	}
	if tc.Execution != nil {
		out.Execution = session.ToolCallExecution(*tc.Execution)
	}
	if tc.Arguments != nil {
		out.Arguments = make(map[string]any, len(*tc.Arguments))
		maps.Copy(out.Arguments, *tc.Arguments)
	}
	if tc.Result != nil {
		out.Result = *tc.Result
	}
	return out
}

// ToolCallsFromAPI converts a slice of generated ToolCalls to internal ToolCalls.
func ToolCallsFromAPI(tcs []ToolCall) []session.ToolCall {
	if tcs == nil {
		return nil
	}
	out := make([]session.ToolCall, len(tcs))
	for i, tc := range tcs {
		out[i] = ToolCallFromAPI(tc)
	}
	return out
}

// ProviderCallFromAPI converts a generated ProviderCall to an internal ProviderCall.
func ProviderCallFromAPI(pc ProviderCall) session.ProviderCall {
	out := session.ProviderCall{
		ID:            deref(pc.Id),
		SessionID:     uuidToString(pc.SessionId),
		Provider:      deref(pc.Provider),
		Model:         deref(pc.Model),
		InputTokens:   deref(pc.InputTokens),
		OutputTokens:  deref(pc.OutputTokens),
		CachedTokens:  deref(pc.CachedTokens),
		CostUSD:       deref(pc.CostUsd),
		DurationMs:    deref(pc.DurationMs),
		FinishReason:  deref(pc.FinishReason),
		ToolCallCount: deref(pc.ToolCallCount),
		ErrorMessage:  deref(pc.ErrorMessage),
		CreatedAt:     deref(pc.CreatedAt),
		Labels:        derefMap(pc.Labels),
	}
	if pc.Status != nil {
		out.Status = session.ProviderCallStatus(*pc.Status)
	}
	return out
}

// ProviderCallsFromAPI converts a slice of generated ProviderCalls to internal ProviderCalls.
func ProviderCallsFromAPI(pcs []ProviderCall) []session.ProviderCall {
	if pcs == nil {
		return nil
	}
	out := make([]session.ProviderCall, len(pcs))
	for i, pc := range pcs {
		out[i] = ProviderCallFromAPI(pc)
	}
	return out
}

// EvalResultFromAPI converts a generated EvalResult to an internal EvalResult.
func EvalResultFromAPI(r EvalResult) *api.EvalResult {
	out := &api.EvalResult{
		ID:                deref(r.Id),
		SessionID:         uuidToString(r.SessionId),
		MessageID:         deref(r.MessageId),
		AgentName:         deref(r.AgentName),
		Namespace:         deref(r.Namespace),
		PromptPackName:    deref(r.PromptpackName),
		PromptPackVersion: deref(r.PromptpackVersion),
		EvalID:            deref(r.EvalId),
		EvalType:          deref(r.EvalType),
		Trigger:           deref(r.Trigger),
		Passed:            deref(r.Passed),
		Score:             r.Score,
		DurationMs:        r.DurationMs,
		JudgeTokens:       r.JudgeTokens,
		JudgeCostUSD:      r.JudgeCostUsd,
		Source:            deref(r.Source),
		CreatedAt:         deref(r.CreatedAt),
	}
	if r.Details != nil {
		data, err := json.Marshal(*r.Details)
		if err == nil {
			out.Details = data
		}
	}
	return out
}

// EvalResultsFromAPI converts a pointer to a slice of generated EvalResults to internal EvalResults.
func EvalResultsFromAPI(results *[]EvalResult) []*api.EvalResult {
	if results == nil {
		return nil
	}
	out := make([]*api.EvalResult, len(*results))
	for i, r := range *results {
		out[i] = EvalResultFromAPI(r)
	}
	return out
}

// --- small helpers ---

func ptrNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrNonZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

func ptrNonZeroInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
