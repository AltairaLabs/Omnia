"use client";

import { cn } from "@/lib/utils";
import {
  useResultsPanelStore,
  type ResultsPanelTab,
} from "@/stores/results-panel-store";
import { ResultsPanelTabs } from "./results-panel-tabs";
import { ProblemsTab, type Problem } from "./problems-tab";
import { JobLogsTab } from "./job-logs-tab";
import { JobResultsTab } from "./job-results-tab";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronUp, X } from "lucide-react";

interface ResultsPanelProps {
  /** Validation problems to display */
  readonly problems?: Problem[];
  /** Callback when clicking on a problem */
  readonly onProblemClick?: (problem: Problem) => void;
  /** Custom content for the console tab */
  readonly consoleContent?: React.ReactNode;
  readonly className?: string;
}

/**
 * Bottom panel container with tabs for problems, logs, results, and console.
 */
export function ResultsPanel({
  problems = [],
  onProblemClick,
  consoleContent,
  className,
}: ResultsPanelProps) {
  const isOpen = useResultsPanelStore((state) => state.isOpen);
  const activeTab = useResultsPanelStore((state) => state.activeTab);
  const close = useResultsPanelStore((state) => state.close);
  const toggle = useResultsPanelStore((state) => state.toggle);

  return (
    <div
      data-results-panel-container
      className={cn("flex flex-col", isOpen && "h-full", className)}
    >
      {/* Collapsed header */}
      {!isOpen && (
        <button
          type="button"
          onClick={toggle}
          className="flex items-center justify-between px-4 py-1 border-t bg-muted/30 hover:bg-muted/50 transition-colors"
        >
          <div className="flex items-center gap-2 text-sm">
            <ChevronUp className="h-4 w-4" />
            <span>Problems, Logs, Results</span>
          </div>
        </button>
      )}

      {/* Expanded panel */}
      {isOpen && (
        <>
          {/* Header with tabs */}
          <div className="flex items-center justify-between border-b bg-muted/30">
            <ResultsPanelTabs />
            <div className="flex items-center gap-1 px-2">
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={toggle}
                title="Minimize panel"
              >
                <ChevronDown className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={close}
                title="Close panel"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>

          {/* Content */}
          <div className="flex-1 min-h-0 overflow-hidden">
            <TabContent
              activeTab={activeTab}
              problems={problems}
              onProblemClick={onProblemClick}
              consoleContent={consoleContent}
            />
          </div>
        </>
      )}
    </div>
  );
}

// =============================================================================
// Tab Content
// =============================================================================

interface TabContentProps {
  readonly activeTab: ResultsPanelTab;
  readonly problems: Problem[];
  readonly onProblemClick?: (problem: Problem) => void;
  readonly consoleContent?: React.ReactNode;
}

function TabContent({
  activeTab,
  problems,
  onProblemClick,
  consoleContent,
}: TabContentProps) {
  switch (activeTab) {
    case "problems":
      return (
        <ProblemsTab problems={problems} onProblemClick={onProblemClick} />
      );
    case "logs":
      return <JobLogsTab />;
    case "results":
      return <JobResultsTab />;
    case "console":
      return consoleContent || <ConsolePlaceholder />;
    default:
      return null;
  }
}

// =============================================================================
// Console Placeholder
// =============================================================================

function ConsolePlaceholder() {
  return (
    <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
      <p className="text-sm">Dev Console</p>
      <p className="text-xs mt-1">Interactive testing coming soon</p>
    </div>
  );
}

// =============================================================================
// Exports
// =============================================================================

export { type Problem } from "./problems-tab";
