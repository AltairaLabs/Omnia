"use client";

import { useState, useEffect, useCallback } from "react";
import { cn } from "@/lib/utils";
import { useResultsPanelStore } from "@/stores/results-panel-store";
import { useWorkspace } from "@/contexts/workspace-context";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  BarChart3,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Clock,
  RefreshCw,
  Loader2,
  ExternalLink,
} from "lucide-react";
import type { EvaluationResult } from "@/types/arena";

interface JobResultsTabProps {
  readonly className?: string;
}

interface JobResultsData {
  type: "evaluation" | "loadtest" | "datagen";
  summary?: {
    total?: number;
    passed?: number;
    failed?: number;
    errors?: number;
    skipped?: number;
    passRate?: number;
    avgDurationMs?: number;
  };
  results?: EvaluationResult[];
  resultUrl?: string;
}

/**
 * Job results tab showing evaluation results.
 */
export function JobResultsTab({ className }: JobResultsTabProps) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const currentJobName = useResultsPanelStore((state) => state.currentJobName);

  const [results, setResults] = useState<JobResultsData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchResults = useCallback(async () => {
    if (!workspace || !currentJobName) return;

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/jobs/${currentJobName}/results`
      );

      if (!response.ok) {
        if (response.status === 404) {
          setResults(null);
          return;
        }
        throw new Error(`Failed to fetch results: ${response.statusText}`);
      }

      const data = await response.json();
      setResults(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [workspace, currentJobName]);

  useEffect(() => {
    if (currentJobName) {
      fetchResults();
    }
  }, [currentJobName, fetchResults]);

  if (!currentJobName) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center h-full text-muted-foreground",
          className
        )}
      >
        <BarChart3 className="h-8 w-8 mb-2 opacity-50" />
        <p className="text-sm">No job selected</p>
        <p className="text-xs mt-1">Run a job to see results here</p>
      </div>
    );
  }

  if (loading) {
    return (
      <div
        className={cn(
          "flex items-center justify-center h-full",
          className
        )}
      >
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center h-full text-muted-foreground",
          className
        )}
      >
        <AlertTriangle className="h-8 w-8 mb-2 text-destructive" />
        <p className="text-sm text-destructive">{error.message}</p>
        <Button variant="ghost" size="sm" onClick={fetchResults} className="mt-2">
          <RefreshCw className="h-4 w-4 mr-2" />
          Retry
        </Button>
      </div>
    );
  }

  if (!results) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center h-full text-muted-foreground",
          className
        )}
      >
        <Clock className="h-8 w-8 mb-2 opacity-50" />
        <p className="text-sm">Results not available yet</p>
        <p className="text-xs mt-1">Job may still be running</p>
        <Button variant="ghost" size="sm" onClick={fetchResults} className="mt-2">
          <RefreshCw className="h-4 w-4 mr-2" />
          Refresh
        </Button>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col h-full", className)}>
      {/* Summary */}
      {results.summary && (
        <div className="flex items-center gap-4 px-4 py-3 border-b bg-muted/20">
          <ResultsSummary summary={results.summary} />
          {results.resultUrl && (
            <a
              href={results.resultUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-primary hover:underline flex items-center gap-1 ml-auto"
            >
              View Full Results
              <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      )}

      {/* Results table */}
      {results.results && results.results.length > 0 ? (
        <div className="flex-1 overflow-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/30 sticky top-0">
              <tr>
                <th className="px-4 py-2 text-left font-medium">Status</th>
                <th className="px-4 py-2 text-left font-medium">Scenario</th>
                <th className="px-4 py-2 text-left font-medium">Score</th>
                <th className="px-4 py-2 text-left font-medium">Duration</th>
                <th className="px-4 py-2 text-left font-medium">Details</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {results.results.map((result) => (
                <ResultRow key={`${result.scenario}-${result.status}-${result.score ?? 0}-${result.durationMs ?? 0}`} result={result} />
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center text-muted-foreground">
          <p className="text-sm">No detailed results available</p>
        </div>
      )}
    </div>
  );
}

// =============================================================================
// Results Summary
// =============================================================================

interface ResultsSummaryProps {
  readonly summary: NonNullable<JobResultsData["summary"]>;
}

function getPassRateColor(passRate: number): string {
  if (passRate >= 0.9) return "text-green-500";
  if (passRate >= 0.7) return "text-amber-500";
  return "text-destructive";
}

function ResultsSummary({ summary }: ResultsSummaryProps) {
  const passRate = summary.passRate ?? 0;
  const passRateColor = getPassRateColor(passRate);

  return (
    <div className="flex items-center gap-6 text-sm">
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">Pass Rate:</span>
        <span className={cn("font-medium", passRateColor)}>
          {(passRate * 100).toFixed(1)}%
        </span>
      </div>
      {summary.total !== undefined && (
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground">Total:</span>
          <span className="font-medium">{summary.total}</span>
        </div>
      )}
      {summary.passed !== undefined && (
        <div className="flex items-center gap-1">
          <CheckCircle2 className="h-4 w-4 text-green-500" />
          <span>{summary.passed}</span>
        </div>
      )}
      {summary.failed !== undefined && summary.failed > 0 && (
        <div className="flex items-center gap-1">
          <XCircle className="h-4 w-4 text-destructive" />
          <span>{summary.failed}</span>
        </div>
      )}
      {summary.avgDurationMs !== undefined && (
        <div className="flex items-center gap-2">
          <Clock className="h-4 w-4 text-muted-foreground" />
          <span>{formatDuration(summary.avgDurationMs)}</span>
        </div>
      )}
    </div>
  );
}

// =============================================================================
// Result Row
// =============================================================================

interface ResultRowProps {
  readonly result: EvaluationResult;
}

function ResultRow({ result }: ResultRowProps) {
  const StatusIcon = {
    pass: CheckCircle2,
    fail: XCircle,
    error: AlertTriangle,
    skipped: Clock,
  }[result.status];

  const statusColor = {
    pass: "text-green-500",
    fail: "text-destructive",
    error: "text-amber-500",
    skipped: "text-muted-foreground",
  }[result.status];

  return (
    <tr className="hover:bg-muted/30">
      <td className="px-4 py-2">
        <Badge variant="outline" className={cn("gap-1", statusColor)}>
          <StatusIcon className="h-3 w-3" />
          {result.status}
        </Badge>
      </td>
      <td className="px-4 py-2 font-medium">{result.scenario}</td>
      <td className="px-4 py-2">
        {result.score === undefined ? (
          <span className="text-muted-foreground">—</span>
        ) : (
          <span className={cn(result.score >= 0.7 ? "text-green-500" : "text-amber-500")}>
            {(result.score * 100).toFixed(0)}%
          </span>
        )}
      </td>
      <td className="px-4 py-2 text-muted-foreground">
        {result.durationMs ? formatDuration(result.durationMs) : "—"}
      </td>
      <td className="px-4 py-2">
        {result.error && (
          <span className="text-destructive text-xs">{result.error}</span>
        )}
        {!result.error && result.assertions && result.assertions.length > 0 && (
          <AssertionBadges assertions={result.assertions} />
        )}
        {!result.error && (!result.assertions || result.assertions.length === 0) && (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
    </tr>
  );
}

// =============================================================================
// Assertion Badges
// =============================================================================

interface AssertionBadgesProps {
  readonly assertions: NonNullable<EvaluationResult["assertions"]>;
}

function AssertionBadges({ assertions }: AssertionBadgesProps) {
  const passed = assertions.filter((a) => a.passed).length;
  const total = assertions.length;

  return (
    <div className="flex items-center gap-1">
      <span className="text-xs text-muted-foreground">
        {passed}/{total} assertions
      </span>
    </div>
  );
}

// =============================================================================
// Utilities
// =============================================================================

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}
