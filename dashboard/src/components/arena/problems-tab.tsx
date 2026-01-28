"use client";

import { useEffect } from "react";
import { cn } from "@/lib/utils";
import { useResultsPanelStore } from "@/stores/results-panel-store";
import { AlertCircle, AlertTriangle, Info, FileCode } from "lucide-react";

// =============================================================================
// Types
// =============================================================================

export interface Problem {
  severity: "error" | "warning" | "info";
  message: string;
  file: string;
  line?: number;
  column?: number;
  source?: string;
}

interface ProblemsTabProps {
  problems: Problem[];
  onProblemClick?: (problem: Problem) => void;
  className?: string;
}

// =============================================================================
// Component
// =============================================================================

/**
 * Problems tab showing validation errors and warnings.
 */
export function ProblemsTab({
  problems,
  onProblemClick,
  className,
}: ProblemsTabProps) {
  const setProblemsCount = useResultsPanelStore(
    (state) => state.setProblemsCount
  );

  // Update problems count in store
  useEffect(() => {
    const errorCount = problems.filter((p) => p.severity === "error").length;
    setProblemsCount(errorCount);
  }, [problems, setProblemsCount]);

  if (problems.length === 0) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center h-full text-muted-foreground",
          className
        )}
      >
        <AlertCircle className="h-8 w-8 mb-2 opacity-50" />
        <p className="text-sm">No problems detected</p>
      </div>
    );
  }

  // Group problems by file
  const groupedProblems = problems.reduce<Record<string, Problem[]>>(
    (acc, problem) => {
      const file = problem.file || "Unknown";
      if (!acc[file]) {
        acc[file] = [];
      }
      acc[file].push(problem);
      return acc;
    },
    {}
  );

  return (
    <div className={cn("overflow-auto h-full", className)}>
      {Object.entries(groupedProblems).map(([file, fileProblems]) => (
        <div key={file} className="mb-2">
          <div className="flex items-center gap-2 px-3 py-1 bg-muted/30 sticky top-0">
            <FileCode className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm font-medium truncate">{file}</span>
            <span className="text-xs text-muted-foreground">
              ({fileProblems.length})
            </span>
          </div>
          <div className="divide-y">
            {fileProblems.map((problem) => (
              <ProblemRow
                key={`${file}-${problem.line ?? 0}-${problem.column ?? 0}-${problem.message.slice(0, 20)}`}
                problem={problem}
                onClick={() => onProblemClick?.(problem)}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

// =============================================================================
// Problem Row
// =============================================================================

interface ProblemRowProps {
  problem: Problem;
  onClick?: () => void;
}

function ProblemRow({ problem, onClick }: ProblemRowProps) {
  const SeverityIcon = {
    error: AlertCircle,
    warning: AlertTriangle,
    info: Info,
  }[problem.severity];

  const severityColor = {
    error: "text-destructive",
    warning: "text-amber-500",
    info: "text-blue-500",
  }[problem.severity];

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "w-full flex items-start gap-2 px-3 py-2 text-left",
        "hover:bg-muted/50 transition-colors",
        "focus:outline-none focus:bg-muted/50"
      )}
    >
      <SeverityIcon className={cn("h-4 w-4 mt-0.5 shrink-0", severityColor)} />
      <div className="flex-1 min-w-0">
        <p className="text-sm">{problem.message}</p>
        <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
          {problem.line && (
            <span>
              Ln {problem.line}
              {problem.column && `, Col ${problem.column}`}
            </span>
          )}
          {problem.source && <span>({problem.source})</span>}
        </div>
      </div>
    </button>
  );
}

// =============================================================================
// Summary Component
// =============================================================================

interface ProblemsSummaryProps {
  problems: Problem[];
  className?: string;
}

export function ProblemsSummary({ problems, className }: ProblemsSummaryProps) {
  const errorCount = problems.filter((p) => p.severity === "error").length;
  const warningCount = problems.filter((p) => p.severity === "warning").length;
  const infoCount = problems.filter((p) => p.severity === "info").length;

  return (
    <div className={cn("flex items-center gap-4 text-sm", className)}>
      {errorCount > 0 && (
        <span className="flex items-center gap-1 text-destructive">
          <AlertCircle className="h-4 w-4" />
          {errorCount} error{errorCount !== 1 && "s"}
        </span>
      )}
      {warningCount > 0 && (
        <span className="flex items-center gap-1 text-amber-500">
          <AlertTriangle className="h-4 w-4" />
          {warningCount} warning{warningCount !== 1 && "s"}
        </span>
      )}
      {infoCount > 0 && (
        <span className="flex items-center gap-1 text-blue-500">
          <Info className="h-4 w-4" />
          {infoCount} info
        </span>
      )}
      {problems.length === 0 && (
        <span className="text-muted-foreground">No problems</span>
      )}
    </div>
  );
}
