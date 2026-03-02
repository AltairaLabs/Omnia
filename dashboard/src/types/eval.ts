/**
 * Eval result types for session evaluation data.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

export interface EvalResult {
  id: string;
  sessionId: string;
  messageId?: string;
  agentName: string;
  namespace: string;
  promptpackName: string;
  promptpackVersion?: string;
  evalId: string;
  evalType: string;
  trigger: string;
  passed: boolean;
  score?: number;
  details?: Record<string, unknown>;
  durationMs?: number;
  judgeTokens?: number;
  judgeCostUsd?: number;
  source: string;
  createdAt: string;
}

/** Prometheus metric type used for type-aware rendering. */
export type EvalMetricType = "gauge" | "counter" | "histogram" | "boolean";

export interface EvalResultSummary {
  evalId: string;
  evalType: string;
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  avgScore?: number;
  avgDurationMs?: number;
  metricType?: EvalMetricType;
}
