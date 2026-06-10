/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"time"
)

// ProviderUsage is one workspace-scoped, session-less provider-spend record —
// query/ingest/re-embed embeddings, judge calls, consolidation. Unlike
// ProviderCall it has no session; namespace is the attribution key and source
// distinguishes the producer.
type ProviderUsage struct {
	// ID is server-generated when empty.
	ID string `json:"id,omitempty"`
	// Namespace is the attribution key (the workspace namespace). Required.
	Namespace string `json:"namespace"`
	// WorkspaceName is the human-facing workspace name (optional).
	WorkspaceName string `json:"workspaceName,omitempty"`
	// Provider is the provider type (e.g. "openai", "azure"). Required.
	Provider string `json:"provider"`
	// ProviderName is the Provider CRD name, distinguishing same-type providers.
	ProviderName string `json:"providerName,omitempty"`
	// Model is the model/deployment (e.g. "text-embedding-3-small").
	Model string `json:"model,omitempty"`
	// Source identifies the producer: "embedding", "ingestion", "judge", etc.
	// Required.
	Source string `json:"source"`
	// InputTokens / OutputTokens / CachedTokens are the token counts. Embeddings
	// are input-only (OutputTokens stays 0).
	InputTokens  int64 `json:"inputTokens,omitempty"`
	OutputTokens int64 `json:"outputTokens,omitempty"`
	CachedTokens int64 `json:"cachedTokens,omitempty"`
	// CostUSD is the estimated cost in USD.
	CostUSD float64 `json:"costUsd,omitempty"`
	// CallCount is how many provider calls this row aggregates (default 1).
	CallCount int32 `json:"callCount,omitempty"`
	// CreatedAt defaults to now when zero.
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// ProviderUsageStore persists workspace-scoped provider spend that does not
// belong to a single session (the provider_usage table). Written by producers
// like memory-api (embeddings) and the eval worker (judge tokens).
type ProviderUsageStore interface {
	// RecordProviderUsage inserts one or more provider_usage rows. Each row's
	// Namespace, Provider, and Source are required.
	RecordProviderUsage(ctx context.Context, rows []*ProviderUsage) error
}
