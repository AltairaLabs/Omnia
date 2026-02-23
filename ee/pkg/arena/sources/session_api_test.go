/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package sources

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/tools/arena/generate"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// mockClient implements evals.SessionAPIClient for testing.
type mockClient struct {
	sessions    map[string]*session.Session
	messages    map[string][]session.Message
	evalResults []*api.EvalResult
	sessResults map[string][]*api.EvalResult

	getSessionErr            error
	getMessagesErr           error
	listEvalResultsErr       error
	getSessionEvalResultsErr error
}

func (m *mockClient) GetSession(_ context.Context, id string) (*session.Session, error) {
	if m.getSessionErr != nil {
		return nil, m.getSessionErr
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return s, nil
}

func (m *mockClient) GetSessionMessages(_ context.Context, id string) ([]session.Message, error) {
	if m.getMessagesErr != nil {
		return nil, m.getMessagesErr
	}
	return m.messages[id], nil
}

func (m *mockClient) WriteEvalResults(_ context.Context, _ []*api.EvalResult) error {
	return nil
}

func (m *mockClient) ListEvalResults(_ context.Context, _ api.EvalResultListOpts) ([]*api.EvalResult, error) {
	if m.listEvalResultsErr != nil {
		return nil, m.listEvalResultsErr
	}
	return m.evalResults, nil
}

func (m *mockClient) GetSessionEvalResults(_ context.Context, id string) ([]*api.EvalResult, error) {
	if m.getSessionEvalResultsErr != nil {
		return nil, m.getSessionEvalResultsErr
	}
	return m.sessResults[id], nil
}

func TestSessionAPIAdapter_Name(t *testing.T) {
	adapter := NewSessionAPIAdapter(nil)
	assert.Equal(t, "omnia", adapter.Name())
}

func TestSessionAPIAdapter_List_FilterFailed(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1", AgentName: "agent-a", CreatedAt: now, MessageCount: 4, Tags: []string{"prod"}},
			"s2": {ID: "s2", AgentName: "agent-b", CreatedAt: now, MessageCount: 6},
		},
		evalResults: []*api.EvalResult{
			{ID: "er1", SessionID: "s1", Passed: false},
			{ID: "er2", SessionID: "s1", Passed: false},
			{ID: "er3", SessionID: "s2", Passed: false},
		},
	}

	adapter := NewSessionAPIAdapter(mock)
	passed := false
	summaries, err := adapter.List(context.Background(), generate.ListOptions{
		FilterPassed: &passed,
	})

	require.NoError(t, err)
	require.Len(t, summaries, 2)

	assert.Equal(t, "s1", summaries[0].ID)
	assert.Equal(t, "omnia", summaries[0].Source)
	assert.Equal(t, "agent-a", summaries[0].ProviderID)
	assert.Equal(t, 4, summaries[0].TurnCount)
	assert.True(t, summaries[0].HasFailures)
	assert.Equal(t, []string{"prod"}, summaries[0].Tags)

	assert.Equal(t, "s2", summaries[1].ID)
	assert.Equal(t, "agent-b", summaries[1].ProviderID)
	assert.Equal(t, 6, summaries[1].TurnCount)
}

func TestSessionAPIAdapter_List_Limit(t *testing.T) {
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1", AgentName: "a"},
			"s2": {ID: "s2", AgentName: "b"},
			"s3": {ID: "s3", AgentName: "c"},
		},
		evalResults: []*api.EvalResult{
			{SessionID: "s1", Passed: false},
			{SessionID: "s2", Passed: false},
			{SessionID: "s3", Passed: false},
		},
	}

	adapter := NewSessionAPIAdapter(mock)
	summaries, err := adapter.List(context.Background(), generate.ListOptions{Limit: 2})

	require.NoError(t, err)
	require.Len(t, summaries, 2)
}

func TestSessionAPIAdapter_List_Empty(t *testing.T) {
	mock := &mockClient{
		evalResults: []*api.EvalResult{},
	}

	adapter := NewSessionAPIAdapter(mock)
	summaries, err := adapter.List(context.Background(), generate.ListOptions{})

	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestSessionAPIAdapter_List_ClientError(t *testing.T) {
	mock := &mockClient{
		listEvalResultsErr: errors.New("connection refused"),
	}

	adapter := NewSessionAPIAdapter(mock)
	_, err := adapter.List(context.Background(), generate.ListOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "list eval results")
}

func TestSessionAPIAdapter_List_GetSessionError(t *testing.T) {
	mock := &mockClient{
		evalResults:   []*api.EvalResult{{SessionID: "s1"}},
		getSessionErr: errors.New("not found"),
	}

	adapter := NewSessionAPIAdapter(mock)
	_, err := adapter.List(context.Background(), generate.ListOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session s1")
}

func TestSessionAPIAdapter_Get(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1", AgentName: "agent-a", CreatedAt: now, Tags: []string{"test"}},
		},
		messages: map[string][]session.Message{
			"s1": {
				{ID: "m1", Role: session.RoleUser, Content: "hello"},
				{ID: "m2", Role: session.RoleAssistant, Content: "hi there"},
			},
		},
		sessResults: map[string][]*api.EvalResult{
			"s1": {
				{EvalID: "tone", EvalType: "tone", Passed: true, Details: json.RawMessage(`{"score": 0.9}`)},
				{EvalID: "contains", EvalType: "contains", Passed: false, MessageID: "m2", Details: json.RawMessage(`{"expected": "goodbye"}`)},
			},
		},
	}

	adapter := NewSessionAPIAdapter(mock)
	detail, err := adapter.Get(context.Background(), "s1")

	require.NoError(t, err)
	assert.Equal(t, "s1", detail.ID)
	assert.Equal(t, "omnia", detail.Source)
	assert.Equal(t, "agent-a", detail.ProviderID)
	assert.Equal(t, now, detail.Timestamp)
	assert.Equal(t, 2, detail.TurnCount)
	assert.Equal(t, []string{"test"}, detail.Tags)

	require.Len(t, detail.Messages, 2)
	assert.Equal(t, "user", detail.Messages[0].Role)
	assert.Equal(t, "hello", detail.Messages[0].Content)
	assert.Equal(t, "assistant", detail.Messages[1].Role)

	require.Len(t, detail.EvalResults, 1)
	assert.Equal(t, "tone", detail.EvalResults[0].Type)
	assert.True(t, detail.EvalResults[0].Passed)

	require.NotNil(t, detail.TurnEvalResults)
	require.Len(t, detail.TurnEvalResults[1], 1)
	assert.Equal(t, "contains", detail.TurnEvalResults[1][0].Type)
	assert.False(t, detail.TurnEvalResults[1][0].Passed)
}

func TestSessionAPIAdapter_Get_NoEvalResults(t *testing.T) {
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1", AgentName: "agent-a"},
		},
		messages: map[string][]session.Message{
			"s1": {{ID: "m1", Role: session.RoleUser, Content: "hello"}},
		},
		sessResults: map[string][]*api.EvalResult{},
	}

	adapter := NewSessionAPIAdapter(mock)
	detail, err := adapter.Get(context.Background(), "s1")

	require.NoError(t, err)
	require.Len(t, detail.Messages, 1)
	assert.Nil(t, detail.EvalResults)
	assert.Nil(t, detail.TurnEvalResults)
}

func TestSessionAPIAdapter_Get_SessionError(t *testing.T) {
	mock := &mockClient{
		getSessionErr: errors.New("not found"),
	}

	adapter := NewSessionAPIAdapter(mock)
	_, err := adapter.Get(context.Background(), "s1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session")
}

func TestSessionAPIAdapter_Get_MessagesError(t *testing.T) {
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1"},
		},
		getMessagesErr: errors.New("timeout"),
	}

	adapter := NewSessionAPIAdapter(mock)
	_, err := adapter.Get(context.Background(), "s1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session messages")
}

func TestSessionAPIAdapter_Get_EvalResultsError(t *testing.T) {
	mock := &mockClient{
		sessions: map[string]*session.Session{
			"s1": {ID: "s1"},
		},
		messages: map[string][]session.Message{
			"s1": {{ID: "m1", Role: session.RoleUser, Content: "hello"}},
		},
		getSessionEvalResultsErr: errors.New("db error"),
	}

	adapter := NewSessionAPIAdapter(mock)
	_, err := adapter.Get(context.Background(), "s1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session eval results")
}
