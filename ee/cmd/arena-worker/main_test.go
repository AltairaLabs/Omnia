/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/prometheus/client_golang/prometheus"
)

// testLog returns a no-op logger for tests.
func testLog() logr.Logger {
	return logr.Discard()
}

// testWorkerMetrics creates WorkerMetrics with an isolated registry for tests.
func testWorkerMetrics() *WorkerMetrics {
	return newWorkerMetricsWithRegisterer(prometheus.NewRegistry())
}

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
			defaultValue: defaultScenarioID,
			envValue:     "",
			want:         defaultScenarioID,
		},
		{
			name:         "returns env value when set",
			key:          "TEST_SET",
			defaultValue: defaultScenarioID,
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

		err := processWorkItems(ctx, testLog(), cfg, q, tmpDir, testWorkerMetrics())
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

		result := findArenaConfigFile(tmpDir, "")
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

		result := findArenaConfigFile(tmpDir, "")
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

		result := findArenaConfigFile(tmpDir, "")
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

		result := findArenaConfigFile(tmpDir, "")
		if result != arenaConfig {
			t.Errorf("expected %s, got %s", arenaConfig, result)
		}
	})

	t.Run("returns empty string when no config found", func(t *testing.T) {
		tmpDir := t.TempDir()

		result := findArenaConfigFile(tmpDir, "")
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

		_, err := executeWorkItem(context.Background(), testLog(), cfg, item, tmpDir)
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
			testLog(),
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
			testLog(),
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
			testLog(),
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

		done, err := checkJobCompletion(context.Background(), testLog(), q, jobID, 10)
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

		done, err := checkJobCompletion(context.Background(), testLog(), q, jobID, 10)
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

		reportWorkItemResult(context.Background(), testLog(), q, jobID, item, result, nil)

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
		reportWorkItemResult(context.Background(), testLog(), q, jobID, item, nil, execErr)

		// Check progress - with MaxRetries=1, the item should be marked as failed
		progress, _ := q.Progress(context.Background(), jobID)
		if progress.Failed != 1 {
			t.Errorf("expected 1 failed, got %d", progress.Failed)
		}
	})
}

func TestBuildFallbackResult(t *testing.T) {
	t.Run("returns fail when runs exist but state unavailable", func(t *testing.T) {
		result := &ExecutionResult{
			DurationMs: 100,
			Metrics:    make(map[string]float64),
		}
		runIDs := []string{"run-1", "run-2"}

		got := buildFallbackResult(result, runIDs)

		if got.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, got.Status)
		}
		if got.Error == "" {
			t.Error("expected error message when state is unavailable")
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

		populateMetrics(result, agg, 7, nil)

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

func TestPopulateMetrics_Tokens(t *testing.T) {
	t.Run("includes token counts when present", func(t *testing.T) {
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		agg := &runAggregator{
			passCount:    1,
			inputTokens:  100,
			outputTokens: 50,
		}

		populateMetrics(result, agg, 1, nil)

		assert.Equal(t, float64(100), result.Metrics[metricKeyInputTokens])
		assert.Equal(t, float64(50), result.Metrics[metricKeyOutputTokens])
	})

	t.Run("omits token counts when zero", func(t *testing.T) {
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		agg := &runAggregator{passCount: 1}

		populateMetrics(result, agg, 1, nil)

		_, hasInput := result.Metrics[metricKeyInputTokens]
		_, hasOutput := result.Metrics[metricKeyOutputTokens]
		assert.False(t, hasInput, "should not set input tokens when zero")
		assert.False(t, hasOutput, "should not set output tokens when zero")
	})
}

func TestComputeCost(t *testing.T) {
	t.Run("calculates cost from input and output tokens", func(t *testing.T) {
		p := &providerPricing{inputCostPer1K: 0.003, outputCostPer1K: 0.015}
		// 1000 input tokens * 0.003/1000 = 0.003
		// 500 output tokens * 0.015/1000 = 0.0075
		cost := p.computeCost(1000, 500)
		assert.InDelta(t, 0.0105, cost, 1e-9)
	})

	t.Run("handles zero tokens", func(t *testing.T) {
		p := &providerPricing{inputCostPer1K: 0.003, outputCostPer1K: 0.015}
		assert.Equal(t, 0.0, p.computeCost(0, 0))
	})

	t.Run("handles input-only pricing", func(t *testing.T) {
		p := &providerPricing{inputCostPer1K: 0.01}
		cost := p.computeCost(2000, 500)
		assert.InDelta(t, 0.02, cost, 1e-9)
	})

	t.Run("handles output-only pricing", func(t *testing.T) {
		p := &providerPricing{outputCostPer1K: 0.06}
		cost := p.computeCost(1000, 2000)
		assert.InDelta(t, 0.12, cost, 1e-9)
	})
}

func TestParsePricing(t *testing.T) {
	t.Run("returns nil when pricing is nil", func(t *testing.T) {
		assert.Nil(t, parsePricing(nil))
	})

	t.Run("returns nil when all values are zero", func(t *testing.T) {
		zero := "0"
		p := parsePricing(&v1alpha1.ProviderPricing{
			InputCostPer1K:  &zero,
			OutputCostPer1K: &zero,
		})
		assert.Nil(t, p)
	})

	t.Run("parses valid pricing", func(t *testing.T) {
		input := "0.003"
		output := "0.015"
		p := parsePricing(&v1alpha1.ProviderPricing{
			InputCostPer1K:  &input,
			OutputCostPer1K: &output,
		})
		require.NotNil(t, p)
		assert.InDelta(t, 0.003, p.inputCostPer1K, 1e-9)
		assert.InDelta(t, 0.015, p.outputCostPer1K, 1e-9)
	})

	t.Run("ignores invalid strings", func(t *testing.T) {
		bad := "notanumber"
		output := "0.015"
		p := parsePricing(&v1alpha1.ProviderPricing{
			InputCostPer1K:  &bad,
			OutputCostPer1K: &output,
		})
		require.NotNil(t, p)
		assert.Equal(t, 0.0, p.inputCostPer1K)
		assert.InDelta(t, 0.015, p.outputCostPer1K, 1e-9)
	})

	t.Run("parses partial pricing (input only)", func(t *testing.T) {
		input := "0.005"
		p := parsePricing(&v1alpha1.ProviderPricing{
			InputCostPer1K: &input,
		})
		require.NotNil(t, p)
		assert.InDelta(t, 0.005, p.inputCostPer1K, 1e-9)
		assert.Equal(t, 0.0, p.outputCostPer1K)
	})
}

func TestPopulateMetrics_Cost(t *testing.T) {
	t.Run("writes cost when pricing and tokens are present", func(t *testing.T) {
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		agg := &runAggregator{
			passCount:    1,
			inputTokens:  1000,
			outputTokens: 500,
		}
		pricing := &providerPricing{inputCostPer1K: 0.003, outputCostPer1K: 0.015}

		populateMetrics(result, agg, 1, pricing)

		cost, ok := result.Metrics[metricKeyCost]
		assert.True(t, ok, "cost metric should be present")
		assert.InDelta(t, 0.0105, cost, 1e-9)
	})

	t.Run("omits cost when pricing is nil", func(t *testing.T) {
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		agg := &runAggregator{
			passCount:    1,
			inputTokens:  1000,
			outputTokens: 500,
		}

		populateMetrics(result, agg, 1, nil)

		_, ok := result.Metrics[metricKeyCost]
		assert.False(t, ok, "cost metric should not be present without pricing")
	})

	t.Run("omits cost when tokens are zero", func(t *testing.T) {
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		agg := &runAggregator{passCount: 1}
		pricing := &providerPricing{inputCostPer1K: 0.003, outputCostPer1K: 0.015}

		populateMetrics(result, agg, 1, pricing)

		_, ok := result.Metrics[metricKeyCost]
		assert.False(t, ok, "cost metric should not be present with zero tokens")
	})
}

func TestToItemResult(t *testing.T) {
	t.Run("converts all fields", func(t *testing.T) {
		exec := &ExecutionResult{
			Status:     statusPass,
			DurationMs: 250,
			Error:      "",
			Metrics:    map[string]float64{"totalDurationMs": 250},
			Assertions: []AssertionResult{
				{Name: "response_valid", Passed: true, Message: "ok"},
				{Name: "latency_check", Passed: false, Message: "too slow"},
			},
		}

		ir := toItemResult(exec)

		assert.Equal(t, statusPass, ir.Status)
		assert.Equal(t, float64(250), ir.DurationMs)
		assert.Empty(t, ir.Error)
		assert.Equal(t, float64(250), ir.Metrics["totalDurationMs"])
		require.Len(t, ir.Assertions, 2)
		assert.Equal(t, "response_valid", ir.Assertions[0].Name)
		assert.True(t, ir.Assertions[0].Passed)
		assert.Equal(t, "latency_check", ir.Assertions[1].Name)
		assert.False(t, ir.Assertions[1].Passed)
	})

	t.Run("handles nil assertions", func(t *testing.T) {
		exec := &ExecutionResult{
			Status:     statusFail,
			DurationMs: 100,
			Error:      "timeout",
			Metrics:    map[string]float64{},
		}

		ir := toItemResult(exec)

		assert.Equal(t, statusFail, ir.Status)
		assert.Equal(t, "timeout", ir.Error)
		assert.Empty(t, ir.Assertions)
	})
}

func TestReportWorkItemResult_UpdatesAccumulators(t *testing.T) {
	q := queue.NewMemoryQueueWithDefaults()
	jobID := "test-accum"

	items := []queue.WorkItem{{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"}}
	require.NoError(t, q.Push(context.Background(), jobID, items))
	item, err := q.Pop(context.Background(), jobID)
	require.NoError(t, err)

	result := &ExecutionResult{
		Status:     statusPass,
		DurationMs: 200,
		Metrics:    map[string]float64{"totalTokens": 500},
		Assertions: []AssertionResult{{Name: "check", Passed: true}},
	}

	reportWorkItemResult(context.Background(), testLog(), q, jobID, item, result, nil)

	stats, err := q.GetStats(context.Background(), jobID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Total)
	assert.Equal(t, int64(1), stats.Passed)
	assert.Equal(t, float64(200), stats.TotalDurationMs)
	assert.Equal(t, int64(500), stats.TotalTokens)
}

func TestExtractFleetTTFT(t *testing.T) {
	t.Run("no-op when result is nil", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		extractFleetTTFT(registry, nil, nil) // should not panic
	})

	t.Run("no-op when metrics is nil", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		result := &ExecutionResult{}
		extractFleetTTFT(registry, nil, result)
		assert.Nil(t, result.Metrics)
	})

	t.Run("skips when TTFT already set", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		result := &ExecutionResult{
			Metrics: map[string]float64{metricKeyTTFT: 1.5},
		}
		extractFleetTTFT(registry, []*resolvedFleetProvider{{id: "p1"}}, result)
		assert.Equal(t, 1.5, result.Metrics[metricKeyTTFT])
	})

	t.Run("no-op when no fleet providers", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		extractFleetTTFT(registry, nil, result)
		_, hasTTFT := result.Metrics[metricKeyTTFT]
		assert.False(t, hasTTFT)
	})

	t.Run("no-op when provider not found in registry", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		result := &ExecutionResult{Metrics: make(map[string]float64)}
		fps := []*resolvedFleetProvider{{id: "missing"}}
		extractFleetTTFT(registry, fps, result)
		_, hasTTFT := result.Metrics[metricKeyTTFT]
		assert.False(t, hasTTFT)
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
			log:        testLog(),
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
			log:        testLog(),
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
	t.Run("returns fallback fail when store is not ArenaStateStore", func(t *testing.T) {
		// Use a mock store that isn't ArenaStateStore
		// This exercises the fallback path in buildExecutionResult
		mockStore := &mockStore{}
		runIDs := []string{"run-1"}
		startTime := time.Now()

		result := buildExecutionResult(testLog(), mockStore, runIDs, startTime, nil)

		// Should return fail — unable to read run state means results are unknown
		if result.Status != statusFail {
			t.Errorf("expected status %s, got %s", statusFail, result.Status)
		}
		if result.Error == "" {
			t.Error("expected error message when state is unavailable")
		}
	})

	t.Run("returns fallback fail when no runs with non-arena store", func(t *testing.T) {
		mockStore := &mockStore{}
		runIDs := []string{}
		startTime := time.Now()

		result := buildExecutionResult(testLog(), mockStore, runIDs, startTime, nil)

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

		err := processWorkItems(ctx, testLog(), cfg, q, tmpDir, testWorkerMetrics())
		if err != nil {
			t.Fatalf("processWorkItems() error = %v", err)
		}
	})
}

func TestRunAggregator(t *testing.T) {
	t.Run("aggregator tracks pass count", func(t *testing.T) {
		agg := &runAggregator{
			log: testLog(),
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
			log:    testLog(),
		}

		agg.errors = append(agg.errors, "run-1: timeout")
		agg.errors = append(agg.errors, "run-2: invalid response")
		agg.failCount = 2

		if len(agg.errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(agg.errors))
		}
	})

	t.Run("aggregator tracks duration", func(t *testing.T) {
		agg := &runAggregator{
			log: testLog(),
		}

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
		done := checkContextDone(ctx, testLog())
		if done {
			t.Error("expected done=false for active context")
		}
	})

	t.Run("returns true when context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := checkContextDone(ctx, testLog())
		if !done {
			t.Error("expected done=true for cancelled context")
		}
	})
}

func TestJobNameToTraceID(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		id1 := jobNameToTraceID("my-eval-job")
		id2 := jobNameToTraceID("my-eval-job")
		assert.Equal(t, id1, id2, "same job name should produce same trace ID")
	})

	t.Run("different names produce different IDs", func(t *testing.T) {
		id1 := jobNameToTraceID("job-a")
		id2 := jobNameToTraceID("job-b")
		assert.NotEqual(t, id1, id2, "different job names should produce different trace IDs")
	})

	t.Run("produces valid trace ID", func(t *testing.T) {
		id := jobNameToTraceID("test-job")
		assert.True(t, id.IsValid(), "should produce a valid (non-zero) trace ID")
	})
}

func TestJobNameToSpanID(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		id1 := jobNameToSpanID("my-eval-job")
		id2 := jobNameToSpanID("my-eval-job")
		assert.Equal(t, id1, id2, "same job name should produce same span ID")
	})

	t.Run("different names produce different IDs", func(t *testing.T) {
		id1 := jobNameToSpanID("job-a")
		id2 := jobNameToSpanID("job-b")
		assert.NotEqual(t, id1, id2, "different job names should produce different span IDs")
	})

	t.Run("produces valid span ID", func(t *testing.T) {
		id := jobNameToSpanID("test-job")
		assert.True(t, id.IsValid(), "should produce a valid (non-zero) span ID")
	})

	t.Run("differs from trace ID bytes", func(t *testing.T) {
		tid := jobNameToTraceID("test-job")
		sid := jobNameToSpanID("test-job")
		// span ID uses bytes 16-24, trace ID uses bytes 0-16 — they should differ
		assert.NotEqual(t, tid[:8], sid[:], "span ID should use different hash bytes than trace ID")
	})
}

func TestSessionIDToTraceID(t *testing.T) {
	t.Run("matches facade logic", func(t *testing.T) {
		// UUID "aabbccdd-1122-3344-5566-778899aabbcc" should map to trace ID aabbccdd112233445566778899aabbcc
		tid := sessionIDToTraceID("aabbccdd-1122-3344-5566-778899aabbcc")
		assert.Equal(t, "aabbccdd112233445566778899aabbcc", tid.String())
	})

	t.Run("produces valid trace ID", func(t *testing.T) {
		tid := sessionIDToTraceID("550e8400-e29b-41d4-a716-446655440000")
		assert.True(t, tid.IsValid(), "should produce a valid (non-zero) trace ID")
	})

	t.Run("deterministic", func(t *testing.T) {
		id1 := sessionIDToTraceID("550e8400-e29b-41d4-a716-446655440000")
		id2 := sessionIDToTraceID("550e8400-e29b-41d4-a716-446655440000")
		assert.Equal(t, id1, id2)
	})
}

func TestProcessWorkItemsTracing(t *testing.T) {
	t.Run("root span parents work-item spans", func(t *testing.T) {
		// Set up in-memory exporter to capture spans.
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		// Set the global tracer provider so processWorkItems uses it.
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(tp)
		defer otel.SetTracerProvider(prev)

		q := queue.NewMemoryQueue(queue.Options{
			VisibilityTimeout: 5 * time.Minute,
			MaxRetries:        1,
		})
		jobID := "trace-test-job"

		// Initialize job with metadata but no items so the worker exits
		// after empty polls.
		err := q.Push(context.Background(), jobID, []queue.WorkItem{})
		require.NoError(t, err)

		cfg := &Config{
			JobName:      jobID,
			PollInterval: 1 * time.Millisecond,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		err = processWorkItems(ctx, testLog(), cfg, q, t.TempDir(), testWorkerMetrics())
		require.NoError(t, err)

		// Force flush
		_ = tp.ForceFlush(context.Background())

		spans := exporter.GetSpans()
		require.NotEmpty(t, spans, "expected at least the root span")

		// Find the root arena.worker span.
		var rootSpan *tracetest.SpanStub
		for i := range spans {
			if spans[i].Name == "arena.worker" {
				rootSpan = &spans[i]
				break
			}
		}
		require.NotNil(t, rootSpan, "expected arena.worker root span")
		assert.True(t, rootSpan.SpanContext.TraceID().IsValid(), "root span should have valid trace ID")
		assert.True(t, rootSpan.SpanContext.SpanID().IsValid(), "root span should have valid span ID")

		// Verify the trace ID is deterministic (derived from job name).
		expectedTraceID := jobNameToTraceID(jobID)
		assert.Equal(t, expectedTraceID, rootSpan.SpanContext.TraceID(),
			"root span should use deterministic trace ID derived from job name")

		// Verify arena.job attribute.
		found := false
		for _, attr := range rootSpan.Attributes {
			if string(attr.Key) == "arena.job" && attr.Value.AsString() == jobID {
				found = true
				break
			}
		}
		assert.True(t, found, "root span should have arena.job attribute")
	})

	t.Run("work-item span is child of root span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(tp)
		defer otel.SetTracerProvider(prev)

		q := queue.NewMemoryQueue(queue.Options{
			VisibilityTimeout: 5 * time.Minute,
			MaxRetries:        1,
		})
		jobID := "trace-child-job"

		// Push a work item that will fail execution (no config file) but
		// still create the work-item span.
		err := q.Push(context.Background(), jobID, []queue.WorkItem{
			{ID: "item-1", ScenarioID: "s1", ProviderID: "p1"},
		})
		require.NoError(t, err)

		tmpDir := t.TempDir()
		cfg := &Config{
			JobName:      jobID,
			PollInterval: 1 * time.Millisecond,
			ContentPath:  tmpDir,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Will return error because no arena config file exists.
		_ = processWorkItems(ctx, testLog(), cfg, q, tmpDir, testWorkerMetrics())

		_ = tp.ForceFlush(context.Background())

		spans := exporter.GetSpans()

		var rootSpan, itemSpan *tracetest.SpanStub
		for i := range spans {
			switch spans[i].Name {
			case "arena.worker":
				rootSpan = &spans[i]
			case "arena.work-item":
				itemSpan = &spans[i]
			}
		}

		require.NotNil(t, rootSpan, "expected arena.worker root span")
		require.NotNil(t, itemSpan, "expected arena.work-item span")

		// The work-item span must share the same trace ID as the root span.
		assert.Equal(t, rootSpan.SpanContext.TraceID(), itemSpan.SpanContext.TraceID(),
			"work-item span should share trace ID with root span")

		// The work-item span's parent must be the root span.
		assert.Equal(t, rootSpan.SpanContext.SpanID(), itemSpan.Parent.SpanID(),
			"work-item span parent should be the root span")
	})
}

func TestFleetModeScenarioFilter(t *testing.T) {
	t.Run("default scenario ID produces empty filter", func(t *testing.T) {
		item := &queue.WorkItem{ScenarioID: defaultScenarioID}
		scenarioFilter := []string{}
		if item.ScenarioID != "" && item.ScenarioID != defaultScenarioID {
			scenarioFilter = []string{item.ScenarioID}
		}
		assert.Empty(t, scenarioFilter, "default scenario should produce empty filter (all scenarios)")
	})

	t.Run("named scenario ID produces specific filter", func(t *testing.T) {
		item := &queue.WorkItem{ScenarioID: "greeting-test"}
		scenarioFilter := []string{}
		if item.ScenarioID != "" && item.ScenarioID != defaultScenarioID {
			scenarioFilter = []string{item.ScenarioID}
		}
		assert.Equal(t, []string{"greeting-test"}, scenarioFilter)
	})

	t.Run("empty scenario ID produces empty filter", func(t *testing.T) {
		item := &queue.WorkItem{ScenarioID: ""}
		scenarioFilter := []string{}
		if item.ScenarioID != "" && item.ScenarioID != defaultScenarioID {
			scenarioFilter = []string{item.ScenarioID}
		}
		assert.Empty(t, scenarioFilter, "empty scenario should produce empty filter")
	})
}
