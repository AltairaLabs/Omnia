/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
)

const testModified = "modified"

func TestMemoryStorage_Store(t *testing.T) {
	s := NewMemoryStorage()
	defer func() { _ = s.Close() }()

	results := &JobResults{
		JobID:       "test-job-1",
		Namespace:   "default",
		ConfigName:  "test-config",
		StartedAt:   time.Now().Add(-time.Minute),
		CompletedAt: time.Now(),
		Summary: &aggregator.AggregatedResult{
			TotalItems:  10,
			PassedItems: 8,
			FailedItems: 2,
			PassRate:    80.0,
		},
		Results: []aggregator.ExecutionResult{
			{WorkItemID: "item-1", Status: "pass"},
			{WorkItemID: "item-2", Status: "fail"},
		},
		Metadata: map[string]string{
			"version": "1.0",
		},
	}

	t.Run("stores results successfully", func(t *testing.T) {
		err := s.Store(context.Background(), "test-job-1", results)
		if err != nil {
			t.Errorf("Store() error = %v", err)
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		err := s.Store(context.Background(), "", results)
		if err != ErrInvalidJobID {
			t.Errorf("Store() error = %v, want %v", err, ErrInvalidJobID)
		}
	})

	t.Run("overwrites existing results", func(t *testing.T) {
		newResults := &JobResults{
			JobID:     "test-job-1",
			Namespace: "updated",
			Summary: &aggregator.AggregatedResult{
				TotalItems:  20,
				PassedItems: 20,
				PassRate:    100.0,
			},
		}
		err := s.Store(context.Background(), "test-job-1", newResults)
		if err != nil {
			t.Errorf("Store() error = %v", err)
		}

		got, err := s.Get(context.Background(), "test-job-1")
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if got.Namespace != "updated" {
			t.Errorf("Namespace = %v, want updated", got.Namespace)
		}
		if got.Summary.TotalItems != 20 {
			t.Errorf("TotalItems = %v, want 20", got.Summary.TotalItems)
		}
	})
}

func TestMemoryStorage_Get(t *testing.T) {
	s := NewMemoryStorage()
	defer func() { _ = s.Close() }()

	results := &JobResults{
		JobID:     "test-job-1",
		Namespace: "default",
		Summary: &aggregator.AggregatedResult{
			TotalItems: 10,
		},
		Results: []aggregator.ExecutionResult{
			{WorkItemID: "item-1"},
		},
		Metadata: map[string]string{
			"key": "value",
		},
	}
	_ = s.Store(context.Background(), "test-job-1", results)

	t.Run("retrieves stored results", func(t *testing.T) {
		got, err := s.Get(context.Background(), "test-job-1")
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if got.JobID != "test-job-1" {
			t.Errorf("JobID = %v, want test-job-1", got.JobID)
		}
		if got.Summary.TotalItems != 10 {
			t.Errorf("TotalItems = %v, want 10", got.Summary.TotalItems)
		}
	})

	t.Run("returns copy not reference", func(t *testing.T) {
		got, _ := s.Get(context.Background(), "test-job-1")
		got.Namespace = testModified
		got.Summary.TotalItems = 999
		got.Results[0].WorkItemID = testModified
		got.Metadata["key"] = testModified

		original, _ := s.Get(context.Background(), "test-job-1")
		if original.Namespace == testModified {
			t.Error("Get() returned reference instead of copy")
		}
		if original.Summary.TotalItems == 999 {
			t.Error("Get() returned reference to Summary")
		}
		if original.Results[0].WorkItemID == testModified {
			t.Error("Get() returned reference to Results")
		}
		if original.Metadata["key"] == testModified {
			t.Error("Get() returned reference to Metadata")
		}
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		_, err := s.Get(context.Background(), "non-existent")
		if err != ErrResultNotFound {
			t.Errorf("Get() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		_, err := s.Get(context.Background(), "")
		if err != ErrInvalidJobID {
			t.Errorf("Get() error = %v, want %v", err, ErrInvalidJobID)
		}
	})
}

func TestMemoryStorage_List(t *testing.T) {
	s := NewMemoryStorage()
	defer func() { _ = s.Close() }()

	// Store multiple results
	for _, jobID := range []string{"job-a-1", "job-a-2", "job-b-1", "job-c-1"} {
		_ = s.Store(context.Background(), jobID, &JobResults{JobID: jobID})
	}

	t.Run("lists all jobs with empty prefix", func(t *testing.T) {
		jobs, err := s.List(context.Background(), "")
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(jobs) != 4 {
			t.Errorf("List() returned %d jobs, want 4", len(jobs))
		}
	})

	t.Run("lists jobs with prefix", func(t *testing.T) {
		jobs, err := s.List(context.Background(), "job-a")
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(jobs) != 2 {
			t.Errorf("List() returned %d jobs, want 2", len(jobs))
		}
	})

	t.Run("returns sorted results", func(t *testing.T) {
		jobs, _ := s.List(context.Background(), "")
		for i := 1; i < len(jobs); i++ {
			if jobs[i-1] > jobs[i] {
				t.Errorf("List() not sorted: %v > %v", jobs[i-1], jobs[i])
			}
		}
	})

	t.Run("returns empty slice for no matches", func(t *testing.T) {
		jobs, err := s.List(context.Background(), "no-match")
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(jobs) != 0 {
			t.Errorf("List() returned %d jobs, want 0", len(jobs))
		}
	})
}

func TestMemoryStorage_Delete(t *testing.T) {
	s := NewMemoryStorage()
	defer func() { _ = s.Close() }()

	_ = s.Store(context.Background(), "test-job-1", &JobResults{JobID: "test-job-1"})

	t.Run("deletes existing results", func(t *testing.T) {
		err := s.Delete(context.Background(), "test-job-1")
		if err != nil {
			t.Errorf("Delete() error = %v", err)
		}

		_, err = s.Get(context.Background(), "test-job-1")
		if err != ErrResultNotFound {
			t.Errorf("Get() after Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		err := s.Delete(context.Background(), "non-existent")
		if err != ErrResultNotFound {
			t.Errorf("Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		err := s.Delete(context.Background(), "")
		if err != ErrInvalidJobID {
			t.Errorf("Delete() error = %v, want %v", err, ErrInvalidJobID)
		}
	})
}

func TestMemoryStorage_Close(t *testing.T) {
	s := NewMemoryStorage()
	_ = s.Store(context.Background(), "test-job-1", &JobResults{JobID: "test-job-1"})

	err := s.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	t.Run("Store returns error after close", func(t *testing.T) {
		err := s.Store(context.Background(), "test", &JobResults{})
		if err != ErrStorageClosed {
			t.Errorf("Store() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("Get returns error after close", func(t *testing.T) {
		_, err := s.Get(context.Background(), "test-job-1")
		if err != ErrStorageClosed {
			t.Errorf("Get() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("List returns error after close", func(t *testing.T) {
		_, err := s.List(context.Background(), "")
		if err != ErrStorageClosed {
			t.Errorf("List() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("Delete returns error after close", func(t *testing.T) {
		err := s.Delete(context.Background(), "test-job-1")
		if err != ErrStorageClosed {
			t.Errorf("Delete() error = %v, want %v", err, ErrStorageClosed)
		}
	})
}

func TestMemoryStorage_ListWithInfo(t *testing.T) {
	s := NewMemoryStorage()
	defer func() { _ = s.Close() }()

	now := time.Now()
	results := []*JobResults{
		{
			JobID:       "job-1",
			Namespace:   "ns-1",
			CompletedAt: now,
			Summary: &aggregator.AggregatedResult{
				TotalItems:  10,
				PassedItems: 8,
				FailedItems: 2,
			},
		},
		{
			JobID:       "job-2",
			Namespace:   "ns-2",
			CompletedAt: now.Add(-time.Hour),
			Summary: &aggregator.AggregatedResult{
				TotalItems:  5,
				PassedItems: 5,
				FailedItems: 0,
			},
		},
	}
	for _, r := range results {
		_ = s.Store(context.Background(), r.JobID, r)
	}

	t.Run("lists with metadata", func(t *testing.T) {
		infos, err := s.ListWithInfo(context.Background(), "")
		if err != nil {
			t.Errorf("ListWithInfo() error = %v", err)
		}
		if len(infos) != 2 {
			t.Errorf("ListWithInfo() returned %d infos, want 2", len(infos))
		}

		// Check first result (sorted by job ID, so job-1 first)
		if infos[0].JobID != "job-1" {
			t.Errorf("infos[0].JobID = %v, want job-1", infos[0].JobID)
		}
		if infos[0].TotalItems != 10 {
			t.Errorf("infos[0].TotalItems = %v, want 10", infos[0].TotalItems)
		}
		if infos[0].PassedItems != 8 {
			t.Errorf("infos[0].PassedItems = %v, want 8", infos[0].PassedItems)
		}
	})

	t.Run("includes size information", func(t *testing.T) {
		infos, _ := s.ListWithInfo(context.Background(), "")
		for _, info := range infos {
			if info.SizeBytes <= 0 {
				t.Errorf("info.SizeBytes = %v, want > 0", info.SizeBytes)
			}
		}
	})
}
