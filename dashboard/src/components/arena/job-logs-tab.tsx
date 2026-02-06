"use client";

import { useRef, useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { useResultsPanelStore } from "@/stores/results-panel-store";
import {
  useJobLogsStream,
  formatLogTimestamp,
  parseLogLevel,
  type LogEntry,
} from "@/hooks/use-job-logs-stream";
import { Button } from "@/components/ui/button";
import {
  ScrollText,
  Play,
  Pause,
  Trash2,
  ArrowDown,
  Loader2,
} from "lucide-react";

interface JobLogsTabProps {
  readonly className?: string;
}

/**
 * Job logs tab with live streaming.
 */
export function JobLogsTab({ className }: JobLogsTabProps) {
  const currentJobName = useResultsPanelStore((state) => state.currentJobName);
  const { logs, loading, streaming, start, stop, clear } =
    useJobLogsStream(currentJobName);

  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // Handle scroll to detect if user scrolled up
  const handleScroll = () => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    setAutoScroll(isAtBottom);
  };

  const scrollToBottom = () => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      setAutoScroll(true);
    }
  };

  if (!currentJobName) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center h-full text-muted-foreground",
          className
        )}
      >
        <ScrollText className="h-8 w-8 mb-2 opacity-50" />
        <p className="text-sm">No job selected</p>
        <p className="text-xs mt-1">Run a job to see logs here</p>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col h-full", className)}>
      {/* Toolbar */}
      <div className="flex items-center justify-between px-2 py-1 border-b bg-muted/20">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            Job: <span className="font-medium text-foreground">{currentJobName}</span>
          </span>
          {loading && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={streaming ? stop : start}
            title={streaming ? "Pause streaming" : "Resume streaming"}
          >
            {streaming ? (
              <Pause className="h-4 w-4" />
            ) : (
              <Play className="h-4 w-4" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={clear}
            title="Clear logs"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
          {!autoScroll && (
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              onClick={scrollToBottom}
              title="Scroll to bottom"
            >
              <ArrowDown className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Log content */}
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="flex-1 overflow-auto font-mono text-xs bg-muted/10"
      >
        {logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-muted-foreground">
            <p>No logs yet</p>
          </div>
        ) : (
          <div className="p-2">
            {logs.map((log) => (
              <LogLine key={`${log.timestamp}-${log.container ?? "main"}-${log.message.slice(0, 30)}`} log={log} />
            ))}
          </div>
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between px-2 py-1 border-t text-xs text-muted-foreground">
        <span>{logs.length} lines</span>
        <span>
          {streaming ? (
            <span className="flex items-center gap-1">
              <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />{" "}
              Live
            </span>
          ) : (
            "Paused"
          )}
        </span>
      </div>
    </div>
  );
}

// =============================================================================
// Log Line Component
// =============================================================================

interface LogLineProps {
  readonly log: LogEntry;
}

function LogLine({ log }: LogLineProps) {
  const level = log.level || parseLogLevel(log.message) || "info";

  const levelColors: Record<string, string> = {
    error: "text-destructive",
    warn: "text-amber-500",
    info: "text-foreground",
    debug: "text-muted-foreground",
  };
  const levelColor = levelColors[level] || "text-foreground";

  return (
    <div className="flex gap-2 py-0.5 hover:bg-muted/50">
      <span className="text-muted-foreground shrink-0 select-none">
        {formatLogTimestamp(log.timestamp)}
      </span>
      {log.container && (
        <span className="text-blue-500 shrink-0">[{log.container}]</span>
      )}
      <span className={cn("break-all", levelColor)}>{log.message}</span>
    </div>
  );
}
