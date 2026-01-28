"use client";

import { useCallback, useState } from "react";
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
  problems?: Problem[];
  /** Callback when clicking on a problem */
  onProblemClick?: (problem: Problem) => void;
  /** Custom content for the console tab */
  consoleContent?: React.ReactNode;
  className?: string;
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
  const height = useResultsPanelStore((state) => state.height);
  const close = useResultsPanelStore((state) => state.close);
  const toggle = useResultsPanelStore((state) => state.toggle);
  const setHeight = useResultsPanelStore((state) => state.setHeight);

  // Resize handling
  const [isResizing, setIsResizing] = useState(false);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);

      const startY = e.clientY;
      const startHeight = height;

      const handleMouseMove = (moveEvent: MouseEvent) => {
        const container = (e.target as HTMLElement).closest(
          "[data-results-panel-container]"
        );
        if (!container) return;

        const containerRect = container.getBoundingClientRect();
        const deltaY = startY - moveEvent.clientY;
        const deltaPercent = (deltaY / containerRect.height) * 100;
        const newHeight = startHeight + deltaPercent;

        setHeight(newHeight);
      };

      const handleMouseUp = () => {
        setIsResizing(false);
        document.removeEventListener("mousemove", handleMouseMove);
        document.removeEventListener("mouseup", handleMouseUp);
      };

      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);
    },
    [height, setHeight]
  );

  return (
    <div
      data-results-panel-container
      className={cn("flex flex-col", className)}
      style={{ height: isOpen ? `${height}%` : "auto" }}
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
          {/* Resize handle */}
          <div
            role="separator"
            aria-orientation="horizontal"
            aria-label="Resize results panel"
            tabIndex={0}
            onMouseDown={handleMouseDown}
            onKeyDown={(e) => {
              if (e.key === "ArrowUp") {
                e.preventDefault();
                setHeight(height + 5);
              } else if (e.key === "ArrowDown") {
                e.preventDefault();
                setHeight(height - 5);
              }
            }}
            className={cn(
              "h-1 border-t cursor-ns-resize hover:bg-primary/20 transition-colors focus:outline-none focus:bg-primary/30",
              isResizing && "bg-primary/30"
            )}
          />

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
  activeTab: ResultsPanelTab;
  problems: Problem[];
  onProblemClick?: (problem: Problem) => void;
  consoleContent?: React.ReactNode;
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
