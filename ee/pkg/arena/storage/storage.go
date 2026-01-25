/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package storage provides interfaces and implementations for storing
// Arena job results. It supports multiple backends including local filesystem
// (PVC) and S3-compatible object storage.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
)

// Common errors returned by storage implementations.
var (
	// ErrResultNotFound is returned when job results do not exist.
	ErrResultNotFound = errors.New("result not found")

	// ErrStorageClosed is returned when operations are attempted on a closed storage.
	ErrStorageClosed = errors.New("storage closed")

	// ErrInvalidJobID is returned when a job ID is empty or malformed.
	ErrInvalidJobID = errors.New("invalid job ID")
)

// JobResults contains the complete results for an Arena job.
type JobResults struct {
	// JobID is the unique identifier for the job.
	JobID string `json:"jobId"`

	// Namespace is the Kubernetes namespace of the job.
	Namespace string `json:"namespace,omitempty"`

	// ConfigName is the name of the ArenaConfig used.
	ConfigName string `json:"configName,omitempty"`

	// StartedAt is when the job started executing.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the job finished.
	CompletedAt time.Time `json:"completedAt"`

	// Summary contains aggregated metrics for the job.
	Summary *aggregator.AggregatedResult `json:"summary"`

	// Results contains individual execution results for each work item.
	Results []aggregator.ExecutionResult `json:"results,omitempty"`

	// Metadata contains additional key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ResultStorage defines the interface for storing Arena job results.
// Implementations must be safe for concurrent use.
type ResultStorage interface {
	// Store persists job results to storage.
	// If results for the job already exist, they are overwritten.
	// Returns ErrInvalidJobID if jobID is empty.
	Store(ctx context.Context, jobID string, results *JobResults) error

	// Get retrieves job results from storage.
	// Returns ErrResultNotFound if no results exist for the job.
	// Returns ErrInvalidJobID if jobID is empty.
	Get(ctx context.Context, jobID string) (*JobResults, error)

	// List returns job IDs that match the given prefix.
	// If prefix is empty, all job IDs are returned.
	// Results are returned in lexicographic order.
	List(ctx context.Context, prefix string) ([]string, error)

	// Delete removes job results from storage.
	// Returns ErrResultNotFound if no results exist for the job.
	// Returns ErrInvalidJobID if jobID is empty.
	Delete(ctx context.Context, jobID string) error

	// Close releases any resources held by the storage.
	// After Close is called, all other methods return ErrStorageClosed.
	Close() error
}

// ResultInfo contains metadata about stored results without the full result data.
type ResultInfo struct {
	// JobID is the unique identifier for the job.
	JobID string `json:"jobId"`

	// Namespace is the Kubernetes namespace of the job.
	Namespace string `json:"namespace,omitempty"`

	// CompletedAt is when the job finished.
	CompletedAt time.Time `json:"completedAt"`

	// TotalItems is the total number of work items.
	TotalItems int `json:"totalItems"`

	// PassedItems is the number of items that passed.
	PassedItems int `json:"passedItems"`

	// FailedItems is the number of items that failed.
	FailedItems int `json:"failedItems"`

	// SizeBytes is the size of the stored result data.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// ListableResultStorage extends ResultStorage with the ability to list
// results with metadata without loading full result data.
type ListableResultStorage interface {
	ResultStorage

	// ListWithInfo returns result metadata for jobs matching the prefix.
	// This is more efficient than calling Get for each job when only
	// summary information is needed.
	ListWithInfo(ctx context.Context, prefix string) ([]ResultInfo, error)
}
