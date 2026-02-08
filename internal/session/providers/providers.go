/*
Copyright 2025.

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

// Package providers defines tier-specific interfaces for pluggable session
// storage backends (hot/warm/cold) and a Registry to manage them.
package providers

import (
	"errors"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// Sentinel errors returned by provider operations.
var (
	// ErrProviderNotConfigured is returned when a requested provider tier has not been set.
	ErrProviderNotConfigured = errors.New("provider not configured")
	// ErrPartitionExists is returned when attempting to create a partition that already exists.
	ErrPartitionExists = errors.New("partition already exists")
	// ErrPartitionNotFound is returned when a referenced partition does not exist.
	ErrPartitionNotFound = errors.New("partition not found")
)

// SortOrder specifies the ordering direction for query results.
type SortOrder string

const (
	// SortAsc sorts results in ascending (chronological) order.
	SortAsc SortOrder = "asc"
	// SortDesc sorts results in descending (reverse chronological) order.
	SortDesc SortOrder = "desc"
)

// MessageQueryOpts configures message retrieval from a session.
type MessageQueryOpts struct {
	// Limit is the maximum number of messages to return (0 = no limit).
	Limit int
	// Offset is the number of messages to skip.
	Offset int
	// SortOrder controls ordering; default is SortAsc (chronological).
	SortOrder SortOrder
	// AfterSeq returns only messages with SequenceNum > AfterSeq.
	AfterSeq int32
	// BeforeSeq returns only messages with SequenceNum < BeforeSeq.
	BeforeSeq int32
	// Roles filters messages to only these roles.
	Roles []session.MessageRole
}

// SessionListOpts configures session listing and search queries.
type SessionListOpts struct {
	// Limit is the maximum number of sessions to return (0 = no limit).
	Limit int
	// Offset is the number of sessions to skip.
	Offset int
	// SortOrder controls ordering; default is SortDesc (newest first).
	SortOrder SortOrder
	// AgentName filters sessions by agent name.
	AgentName string
	// Namespace filters sessions by Kubernetes namespace.
	Namespace string
	// WorkspaceName filters sessions by workspace name.
	WorkspaceName string
	// Status filters sessions by lifecycle status.
	Status session.SessionStatus
	// CreatedAfter filters sessions created after this time.
	CreatedAfter time.Time
	// CreatedBefore filters sessions created before this time.
	CreatedBefore time.Time
	// Tags filters sessions that have all of the specified tags.
	Tags []string
}

// SessionPage is a paginated result of sessions.
type SessionPage struct {
	// Sessions contains the result set for this page.
	Sessions []*session.Session
	// TotalCount is the total number of matching sessions across all pages.
	TotalCount int64
	// HasMore indicates whether additional pages are available.
	HasMore bool
}

// PartitionInfo describes a table partition in the warm store.
type PartitionInfo struct {
	// Name is the partition identifier (e.g. "sessions_2025_w01").
	Name string
	// StartDate is the inclusive start of the partition range.
	StartDate time.Time
	// EndDate is the exclusive end of the partition range.
	EndDate time.Time
	// RowCount is the number of rows in the partition.
	RowCount int64
	// SizeBytes is the storage size of the partition in bytes.
	SizeBytes int64
}

// WriteOpts configures cold archive write operations.
type WriteOpts struct {
	// BasePath is the object storage prefix (e.g. "sessions/2025/w01/").
	BasePath string
	// Compression is the compression codec ("snappy", "gzip", "zstd").
	Compression string
	// MaxFileSize is the maximum bytes per output file.
	MaxFileSize int64
}

// Registry holds configured provider instances for each storage tier.
type Registry struct {
	hotCache    HotCacheProvider
	warmStore   WarmStoreProvider
	coldArchive ColdArchiveProvider
}

// NewRegistry creates an empty Registry with no providers configured.
func NewRegistry() *Registry {
	return &Registry{}
}

// SetHotCache registers a hot cache provider.
func (r *Registry) SetHotCache(p HotCacheProvider) {
	r.hotCache = p
}

// SetWarmStore registers a warm store provider.
func (r *Registry) SetWarmStore(p WarmStoreProvider) {
	r.warmStore = p
}

// SetColdArchive registers a cold archive provider.
func (r *Registry) SetColdArchive(p ColdArchiveProvider) {
	r.coldArchive = p
}

// HotCache returns the configured hot cache provider.
// Returns ErrProviderNotConfigured if no hot cache has been set.
func (r *Registry) HotCache() (HotCacheProvider, error) {
	if r.hotCache == nil {
		return nil, ErrProviderNotConfigured
	}
	return r.hotCache, nil
}

// WarmStore returns the configured warm store provider.
// Returns ErrProviderNotConfigured if no warm store has been set.
func (r *Registry) WarmStore() (WarmStoreProvider, error) {
	if r.warmStore == nil {
		return nil, ErrProviderNotConfigured
	}
	return r.warmStore, nil
}

// ColdArchive returns the configured cold archive provider.
// Returns ErrProviderNotConfigured if no cold archive has been set.
func (r *Registry) ColdArchive() (ColdArchiveProvider, error) {
	if r.coldArchive == nil {
		return nil, ErrProviderNotConfigured
	}
	return r.coldArchive, nil
}

// Close closes all configured providers, collecting any errors.
func (r *Registry) Close() error {
	var errs []error
	if r.hotCache != nil {
		if err := r.hotCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.warmStore != nil {
		if err := r.warmStore.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.coldArchive != nil {
		if err := r.coldArchive.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
