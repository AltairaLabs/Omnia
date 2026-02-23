/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package sources provides SessionSourceAdapter implementations for PromptKit's
// promptarena generate command. The "omnia" adapter reads session and eval data
// from the Omnia session-api.
package sources

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/tools/arena/generate"
	"github.com/altairalabs/omnia/ee/pkg/evals"
	"github.com/altairalabs/omnia/internal/session/api"
)

const adapterName = "omnia"

// SessionAPIAdapter implements generate.SessionSourceAdapter by reading session
// and eval data from the Omnia session-api.
type SessionAPIAdapter struct {
	client evals.SessionAPIClient
}

// NewSessionAPIAdapter creates a new adapter backed by the given session-api client.
func NewSessionAPIAdapter(client evals.SessionAPIClient) *SessionAPIAdapter {
	return &SessionAPIAdapter{client: client}
}

// Name returns the adapter's unique identifier.
func (a *SessionAPIAdapter) Name() string {
	return adapterName
}

// List returns session summaries that have eval results matching the given filters.
func (a *SessionAPIAdapter) List(ctx context.Context, opts generate.ListOptions) ([]generate.SessionSummary, error) {
	listOpts := api.EvalResultListOpts{
		Passed: opts.FilterPassed,
	}
	if opts.FilterEvalType != "" {
		listOpts.EvalID = opts.FilterEvalType
	}
	// Request more results to account for deduplication by session.
	if opts.Limit > 0 {
		listOpts.Limit = opts.Limit * 5 //nolint:mnd // over-fetch to ensure enough unique sessions after dedup
	}

	evalResults, err := a.client.ListEvalResults(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list eval results: %w", err)
	}

	summaries, err := a.buildSummaries(ctx, evalResults, opts.Limit)
	if err != nil {
		return nil, err
	}

	return summaries, nil
}

// buildSummaries deduplicates eval results by session ID and fetches session metadata.
func (a *SessionAPIAdapter) buildSummaries(
	ctx context.Context,
	evalResults []*api.EvalResult,
	limit int,
) ([]generate.SessionSummary, error) {
	seen := make(map[string]bool)
	summaries := make([]generate.SessionSummary, 0, len(evalResults))

	for _, er := range evalResults {
		if seen[er.SessionID] {
			continue
		}
		seen[er.SessionID] = true

		summary, err := a.fetchSummary(ctx, er)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)

		if limit > 0 && len(summaries) >= limit {
			break
		}
	}

	return summaries, nil
}

// fetchSummary retrieves session metadata and builds a SessionSummary.
func (a *SessionAPIAdapter) fetchSummary(
	ctx context.Context,
	er *api.EvalResult,
) (generate.SessionSummary, error) {
	sess, err := a.client.GetSession(ctx, er.SessionID)
	if err != nil {
		return generate.SessionSummary{}, fmt.Errorf("get session %s: %w", er.SessionID, err)
	}

	return generate.SessionSummary{
		ID:          sess.ID,
		Source:      adapterName,
		ProviderID:  sess.AgentName,
		Timestamp:   sess.CreatedAt,
		TurnCount:   int(sess.MessageCount),
		HasFailures: !er.Passed,
		Tags:        sess.Tags,
	}, nil
}

// Get returns the full session detail including messages and eval results.
func (a *SessionAPIAdapter) Get(ctx context.Context, sessionID string) (*generate.SessionDetail, error) {
	sess, err := a.client.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	messages, err := a.client.GetSessionMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session messages: %w", err)
	}

	typesMessages := evals.ConvertToTypesMessages(messages)

	evalResultsRaw, err := a.client.GetSessionEvalResults(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session eval results: %w", err)
	}

	convResults, turnResults := convertEvalResults(evalResultsRaw, messages)

	return &generate.SessionDetail{
		SessionSummary: generate.SessionSummary{
			ID:         sess.ID,
			Source:     adapterName,
			ProviderID: sess.AgentName,
			Timestamp:  sess.CreatedAt,
			TurnCount:  len(messages),
			Tags:       sess.Tags,
		},
		Messages:        typesMessages,
		EvalResults:     convResults,
		TurnEvalResults: turnResults,
	}, nil
}
