/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Sentinel errors for eval result operations.
var (
	ErrMissingEvalResults    = errors.New("at least one eval result is required")
	ErrMissingEvalStore      = errors.New("eval store is not configured")
	ErrMissingEvalDefinition = errors.New("at least one eval definition is required")
	ErrNoMessages            = errors.New("session has no messages")
)

// MessageFetcher abstracts message retrieval for eval execution.
// This allows the eval service to fetch messages without depending on the full SessionService.
type MessageFetcher interface {
	GetMessages(ctx context.Context, sessionID string, opts providers.MessageQueryOpts) ([]*session.Message, error)
}

// EvalService provides business logic for eval result CRUD operations.
type EvalService struct {
	store          EvalStore
	messageFetcher MessageFetcher
	log            logr.Logger
}

// NewEvalService creates a new EvalService with the given store.
func NewEvalService(store EvalStore, log logr.Logger) *EvalService {
	return &EvalService{
		store: store,
		log:   log.WithName("eval-service"),
	}
}

// SetMessageFetcher sets the message fetcher used by EvaluateSession.
func (s *EvalService) SetMessageFetcher(mf MessageFetcher) {
	s.messageFetcher = mf
}

// CreateEvalResults persists one or more eval results.
func (s *EvalService) CreateEvalResults(ctx context.Context, results []*EvalResult) error {
	if len(results) == 0 {
		return ErrMissingEvalResults
	}
	if s.store == nil {
		return ErrMissingEvalStore
	}
	return s.store.InsertEvalResults(ctx, results)
}

// GetSessionEvalResults retrieves all eval results for a session.
func (s *EvalService) GetSessionEvalResults(ctx context.Context, sessionID string) ([]*EvalResult, error) {
	if sessionID == "" {
		return nil, ErrMissingSessionID
	}
	if s.store == nil {
		return nil, ErrMissingEvalStore
	}
	return s.store.GetSessionEvalResults(ctx, sessionID)
}

// ListEvalResults retrieves eval results matching the given filters.
func (s *EvalService) ListEvalResults(ctx context.Context, opts EvalResultListOpts) ([]*EvalResult, int64, error) {
	if s.store == nil {
		return nil, 0, ErrMissingEvalStore
	}
	return s.store.ListEvalResults(ctx, opts)
}

// GetEvalResultSummary returns aggregate statistics for eval results.
func (s *EvalService) GetEvalResultSummary(ctx context.Context, opts EvalResultSummaryOpts) ([]*EvalResultSummary, error) {
	if s.store == nil {
		return nil, ErrMissingEvalStore
	}
	return s.store.GetEvalResultSummary(ctx, opts)
}

// EvaluateSession runs rule-based evals against a session's messages,
// stores the results, and returns them.
func (s *EvalService) EvaluateSession(
	ctx context.Context, sessionID string, evals []EvalDefinition,
) (*EvaluateResponse, error) {
	if err := s.validateEvaluateRequest(sessionID, evals); err != nil {
		return nil, err
	}

	msgs, err := s.fetchMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	results := s.runEvals(evals, msgs)
	resp := buildEvaluateResponse(results)

	s.storeResults(ctx, sessionID, results)

	return resp, nil
}

// validateEvaluateRequest checks preconditions for running evals.
func (s *EvalService) validateEvaluateRequest(sessionID string, evals []EvalDefinition) error {
	if sessionID == "" {
		return ErrMissingSessionID
	}
	if len(evals) == 0 {
		return ErrMissingEvalDefinition
	}
	return nil
}

// fetchMessages retrieves messages for the session.
func (s *EvalService) fetchMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	if s.messageFetcher == nil {
		return nil, fmt.Errorf("message fetcher not configured")
	}

	msgPtrs, err := s.messageFetcher.GetMessages(ctx, sessionID, providers.MessageQueryOpts{
		Limit: maxEvalMessageLimit,
	})
	if err != nil {
		return nil, err
	}

	if len(msgPtrs) == 0 {
		return nil, ErrNoMessages
	}

	msgs := make([]session.Message, 0, len(msgPtrs))
	for _, m := range msgPtrs {
		msgs = append(msgs, *m)
	}
	return msgs, nil
}

// maxEvalMessageLimit is the maximum number of messages to fetch for evaluation.
const maxEvalMessageLimit = 1000

// runEvals executes each eval definition and collects results.
func (s *EvalService) runEvals(evals []EvalDefinition, msgs []session.Message) []EvaluateResultItem {
	results := make([]EvaluateResultItem, 0, len(evals))
	for _, evalDef := range evals {
		item, err := RunRuleEval(evalDef, msgs)
		if err != nil {
			s.log.Error(err, "eval failed", "evalId", evalDef.ID)
			continue
		}
		results = append(results, item)
	}
	return results
}

// buildEvaluateResponse creates the response from eval results.
func buildEvaluateResponse(results []EvaluateResultItem) *EvaluateResponse {
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	return &EvaluateResponse{
		Results: results,
		Summary: EvaluateResponseSummary{
			Total:  len(results),
			Passed: passed,
			Failed: len(results) - passed,
		},
	}
}

// storeResults persists eval results on a best-effort basis.
func (s *EvalService) storeResults(ctx context.Context, sessionID string, items []EvaluateResultItem) {
	if s.store == nil || len(items) == 0 {
		return
	}

	now := time.Now()
	records := make([]*EvalResult, 0, len(items))
	for _, item := range items {
		dur := item.DurationMs
		r := &EvalResult{
			ID:         uuid.New().String(),
			SessionID:  sessionID,
			EvalID:     item.EvalID,
			EvalType:   item.EvalType,
			Trigger:    item.Trigger,
			Passed:     item.Passed,
			Score:      item.Score,
			DurationMs: &dur,
			Source:     "manual",
			CreatedAt:  now,
		}
		records = append(records, r)
	}

	if err := s.store.InsertEvalResults(ctx, records); err != nil {
		s.log.Error(err, "failed to store manual eval results", "sessionID", sessionID)
	}
}
