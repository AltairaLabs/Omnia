/**
 * Eval results badge component for inline display with session messages.
 *
 * Shows a compact pass/fail badge next to evaluated messages, with an
 * expandable details section for score, eval type, and metadata.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";
import { CheckCircle2, XCircle, ChevronDown, ChevronRight } from "lucide-react";
import type { EvalResult } from "@/types/eval";
import { cn } from "@/lib/utils";

/**
 * Get a human-readable label for an eval type.
 */
function evalTypeLabel(evalType: string): string {
  const labels: Record<string, string> = {
    rule: "Rule",
    llm_judge: "LLM Judge",
    similarity: "Similarity",
    regex: "Regex",
    custom: "Custom",
  };
  return labels[evalType] || evalType;
}

/**
 * Get a human-readable label for an eval source.
 */
function evalSourceLabel(source: string): string {
  const labels: Record<string, string> = {
    in_proc: "In-process",
    worker: "Worker",
    manual: "Manual",
  };
  return labels[source] || source;
}

/**
 * Format a score value as a percentage string.
 */
function formatScore(score: number): string {
  return `${(score * 100).toFixed(0)}%`;
}

/**
 * Single eval result detail row.
 */
function EvalDetailRow({ result }: Readonly<{ result: EvalResult }>) {
  const [expanded, setExpanded] = useState(false);
  const hasDetails = result.details && Object.keys(result.details).length > 0;

  return (
    <div className="border rounded p-2 text-xs space-y-1">
      <button
        className="flex items-center justify-between w-full text-left"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-2">
          {result.passed ? (
            <CheckCircle2 className="h-3 w-3 text-green-500" />
          ) : (
            <XCircle className="h-3 w-3 text-red-500" />
          )}
          <span className="font-medium">{result.evalId}</span>
          <Badge variant="outline" className="text-[10px] px-1 py-0">
            {evalTypeLabel(result.evalType)}
          </Badge>
          {result.score !== undefined && (
            <span className="text-muted-foreground">
              Score: {formatScore(result.score)}
            </span>
          )}
        </div>
        {hasDetails && (
          <span>
            {expanded ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
          </span>
        )}
      </button>

      {expanded && hasDetails && (
        <div className="mt-1 space-y-1 pl-5">
          <div className="flex items-center gap-4 text-muted-foreground">
            <span>Source: {evalSourceLabel(result.source)}</span>
            {result.durationMs !== undefined && (
              <span>{result.durationMs}ms</span>
            )}
            {result.judgeCostUsd !== undefined && (
              <span>${result.judgeCostUsd.toFixed(4)}</span>
            )}
          </div>
          <pre className="bg-background p-1.5 rounded text-[10px] overflow-x-auto">
            {JSON.stringify(result.details, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}

/**
 * Compact badge showing aggregated pass/fail status for a message's eval results.
 * Click to expand and see individual eval details.
 */
export function EvalResultsBadge({ results }: Readonly<{ results: EvalResult[] }>) {
  const [expanded, setExpanded] = useState(false);

  if (results.length === 0) {
    return null;
  }

  const passedCount = results.filter((r) => r.passed).length;
  const failedCount = results.length - passedCount;
  const allPassed = failedCount === 0;

  const evalPlural = results.length === 1 ? "" : "s";
  const passedPlural = passedCount === 1 ? "" : "s";
  const summaryText = allPassed
    ? `${passedCount} eval${passedPlural} passed`
    : `${failedCount} of ${results.length} eval${evalPlural} failed`;

  return (
    <div className="mt-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            className="inline-flex items-center gap-1"
            onClick={() => setExpanded(!expanded)}
            data-testid="eval-results-badge"
          >
            <Badge
              variant={allPassed ? "secondary" : "destructive"}
              className={cn(
                "gap-1 text-xs cursor-pointer",
                allPassed && "bg-green-100 text-green-800 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400"
              )}
            >
              {allPassed ? (
                <CheckCircle2 className="h-3 w-3" />
              ) : (
                <XCircle className="h-3 w-3" />
              )}
              {summaryText}
            </Badge>
          </button>
        </TooltipTrigger>
        <TooltipContent>Click to {expanded ? "collapse" : "expand"} eval details</TooltipContent>
      </Tooltip>

      {expanded && (
        <div className="mt-2 space-y-1" data-testid="eval-results-details">
          {results.map((result) => (
            <EvalDetailRow key={result.id} result={result} />
          ))}
        </div>
      )}
    </div>
  );
}
