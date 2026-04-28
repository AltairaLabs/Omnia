/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package memory

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// workerRunning is the canonical "this worker is alive" gauge for the
// memory subsystem. Each worker Sets the gauge to 1 the moment its
// Run loop starts and Sets it back to 0 in a deferred call when the
// loop exits — voluntarily on context cancel, or involuntarily on
// panic-recovery / fatal error.
//
// Why a per-worker gauge: issue #1038 surfaced five wiring failures
// where the worker code was wired in `cmd/memory-api/main.go` but
// silently never ran (operator didn't pass the enabling flags,
// MemoryPolicy CRD didn't exist, etc.). Without a runtime signal
// you couldn't tell from the outside whether a worker was working
// or simply absent. Alert rule:
//
//	expr: max by (name) (omnia_memory_worker_running) == 0
//	for:  10m
//	annotations:
//	  summary: "memory worker {{ $labels.name }} not running"
//
// Use the shared startWorker / stopWorker helpers below so every
// worker emits the gauge consistently and the name labels match the
// alert rules.
var workerRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "omnia_memory_worker_running",
	Help: "1 when the named memory worker's Run loop is active, 0 when it has exited or never started. Used to alert on workers that are wired but inert (issue #1038).",
}, []string{"name"})

// MarkWorkerRunning flips the liveness gauge to 1 for the named
// worker. Call this at the top of a worker's Run() method, before
// any work that could fail. Pair with a deferred MarkWorkerStopped.
func MarkWorkerRunning(name string) {
	workerRunning.WithLabelValues(name).Set(1)
}

// MarkWorkerStopped flips the liveness gauge to 0 for the named
// worker. Call this in a defer at the top of Run() so the gauge
// reflects loop exit regardless of how Run returns (context cancel,
// panic recovery, or normal completion).
func MarkWorkerStopped(name string) {
	workerRunning.WithLabelValues(name).Set(0)
}

// Worker name constants — keep alert rules and dashboards in sync
// by centralising the labels here. Adding a new worker? Add a const
// here, call Mark{Running,Stopped} in its Run loop, and update the
// alert rule's expected-on list.
const (
	WorkerNameCompaction      = "compaction"
	WorkerNameTombstoneGC     = "tombstone_gc"
	WorkerNameReembed         = "reembed"
	WorkerNameRetention       = "retention"
	WorkerNameAnalyticsOptIn  = "analytics_opt_in"
	WorkerNameAccessTouchFlow = "access_touch_flush"
)
