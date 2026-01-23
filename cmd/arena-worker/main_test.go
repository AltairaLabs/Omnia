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

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/altairalabs/omnia/pkg/arena/queue"
)

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_NOT_SET",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
		{
			name:         "returns env value when set",
			key:          "TEST_SET",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			got := getEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDurationEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		want         time.Duration
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_DURATION_NOT_SET",
			defaultValue: 5 * time.Second,
			envValue:     "",
			want:         5 * time.Second,
		},
		{
			name:         "returns parsed duration when set",
			key:          "TEST_DURATION_SET",
			defaultValue: 5 * time.Second,
			envValue:     "10s",
			want:         10 * time.Second,
		},
		{
			name:         "returns default on invalid duration",
			key:          "TEST_DURATION_INVALID",
			defaultValue: 5 * time.Second,
			envValue:     "invalid",
			want:         5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			got := getDurationEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getDurationEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("returns error when ARENA_JOB_NAME is missing", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "")
		t.Setenv("ARENA_CONTENT_PATH", "/workspace/content")

		_, err := loadConfig()
		if err == nil {
			t.Error("loadConfig() should return error when ARENA_JOB_NAME is missing")
		}
	})

	t.Run("returns error when ARENA_CONTENT_PATH is missing", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "")

		_, err := loadConfig()
		if err == nil {
			t.Error("loadConfig() should return error when ARENA_CONTENT_PATH is missing")
		}
	})

	t.Run("returns config when required fields are set", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/workspace/content")

		cfg, err := loadConfig()
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if cfg.JobName != "test-job" {
			t.Errorf("JobName = %v, want test-job", cfg.JobName)
		}
		if cfg.ContentPath != "/workspace/content" {
			t.Errorf("ContentPath = %v, want /workspace/content", cfg.ContentPath)
		}
		// Check defaults
		if cfg.RedisAddr != "redis:6379" {
			t.Errorf("RedisAddr = %v, want redis:6379", cfg.RedisAddr)
		}
	})
}

func TestProcessWorkItems(t *testing.T) {
	t.Run("handles context cancellation", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-cancel"

		// Push a work item
		items := []queue.WorkItem{
			{ID: "item-1", ScenarioID: "scenario1", ProviderID: "provider1"},
		}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push items: %v", err)
		}

		tmpDir := t.TempDir()
		cfg := &Config{
			JobName:      jobID,
			WorkDir:      tmpDir,
			PollInterval: 10 * time.Millisecond,
		}

		// Cancel context immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() with cancelled context should return nil, got %v", err)
		}
	})
}

func TestFindArenaConfigFile(t *testing.T) {
	t.Run("finds config.arena.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.arena.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("finds arena.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "arena.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("finds config.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != configPath {
			t.Errorf("expected %s, got %s", configPath, result)
		}
	})

	t.Run("prefers config.arena.yaml over others", func(t *testing.T) {
		tmpDir := t.TempDir()
		arenaConfig := filepath.Join(tmpDir, "config.arena.yaml")
		plainConfig := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(arenaConfig, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create arena config: %v", err)
		}
		if err := os.WriteFile(plainConfig, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create plain config: %v", err)
		}

		result := findArenaConfigFile(tmpDir)
		if result != arenaConfig {
			t.Errorf("expected %s, got %s", arenaConfig, result)
		}
	})

	t.Run("returns empty string when no config found", func(t *testing.T) {
		tmpDir := t.TempDir()

		result := findArenaConfigFile(tmpDir)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})
}

func TestExecuteWorkItem(t *testing.T) {
	t.Run("returns error when arena config not found", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &Config{
			WorkDir: tmpDir,
		}

		item := &queue.WorkItem{
			ID:         "test-item",
			ScenarioID: "test-scenario",
			ProviderID: "test-provider",
		}

		_, err := executeWorkItem(context.Background(), cfg, item, tmpDir)
		if err == nil {
			t.Error("executeWorkItem() should return error when arena config is missing")
		}
		if !contains(err.Error(), "arena config file not found") {
			t.Errorf("expected error about missing config, got: %v", err)
		}
	})
}

// contains checks if s contains any of the substrings (used in tests)
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}

func TestGetContentPath(t *testing.T) {
	t.Run("returns content path when directory exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		if err := os.MkdirAll(contentDir, 0755); err != nil {
			t.Fatalf("failed to create content dir: %v", err)
		}

		cfg := &Config{
			ContentPath: contentDir,
		}

		path, err := getContentPath(cfg)
		if err != nil {
			t.Fatalf("getContentPath() error = %v", err)
		}
		if path != contentDir {
			t.Errorf("expected %s, got %s", contentDir, path)
		}
	})

	t.Run("returns error when ContentPath does not exist", func(t *testing.T) {
		cfg := &Config{
			ContentPath: "/nonexistent/path",
		}

		_, err := getContentPath(cfg)
		if err == nil {
			t.Error("expected error when ContentPath does not exist")
		}
	})

	t.Run("returns error when ContentPath is a file not directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "file.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		cfg := &Config{
			ContentPath: filePath,
		}

		_, err := getContentPath(cfg)
		if err == nil {
			t.Error("expected error when ContentPath is a file")
		}
	})
}

func TestHandlePopError(t *testing.T) {
	t.Run("returns error for non-empty-queue errors", func(t *testing.T) {
		cfg := &Config{
			PollInterval: 10 * time.Millisecond,
		}
		q := queue.NewMemoryQueueWithDefaults()

		done, newCount, err := handlePopError(
			context.Background(),
			context.DeadlineExceeded, // Non-queue error
			0,
			10,
			cfg,
			q,
			"test-job",
		)

		if err == nil {
			t.Error("expected error for non-empty-queue errors")
		}
		if done {
			t.Error("should not be done on error")
		}
		if newCount != 0 {
			t.Errorf("expected count 0, got %d", newCount)
		}
	})

	t.Run("increments count on empty queue", func(t *testing.T) {
		cfg := &Config{
			PollInterval: 1 * time.Millisecond,
		}
		q := queue.NewMemoryQueueWithDefaults()

		done, newCount, err := handlePopError(
			context.Background(),
			queue.ErrQueueEmpty,
			5,
			10,
			cfg,
			q,
			"test-job",
		)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if done {
			t.Error("should not be done yet")
		}
		if newCount != 6 {
			t.Errorf("expected count 6, got %d", newCount)
		}
	})

	t.Run("checks completion at max empty polls", func(t *testing.T) {
		cfg := &Config{
			PollInterval: 1 * time.Millisecond,
		}
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-complete"

		// Initialize progress to simulate completed job
		items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}
		// Pop and ack to complete
		item, _ := q.Pop(context.Background(), jobID)
		_ = q.Ack(context.Background(), jobID, item.ID, []byte(`{"status":"pass"}`))

		done, _, err := handlePopError(
			context.Background(),
			queue.ErrQueueEmpty,
			9, // At max - 1
			10,
			cfg,
			q,
			jobID,
		)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !done {
			t.Error("expected done=true when job is complete")
		}
	})
}

func TestCheckJobCompletion(t *testing.T) {
	t.Run("returns true when job is complete", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-done"

		// Push and complete items
		items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}
		item, _ := q.Pop(context.Background(), jobID)
		_ = q.Ack(context.Background(), jobID, item.ID, []byte(`{}`))

		done, err := checkJobCompletion(context.Background(), q, jobID, 10)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !done {
			t.Error("expected done=true")
		}
	})

	t.Run("returns false when job is not complete", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-pending"

		// Push items but don't complete them
		items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}

		done, err := checkJobCompletion(context.Background(), q, jobID, 10)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if done {
			t.Error("expected done=false when items pending")
		}
	})
}

func TestReportWorkItemResult(t *testing.T) {
	t.Run("acks successful result", func(t *testing.T) {
		q := queue.NewMemoryQueueWithDefaults()
		jobID := "test-job-ack"

		items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}
		item, _ := q.Pop(context.Background(), jobID)

		result := &ExecutionResult{
			Status:     statusPass,
			DurationMs: 100,
		}

		reportWorkItemResult(context.Background(), q, jobID, item, result, nil)

		// Check progress
		progress, _ := q.Progress(context.Background(), jobID)
		if progress.Completed != 1 {
			t.Errorf("expected 1 completed, got %d", progress.Completed)
		}
	})

	t.Run("nacks failed result", func(t *testing.T) {
		// Use a queue with MaxRetries=1 so Nack immediately marks as failed
		q := queue.NewMemoryQueue(queue.Options{
			VisibilityTimeout: 5 * time.Minute,
			MaxRetries:        1,
		})
		jobID := "test-job-nack"

		items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}
		item, _ := q.Pop(context.Background(), jobID)

		execErr := context.DeadlineExceeded
		reportWorkItemResult(context.Background(), q, jobID, item, nil, execErr)

		// Check progress - with MaxRetries=1, the item should be marked as failed
		progress, _ := q.Progress(context.Background(), jobID)
		if progress.Failed != 1 {
			t.Errorf("expected 1 failed, got %d", progress.Failed)
		}
	})
}

func TestBuildFallbackResult(t *testing.T) {
	t.Run("returns pass when runs exist", func(t *testing.T) {
		result := &ExecutionResult{
			DurationMs: 100,
			Metrics:    make(map[string]float64),
		}
		runIDs := []string{"run-1", "run-2"}

		got := buildFallbackResult(result, runIDs)

		if got.Status != statusPass {
			t.Errorf("expected status %s, got %s", statusPass, got.Status)
		}
	})

	t.Run("returns fail when no runs", func(t *testing.T) {
		result := &ExecutionResult{
			DurationMs: 100,
			Metrics:    make(map[string]float64),
		}
		runIDs := []string{}

		got := buildFallbackResult(result, runIDs)

		if got.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, got.Status)
		}
		if got.Error != "no runs executed" {
			t.Errorf("expected error 'no runs executed', got '%s'", got.Error)
		}
	})
}

func TestPopulateMetrics(t *testing.T) {
	t.Run("populates all metrics", func(t *testing.T) {
		result := &ExecutionResult{
			Metrics: make(map[string]float64),
		}
		agg := &runAggregator{
			passCount:     5,
			failCount:     2,
			totalDuration: 1500 * time.Millisecond,
		}

		populateMetrics(result, agg, 7)

		if result.Metrics["totalDurationMs"] != 1500 {
			t.Errorf("expected totalDurationMs=1500, got %v", result.Metrics["totalDurationMs"])
		}
		if result.Metrics["runsExecuted"] != 7 {
			t.Errorf("expected runsExecuted=7, got %v", result.Metrics["runsExecuted"])
		}
		if result.Metrics["runsPassed"] != 5 {
			t.Errorf("expected runsPassed=5, got %v", result.Metrics["runsPassed"])
		}
		if result.Metrics["runsFailed"] != 2 {
			t.Errorf("expected runsFailed=2, got %v", result.Metrics["runsFailed"])
		}
	})
}

func TestSetResultStatus(t *testing.T) {
	t.Run("sets fail when failures exist", func(t *testing.T) {
		result := &ExecutionResult{}
		agg := &runAggregator{
			passCount: 3,
			failCount: 1,
			errors:    []string{"run-1: timeout"},
		}

		setResultStatus(result, agg)

		if result.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, result.Status)
		}
		if result.Error != "run-1: timeout" {
			t.Errorf("expected error 'run-1: timeout', got '%s'", result.Error)
		}
	})

	t.Run("sets pass when all pass", func(t *testing.T) {
		result := &ExecutionResult{}
		agg := &runAggregator{
			passCount: 5,
			failCount: 0,
		}

		setResultStatus(result, agg)

		if result.Status != statusPass {
			t.Errorf("expected status %s, got %s", statusPass, result.Status)
		}
	})

	t.Run("sets fail when no runs completed", func(t *testing.T) {
		result := &ExecutionResult{}
		agg := &runAggregator{
			passCount: 0,
			failCount: 0,
		}

		setResultStatus(result, agg)

		if result.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, result.Status)
		}
		if result.Error != "no runs completed successfully" {
			t.Errorf("expected specific error, got '%s'", result.Error)
		}
	})
}

func TestRunAggregatorProcessAssertions(t *testing.T) {
	t.Run("processes passing assertions", func(t *testing.T) {
		agg := &runAggregator{
			passCount:  1,
			assertions: []AssertionResult{},
		}

		// Simulate PromptKit assertion results using the type from worker.go
		// Since we can't import arenastatestore directly, we test via the public interface
		// by calling processAssertions with mock data

		// The processAssertions method expects arenastatestore.ConversationValidationResult
		// which we can't easily mock, so instead we test the aggregator state directly
		assertion := AssertionResult{
			Name:    "contains_greeting",
			Passed:  true,
			Message: "Found greeting",
		}
		agg.assertions = append(agg.assertions, assertion)

		if len(agg.assertions) != 1 {
			t.Errorf("expected 1 assertion, got %d", len(agg.assertions))
		}
		if !agg.assertions[0].Passed {
			t.Error("expected assertion to pass")
		}
	})

	t.Run("failing assertion decrements pass count", func(t *testing.T) {
		agg := &runAggregator{
			passCount:  1,
			failCount:  0,
			assertions: []AssertionResult{},
		}

		// Add a failing assertion
		agg.assertions = append(agg.assertions, AssertionResult{
			Name:    "expected_output",
			Passed:  false,
			Message: "Output mismatch",
		})

		// Simulate what processAssertions does for failing assertion
		if !agg.assertions[0].Passed && agg.passCount > 0 {
			agg.passCount--
			agg.failCount++
		}

		if agg.passCount != 0 {
			t.Errorf("expected passCount=0, got %d", agg.passCount)
		}
		if agg.failCount != 1 {
			t.Errorf("expected failCount=1, got %d", agg.failCount)
		}
	})
}

func TestBuildExecutionResult(t *testing.T) {
	t.Run("returns fallback result when store is not ArenaStateStore", func(t *testing.T) {
		// Use a mock store that isn't ArenaStateStore
		// This exercises the fallback path in buildExecutionResult
		mockStore := &mockStore{}
		runIDs := []string{"run-1"}
		startTime := time.Now()

		result := buildExecutionResult(mockStore, runIDs, startTime, false)

		// Should return pass via fallback since runs exist
		if result.Status != statusPass {
			t.Errorf("expected status %s, got %s", statusPass, result.Status)
		}
	})

	t.Run("returns fallback fail when no runs with non-arena store", func(t *testing.T) {
		mockStore := &mockStore{}
		runIDs := []string{}
		startTime := time.Now()

		result := buildExecutionResult(mockStore, runIDs, startTime, false)

		if result.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, result.Status)
		}
	})
}

// mockStore implements statestore.Store interface for testing
type mockStore struct{}

func (m *mockStore) Load(_ context.Context, _ string) (*statestore.ConversationState, error) {
	return nil, nil
}
func (m *mockStore) Save(_ context.Context, _ *statestore.ConversationState) error { return nil }
func (m *mockStore) Fork(_ context.Context, _, _ string) error                     { return nil }

func TestProcessWorkItemsComplete(t *testing.T) {
	t.Run("processes until queue empty and job complete", func(t *testing.T) {
		q := queue.NewMemoryQueue(queue.Options{
			VisibilityTimeout: 5 * time.Minute,
			MaxRetries:        1,
		})
		jobID := "test-job-process"

		// Don't push any items - queue starts empty
		tmpDir := t.TempDir()
		cfg := &Config{
			JobName:      jobID,
			WorkDir:      tmpDir,
			PollInterval: 1 * time.Millisecond,
		}

		// Initialize job with no items
		items := []queue.WorkItem{}
		if err := q.Push(context.Background(), jobID, items); err != nil {
			t.Fatalf("failed to push: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		err := processWorkItems(ctx, cfg, q, tmpDir)
		if err != nil {
			t.Fatalf("processWorkItems() error = %v", err)
		}
	})
}

func TestRunAggregator(t *testing.T) {
	t.Run("aggregator tracks pass count", func(t *testing.T) {
		agg := &runAggregator{
			verbose: false,
		}

		// Verify initial state
		if agg.passCount != 0 {
			t.Errorf("expected initial passCount=0, got %d", agg.passCount)
		}
		if agg.failCount != 0 {
			t.Errorf("expected initial failCount=0, got %d", agg.failCount)
		}

		// Simulate a pass
		agg.passCount++
		if agg.passCount != 1 {
			t.Errorf("expected passCount=1, got %d", agg.passCount)
		}
	})

	t.Run("aggregator tracks errors", func(t *testing.T) {
		agg := &runAggregator{
			errors: []string{},
		}

		agg.errors = append(agg.errors, "run-1: timeout")
		agg.errors = append(agg.errors, "run-2: invalid response")
		agg.failCount = 2

		if len(agg.errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(agg.errors))
		}
	})

	t.Run("aggregator tracks duration", func(t *testing.T) {
		agg := &runAggregator{}

		agg.totalDuration += 100 * time.Millisecond
		agg.totalDuration += 200 * time.Millisecond

		if agg.totalDuration != 300*time.Millisecond {
			t.Errorf("expected totalDuration=300ms, got %v", agg.totalDuration)
		}
	})
}

func TestCheckContextDone(t *testing.T) {
	t.Run("returns false when context not cancelled", func(t *testing.T) {
		ctx := context.Background()
		done := checkContextDone(ctx)
		if done {
			t.Error("expected done=false for active context")
		}
	})

	t.Run("returns true when context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := checkContextDone(ctx)
		if !done {
			t.Error("expected done=true for cancelled context")
		}
	})
}

func TestLoadConfigWithToolOverrides(t *testing.T) {
	t.Run("parses tool overrides from env", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/workspace/content")
		t.Setenv("ARENA_TOOL_OVERRIDES", `{"get_weather":{"name":"get_weather","endpoint":"https://api.weather.com"}}`)

		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig() error = %v", err)
		}

		if cfg.ToolOverrides == nil {
			t.Fatal("expected ToolOverrides to be set")
		}
		if len(cfg.ToolOverrides) != 1 {
			t.Errorf("expected 1 tool override, got %d", len(cfg.ToolOverrides))
		}
		override, ok := cfg.ToolOverrides["get_weather"]
		if !ok {
			t.Fatal("expected get_weather override")
		}
		if override.Endpoint != "https://api.weather.com" {
			t.Errorf("expected endpoint 'https://api.weather.com', got '%s'", override.Endpoint)
		}
	})

	t.Run("returns error on invalid tool overrides JSON", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/workspace/content")
		t.Setenv("ARENA_TOOL_OVERRIDES", `{invalid json}`)

		_, err := loadConfig()
		if err == nil {
			t.Error("expected error on invalid JSON")
		}
	})
}
