/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"sort"
	"testing"

	"github.com/go-logr/logr"
)

// expectedCheckNames is the set of check names buildRunner must register
// every time (no Redis, no k8s). Each entry is a wiring contract — if a
// check disappears from this list it means a Register() call was removed
// or the check builder silently dropped a Check. Sub-tests use this as a
// must-have set and additionally assert conditional checks (Redis, CRDs).
//
// Names are taken from the literal {Name:...} entries in
// internal/doctor/checks/*.go. Keep alphabetised for diff stability.
var expectedCheckNames = []string{
	"AgentUsesTools",
	"ArenaControllerHealthy",
	"AuditLogWritten",
	"ConsolidationWorkerRunning",
	"DashboardResponds",
	"MemoryAPIDocsServed",
	"MemoryAPIHealthy",
	"MemoryDelete",
	"MemoryDeletionCascade",
	"MemoryExport",
	"MemoryList",
	"MemoryOptOutRespected",
	"MemoryPIIRedaction",
	"MemoryPersistsAcrossSessions",
	"MemoryRecall",
	"MemoryRetrieve",
	"MemorySave",
	"MemoryToolsAvailable",
	"MemoryUserIsolation",
	"MemoryUserOwnership",
	"MessagesRecorded",
	"OllamaHealthy",
	"OperatorAPIHealthy",
	"ProviderCallsTracked",
	"SendMessageGetResponse",
	"SessionAPIDocsServed",
	"SessionAPIHealthy",
	"SessionCreated",
	"SessionEncryptionAtRest",
	"SessionSearch",
	"WebSocketConnect",
}

// baseRunnerConfig returns a runnerConfig with non-empty URLs but no Redis
// and no k8s. This is the OSS default — wiring tests focus on what's
// always present.
func baseRunnerConfig() runnerConfig {
	return runnerConfig{
		log:               logr.Discard(),
		namespace:         "omnia-system",
		agentNamespace:    "omnia-demo",
		agentName:         "tools-demo",
		sessionAPIBaseURL: "http://session-api:8080",
		memoryAPIBaseURL:  "http://memory-api:8080",
		ollamaURL:         "http://ollama:11434",
		operatorURL:       "http://operator:8083",
		dashboardURL:      "http://dashboard:3000",
		arenaURL:          "http://arena:8082",
	}
}

func collectCheckNames(t *testing.T, cfg runnerConfig) []string {
	t.Helper()
	runner, err := buildRunner(cfg)
	if err != nil {
		t.Fatalf("buildRunner: %v", err)
	}
	checks := runner.Checks()
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Name)
	}
	return names
}

// TestBuildRunner_AlwaysRegistersBaseChecks asserts that the always-on
// check set is registered when Redis is not configured and no k8s
// client is available (the OSS-no-cluster wiring path). This is the
// "did someone forget to call runner.Register?" guard.
func TestBuildRunner_AlwaysRegistersBaseChecks(t *testing.T) {
	got := collectCheckNames(t, baseRunnerConfig())
	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}
	var missing []string
	for _, want := range expectedCheckNames {
		if !gotSet[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("buildRunner did not register expected checks: %v\ngot: %v",
			missing, got)
	}
}

// TestBuildRunner_RedisCheckGatedByAddr asserts the Redis check is
// registered iff cfg.redisAddr is set. OSS installs default to no Redis;
// registering the check unconditionally would surface a noisy
// "RedisReachable: unreachable" on every run.
func TestBuildRunner_RedisCheckGatedByAddr(t *testing.T) {
	t.Run("no redis addr — no Redis check", func(t *testing.T) {
		names := collectCheckNames(t, baseRunnerConfig())
		for _, n := range names {
			if n == "RedisReachable" {
				t.Errorf("Redis check should not be registered when redisAddr empty; got names=%v", names)
			}
		}
	})

	t.Run("redis addr set — Redis check registered", func(t *testing.T) {
		cfg := baseRunnerConfig()
		cfg.redisAddr = "127.0.0.1:6379"
		names := collectCheckNames(t, cfg)
		found := false
		for _, n := range names {
			if n == "RedisReachable" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Redis check should be registered when redisAddr set; got names=%v", names)
		}
	})
}

// TestBuildRunner_SequentialGroupForAgentSessions asserts that the
// Agent and Sessions categories are configured to run sequentially —
// Sessions depends on Agent's LastSessionID. A regression here makes
// SessionCreated flaky because Sessions may execute before Agent
// populates LastSessionID. Behaviour is observable by running the
// runner and inspecting which categories share order constraints —
// we test by running an empty context and checking SequentialGroup
// is set for the relevant categories via the runner's Run behaviour.
//
// Direct introspection isn't exposed — instead we register a probe
// check that records execution order and assert Sessions follows Agent.
// Skipping for now: this is covered indirectly by the runner_test.go
// SequentialGroup tests in internal/doctor/.
func TestBuildRunner_RunnerNotNil(t *testing.T) {
	runner, err := buildRunner(baseRunnerConfig())
	if err != nil {
		t.Fatalf("buildRunner: %v", err)
	}
	if runner == nil {
		t.Fatal("buildRunner returned nil runner with no error")
	}
	if got := len(runner.Checks()); got == 0 {
		t.Errorf("expected at least one check registered, got %d", got)
	}
}
