/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

const (
	testProviderOpenAI = "openai"
	testModelClaude    = "claude"
)

// fakeProviderCallWriter records the calls forwarded to it.
type fakeProviderCallWriter struct {
	calls   []*session.ProviderCall
	failOn  string // provider name that should error
	failErr error
}

func (f *fakeProviderCallWriter) RecordProviderCall(_ context.Context, _ string, pc *session.ProviderCall) error {
	if f.failOn != "" && pc.Provider == f.failOn {
		return f.failErr
	}
	f.calls = append(f.calls, pc)
	return nil
}

func TestProviderCallCollector_OnCompleted(t *testing.T) {
	c := newProviderCallCollector("sess-1", "omnia-demo", "support")
	ts := time.Now().UTC().Truncate(time.Second)

	c.onCompleted(&events.Event{
		Type:      events.EventProviderCallCompleted,
		Timestamp: ts,
		Data: &events.ProviderCallCompletedData{
			Provider:      testProviderOpenAI,
			Model:         "gpt-4o",
			Duration:      1500 * time.Millisecond,
			InputTokens:   300,
			OutputTokens:  40,
			CachedTokens:  10,
			Cost:          0.012,
			FinishReason:  "stop",
			ToolCallCount: 0,
			Source:        events.SourceJudge,
		},
	})

	calls := c.collected()
	require.Len(t, calls, 1)
	pc := calls[0]
	assert.Equal(t, "sess-1", pc.SessionID)
	assert.Equal(t, "omnia-demo", pc.Namespace)
	assert.Equal(t, "support", pc.AgentName)
	assert.Equal(t, testProviderOpenAI, pc.Provider)
	assert.Equal(t, "gpt-4o", pc.Model)
	assert.Equal(t, session.ProviderCallStatusCompleted, pc.Status)
	assert.Equal(t, int64(300), pc.InputTokens)
	assert.Equal(t, int64(40), pc.OutputTokens)
	assert.Equal(t, int64(10), pc.CachedTokens)
	assert.InDelta(t, 0.012, pc.CostUSD, 1e-9)
	assert.Equal(t, int64(1500), pc.DurationMs)
	assert.Equal(t, "judge", pc.Source)
	assert.Equal(t, ts, pc.CreatedAt)
	assert.NotEmpty(t, pc.ID)
}

func TestProviderCallCollector_OnFailed(t *testing.T) {
	c := newProviderCallCollector("sess-2", "ns", "agent-x")
	c.onFailed(&events.Event{
		Type:      events.EventProviderCallFailed,
		Timestamp: time.Now().UTC(),
		Data: &events.ProviderCallFailedData{
			Provider: "anthropic",
			Model:    testModelClaude,
			Error:    errors.New("boom"),
			Duration: 200 * time.Millisecond,
			Source:   events.SourceJudge,
		},
	})

	calls := c.collected()
	require.Len(t, calls, 1)
	assert.Equal(t, session.ProviderCallStatusFailed, calls[0].Status)
	assert.Equal(t, "boom", calls[0].ErrorMessage)
	assert.Equal(t, "judge", calls[0].Source)
}

func TestProviderCallCollector_IgnoresWrongDataType(t *testing.T) {
	c := newProviderCallCollector("s", "ns", "a")
	c.onCompleted(&events.Event{Type: events.EventProviderCallCompleted, Data: &events.ProviderCallFailedData{}})
	c.onFailed(&events.Event{Type: events.EventProviderCallFailed, Data: &events.ProviderCallCompletedData{}})
	assert.Empty(t, c.collected())
}

func TestFlushProviderCalls_ForwardsAll(t *testing.T) {
	w := &fakeProviderCallWriter{}
	calls := []*session.ProviderCall{
		{SessionID: "s1", Provider: testProviderOpenAI, Source: "judge"},
		{SessionID: "s1", Provider: "azure", Source: "embedding"},
	}
	flushProviderCalls(context.Background(), w, nil, calls)
	require.Len(t, w.calls, 2)
	assert.Equal(t, testProviderOpenAI, w.calls[0].Provider)
	assert.Equal(t, "azure", w.calls[1].Provider)
}

func TestFlushProviderCalls_WriteErrorDoesNotStop(t *testing.T) {
	w := &fakeProviderCallWriter{failOn: testProviderOpenAI, failErr: errors.New("nope")}
	calls := []*session.ProviderCall{
		{SessionID: "s1", Provider: testProviderOpenAI, Source: "judge"},
		{SessionID: "s1", Provider: "azure", Source: "embedding"},
	}
	// nil logger must be tolerated; the azure call still goes through.
	flushProviderCalls(context.Background(), w, nil, calls)
	require.Len(t, w.calls, 1)
	assert.Equal(t, "azure", w.calls[0].Provider)
}
