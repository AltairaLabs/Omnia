/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/threshold"
	"github.com/altairalabs/omnia/pkg/intconv"
)

// updateStatusFromJob updates the ArenaJob status based on the K8s Job status.
func (r *ArenaJobReconciler) updateStatusFromJob(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, job *batchv1.Job) {
	// Only update ActiveWorkers when it changes to avoid unnecessary CRD writes.
	// Live work-item progress is served via SSE from Redis stats.
	arenaJob.Status.ActiveWorkers = job.Status.Active

	// Check job conditions
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			r.handleJobComplete(ctx, arenaJob)
		case batchv1.JobFailed:
			r.handleJobFailed(ctx, arenaJob, condition)
		}
	}

	// If job is still running, check budget and update progress
	if arenaJob.Status.Phase == omniav1alpha1.ArenaJobPhaseRunning {
		r.checkBudgetLimit(ctx, arenaJob)
	}

	// The Progressing condition is set at creation ("Job is running") and
	// updated only on completion or budget breach. Live progress comes from
	// SSE/Redis — we don't rewrite the condition message on every reconcile.
}

// handleJobComplete updates status when the underlying K8s Job reports
// JobComplete. It aggregates results, sets final progress, evaluates load-test
// SLO thresholds, and picks the terminal phase from the aggregated outcome.
func (r *ArenaJobReconciler) handleJobComplete(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) {
	log := logf.FromContext(ctx)

	now := metav1.Now()
	arenaJob.Status.CompletionTime = &now

	// Aggregate results from queue if aggregator is available
	// The aggregated results determine actual success/failure based on test outcomes
	var hasTestFailures bool
	var hasAggregation bool
	var passedItems, failedItems int
	if r.Aggregator != nil {
		log.V(1).Info("aggregating results", "jobID", arenaJob.Name)
		result := r.aggregateJobResults(ctx, arenaJob.Name)
		if result != nil {
			hasAggregation = true
			log.V(1).Info("aggregation complete",
				"totalItems", result.TotalItems,
				"passedItems", result.PassedItems,
				"failedItems", result.FailedItems)
			arenaJob.Status.Result = r.Aggregator.ToJobResult(result)
			hasTestFailures = result.FailedItems > 0
			passedItems = result.PassedItems
			failedItems = result.FailedItems
		}
	} else {
		log.V(1).Info("aggregator not available, skipping result aggregation")
	}

	// Set final progress counts from aggregation or queue stats.
	// Lazy-init Progress so the struct is always populated on a
	// terminal phase — callers (and jsonpath queries) can rely on
	// .status.progress existing once the job reaches Succeeded or
	// Failed, even if createWorkerJob never ran in this reconcile.
	if arenaJob.Status.Progress == nil {
		arenaJob.Status.Progress = &omniav1alpha1.JobProgress{}
	}
	if hasAggregation {
		arenaJob.Status.Progress.Completed = int32(passedItems)
		arenaJob.Status.Progress.Failed = int32(failedItems)
		arenaJob.Status.Progress.Pending = 0
	} else if r.Queue != nil {
		if stats, err := r.Queue.GetStats(ctx, arenaJob.Name); err == nil && stats != nil {
			arenaJob.Status.Progress.Completed = intconv.ClampInt32(stats.Passed)
			arenaJob.Status.Progress.Failed = intconv.ClampInt32(stats.Failed)
			arenaJob.Status.Progress.Pending = 0
		}
	}

	// Evaluate SLO thresholds for load tests
	if r.evaluateLoadTestThresholds(ctx, arenaJob) {
		hasTestFailures = true
	}

	// Set phase based on aggregated test results, not just K8s job completion
	r.setCompletionPhase(ctx, arenaJob, hasTestFailures, hasAggregation, passedItems, failedItems)
}

// setCompletionPhase picks the terminal phase and conditions for a completed
// job from the aggregated test outcome.
func (r *ArenaJobReconciler) setCompletionPhase(
	ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, hasTestFailures, hasAggregation bool, passedItems, failedItems int,
) {
	log := logf.FromContext(ctx)
	if hasTestFailures {
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
			"TestsFailed", "Job completed but some tests failed")
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
			"Failed", "Job completed but some tests failed")
		if r.Recorder != nil {
			r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
				"Job completed but some tests failed")
		}
		log.Info("job completed with test failures",
			"passed", passedItems,
			"failed", failedItems)
	} else if hasAggregation && passedItems == 0 {
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
			"NoTestsRan", "Job completed but no tests produced results")
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
			"Failed", "Job completed but no tests produced results")
		if r.Recorder != nil {
			r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
				"Job completed but no tests produced results")
		}
		log.Info("job completed with no test results",
			"passed", passedItems,
			"failed", failedItems)
	} else {
		arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseSucceeded
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
			"JobSucceeded", "Job completed successfully")
		SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionTrue,
			"Succeeded", "Job completed successfully")
		if r.Recorder != nil {
			r.Recorder.Event(arenaJob, corev1.EventTypeNormal, ArenaJobEventReasonJobSucceeded,
				"Job completed successfully")
		}
		log.Info("job completed successfully",
			"passed", passedItems)
	}
}

// handleJobFailed updates status when the underlying K8s Job reports JobFailed.
func (r *ArenaJobReconciler) handleJobFailed(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, condition batchv1.JobCondition) {
	log := logf.FromContext(ctx)

	arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
	now := metav1.Now()
	arenaJob.Status.CompletionTime = &now
	// Lazy-init Progress so callers can rely on .status.progress
	// existing once the job reaches a terminal phase, even when
	// the failure happened before any work items were enqueued.
	if arenaJob.Status.Progress == nil {
		arenaJob.Status.Progress = &omniav1alpha1.JobProgress{}
	}
	// Pull final counts from the queue if we have one; otherwise
	// leave the zero values (which now serialize thanks to the
	// JobProgress field tag change).
	if r.Queue != nil {
		if stats, err := r.Queue.GetStats(ctx, arenaJob.Name); err == nil && stats != nil {
			arenaJob.Status.Progress.Completed = intconv.ClampInt32(stats.Passed)
			arenaJob.Status.Progress.Failed = intconv.ClampInt32(stats.Failed)
			arenaJob.Status.Progress.Pending = 0
		}
	}
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
		"JobFailed", condition.Message)
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
		"Failed", condition.Message)
	if r.Recorder != nil {
		r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonJobFailed,
			fmt.Sprintf("Job failed: %s", condition.Message))
	}
	log.Info("job failed", "reason", condition.Reason, "message", condition.Message)
}

// evaluateLoadTestThresholds checks SLO thresholds for load test jobs.
// Returns true if any threshold was violated.
func (r *ArenaJobReconciler) evaluateLoadTestThresholds(
	ctx context.Context, arenaJob *omniav1alpha1.ArenaJob,
) bool {
	log := logf.FromContext(ctx)

	if arenaJob.Spec.LoadTest == nil || len(arenaJob.Spec.LoadTest.Thresholds) == 0 {
		return false
	}
	if r.Queue == nil {
		log.V(1).Info("queue not available, skipping threshold evaluation")
		return false
	}

	stats, err := r.Queue.GetStats(ctx, arenaJob.Name)
	if err != nil {
		log.Error(err, "threshold evaluation failed to get stats")
		return false
	}

	results, allPassed := threshold.Evaluate(arenaJob.Spec.LoadTest.Thresholds, stats)
	r.writeThresholdResults(arenaJob, results)

	if !allPassed {
		log.Info("SLO thresholds violated",
			"summary", threshold.SummaryLine(results))
	} else {
		log.V(1).Info("SLO thresholds passed",
			"summary", threshold.SummaryLine(results))
	}

	return !allPassed
}

// writeThresholdResults adds threshold evaluation results to the job status summary.
func (r *ArenaJobReconciler) writeThresholdResults(
	arenaJob *omniav1alpha1.ArenaJob, results []threshold.Result,
) {
	if arenaJob.Status.Result == nil {
		arenaJob.Status.Result = &omniav1alpha1.JobResult{}
	}
	if arenaJob.Status.Result.Summary == nil {
		arenaJob.Status.Result.Summary = make(map[string]string)
	}
	summary := arenaJob.Status.Result.Summary

	for _, r := range results {
		key := "threshold:" + r.Metric
		summary[key] = r.String()
	}
	summary["thresholds_passed"] = threshold.SummaryLine(results)
}

// aggregateJobResults tries stats-based aggregation first (O(1)), then falls
// back to item-level Aggregate when stats are unavailable.
func (r *ArenaJobReconciler) aggregateJobResults(
	ctx context.Context, jobID string,
) *aggregator.AggregatedResult {
	log := logf.FromContext(ctx)

	// Prefer stats-based path (O(1) — reads accumulators, not individual items)
	if r.Queue != nil {
		stats, err := r.Queue.GetStats(ctx, jobID)
		if err == nil && stats.Passed+stats.Failed > 0 {
			log.V(1).Info("using stats-based aggregation", "jobID", jobID)
			return aggregator.StatsToResult(stats)
		}
		if err != nil {
			log.V(1).Info("stats unavailable, falling back to item-level aggregation",
				"jobID", jobID, "error", err)
		}
	}

	// Fall back to item-level aggregation for detailed error/assertion data
	result, err := r.Aggregator.Aggregate(ctx, jobID)
	if err != nil {
		log.Error(err, "failed to aggregate results")
		return nil
	}
	return result
}

// checkBudgetLimit checks if a running load test has exceeded its budget limit.
// When the cost accumulator exceeds the configured budgetLimit, the job phase is
// set to Failed and summary details are populated with cost information.
// This is a no-op for non-loadtest jobs or jobs without a budget limit configured.
func (r *ArenaJobReconciler) checkBudgetLimit(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob) {
	if arenaJob.Spec.LoadTest == nil || arenaJob.Spec.LoadTest.BudgetLimit == nil {
		return
	}
	if r.Queue == nil {
		return
	}

	log := logf.FromContext(ctx)
	stats, err := r.Queue.GetStats(ctx, arenaJob.Name)
	if err != nil {
		log.V(1).Info("budget check skipped",
			"reason", "failed to get stats",
			"error", err)
		return
	}

	currency := arenaJob.Spec.LoadTest.BudgetCurrency
	if currency == "" {
		currency = "USD"
	}

	result := checkBudget(*arenaJob.Spec.LoadTest.BudgetLimit, currency, stats)
	if !result.Breached {
		return
	}

	log.Info("budget limit exceeded",
		"totalCost", stats.TotalCost,
		"budgetLimit", *arenaJob.Spec.LoadTest.BudgetLimit,
		"budgetCurrency", currency)

	now := metav1.Now()
	arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
	arenaJob.Status.CompletionTime = &now

	if arenaJob.Status.Result == nil {
		arenaJob.Status.Result = &omniav1alpha1.JobResult{}
	}
	if arenaJob.Status.Result.Summary == nil {
		arenaJob.Status.Result.Summary = make(map[string]string)
	}
	for k, v := range result.Details {
		arenaJob.Status.Result.Summary[k] = v
	}

	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation,
		ArenaJobConditionTypeProgressing, metav1.ConditionFalse,
		"BudgetExceeded", fmt.Sprintf("Cost %.2f %s exceeds budget limit %s %s",
			stats.TotalCost, currency, *arenaJob.Spec.LoadTest.BudgetLimit, currency))
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation,
		ArenaJobConditionTypeReady, metav1.ConditionFalse,
		"BudgetExceeded", "Job stopped: budget limit exceeded")

	if r.Recorder != nil {
		r.Recorder.Event(arenaJob, corev1.EventTypeWarning, "BudgetExceeded",
			fmt.Sprintf("Load test stopped: cost %.2f %s exceeds budget limit %s %s",
				stats.TotalCost, currency, *arenaJob.Spec.LoadTest.BudgetLimit, currency))
	}
}

// handleSourceNotReady marks the job as waiting (non-terminal) for its
// referenced ArenaSource. Unlike a terminal failure it keeps Phase=Pending, so
// the job is re-reconciled — via requeue or the ArenaSource watch — and proceeds
// once the source becomes available. Setting Phase=Failed here would strand the
// job permanently: the terminal-phase skip guard in Reconcile would ignore every
// subsequent reconcile, including the watch that fires when the source appears.
func (r *ArenaJobReconciler) handleSourceNotReady(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, err error) {
	log := logf.FromContext(ctx)

	arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhasePending
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeSourceValid, metav1.ConditionFalse, "SourceNotReady", err.Error())
	SetCondition(&arenaJob.Status.Conditions, arenaJob.Generation, ArenaJobConditionTypeReady, metav1.ConditionFalse,
		"SourceNotReady", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(arenaJob, corev1.EventTypeWarning, ArenaJobEventReasonConfigNotReady, err.Error())
	}

	if statusErr := r.Status().Update(ctx, arenaJob); statusErr != nil {
		log.Error(statusErr, "failed to update status while waiting for source")
	}
}
