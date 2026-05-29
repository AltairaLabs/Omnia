/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package checks

import (
	"context"
	"strings"

	"github.com/altairalabs/omnia/internal/doctor"
)

// consolidationGaugeNeedle matches the worker-liveness series for the
// consolidation worker in Prometheus text exposition. The series only
// exists once the worker has called MarkWorkerRunning/Stopped, i.e. only
// when consolidation is enabled (interval > 0).
const consolidationGaugeNeedle = `omnia_memory_worker_running{name="consolidation"}`

// ConsolidationChecker verifies the memory consolidation worker's liveness
// via the omnia_memory_worker_running gauge on memory-api's /metrics.
type ConsolidationChecker struct {
	memoryAPIURL string
}

// NewConsolidationChecker constructs a checker against the given memory-api
// base URL (no trailing /metrics — fetchMetrics appends it).
func NewConsolidationChecker(memoryAPIURL string) *ConsolidationChecker {
	return &ConsolidationChecker{memoryAPIURL: memoryAPIURL}
}

// Checks returns the consolidation Doctor checks.
func (c *ConsolidationChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "ConsolidationWorkerRunning", Category: "Memory", Run: c.checkWorkerRunning},
	}
}

// checkWorkerRunning scrapes the worker-liveness gauge:
//   - series absent -> SKIP (consolidation not enabled in this environment)
//   - value 1        -> PASS (worker loop alive)
//   - value 0        -> FAIL (worker started then exited)
func (c *ConsolidationChecker) checkWorkerRunning(ctx context.Context) doctor.TestResult {
	if c.memoryAPIURL == "" {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "no memory-api URL configured"}
	}
	body, err := fetchMetrics(ctx, c.memoryAPIURL)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "failed to fetch memory-api metrics"}
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, consolidationGaugeNeedle) {
			continue
		}
		// Line shape: omnia_memory_worker_running{name="consolidation"} 1
		fields := strings.Fields(line)
		value := ""
		if len(fields) > 0 {
			value = fields[len(fields)-1]
		}
		if value == "1" {
			return doctor.TestResult{Status: doctor.StatusPass, Detail: "consolidation worker running"}
		}
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "consolidation worker not running (gauge=" + value + ")"}
	}
	return doctor.TestResult{Status: doctor.StatusSkip, Detail: "consolidation worker not enabled (gauge series absent)"}
}
