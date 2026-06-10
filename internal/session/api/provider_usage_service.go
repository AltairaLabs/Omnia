/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
)

// ErrMissingProviderUsageStore is returned when no provider_usage store has
// been wired into the Handler — typically because session-api is running
// without database access.
var ErrMissingProviderUsageStore = errors.New("provider usage store is not configured")

// ErrInvalidProviderUsage is returned when a submitted row is missing a
// required field (namespace, provider, or source).
var ErrInvalidProviderUsage = errors.New("provider usage row missing required field")

// ProviderUsageService is the business-logic wrapper around ProviderUsageStore.
type ProviderUsageService struct {
	store ProviderUsageStore
	log   logr.Logger
}

// NewProviderUsageService creates a new ProviderUsageService.
func NewProviderUsageService(store ProviderUsageStore, log logr.Logger) *ProviderUsageService {
	return &ProviderUsageService{
		store: store,
		log:   log.WithName("provider-usage-service"),
	}
}

// RecordProviderUsage validates and persists provider_usage rows. Rows with a
// zero CallCount default to 1.
func (s *ProviderUsageService) RecordProviderUsage(ctx context.Context, rows []*ProviderUsage) error {
	if s.store == nil {
		return ErrMissingProviderUsageStore
	}
	if len(rows) == 0 {
		return nil
	}
	for i, r := range rows {
		if r == nil || r.Namespace == "" || r.Provider == "" || r.Source == "" {
			return fmt.Errorf("%w: row %d", ErrInvalidProviderUsage, i)
		}
		if r.CallCount == 0 {
			r.CallCount = 1
		}
	}
	return s.store.RecordProviderUsage(ctx, rows)
}
