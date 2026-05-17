/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
)

// ErrMissingProviderCallsStore is returned by the service methods when no
// store has been wired into the Handler — typically because session-api is
// running without database access.
var ErrMissingProviderCallsStore = errors.New("provider calls store is not configured")

// ProviderCallsService is the business-logic wrapper around ProviderCallsStore.
// Kept structurally identical to EvalService for consistency.
type ProviderCallsService struct {
	store ProviderCallsStore
	log   logr.Logger
}

// NewProviderCallsService creates a new ProviderCallsService.
func NewProviderCallsService(store ProviderCallsStore, log logr.Logger) *ProviderCallsService {
	return &ProviderCallsService{
		store: store,
		log:   log.WithName("provider-calls-service"),
	}
}

// AggregateProviderCalls runs a namespace-scoped GROUP BY over provider_calls.
// Powers GET /api/v1/provider-calls/aggregate.
func (s *ProviderCallsService) AggregateProviderCalls(ctx context.Context, opts ProviderCallAggregateOpts) ([]*ProviderCallAggregateRow, error) {
	if s.store == nil {
		return nil, ErrMissingProviderCallsStore
	}
	return s.store.AggregateProviderCalls(ctx, opts)
}

// ProviderCallsDiscovery returns the distinct provider/model values for a
// namespace. Powers GET /api/v1/provider-calls/discover.
func (s *ProviderCallsService) ProviderCallsDiscovery(ctx context.Context, namespace string) (*ProviderCallDiscoveryResult, error) {
	if s.store == nil {
		return nil, ErrMissingProviderCallsStore
	}
	return s.store.ProviderCallsDiscovery(ctx, namespace)
}
