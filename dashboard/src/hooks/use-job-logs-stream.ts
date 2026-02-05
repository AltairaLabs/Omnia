"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { useWorkspace } from "@/contexts/workspace-context";

// =============================================================================
// Types
// =============================================================================

export interface LogEntry {
  timestamp: string;
  message: string;
  container?: string;
  level?: "info" | "warn" | "error" | "debug";
}

export interface UseJobLogsStreamOptions {
  /** Poll interval in milliseconds (default: 2000) */
  pollInterval?: number;
  /** Maximum number of log lines to keep (default: 1000) */
  maxLines?: number;
  /** Number of tail lines to fetch per poll (default: 100) */
  tailLines?: number;
  /** Auto-scroll to bottom on new logs (default: true) */
  autoScroll?: boolean;
  /** Only fetch logs since this many seconds ago (default: 3600) */
  sinceSeconds?: number;
}

export interface UseJobLogsStreamResult {
  logs: LogEntry[];
  loading: boolean;
  error: Error | null;
  streaming: boolean;
  /** Start streaming logs */
  start: () => void;
  /** Stop streaming logs */
  stop: () => void;
  /** Clear logs */
  clear: () => void;
  /** Refresh logs now */
  refresh: () => void;
}

// =============================================================================
// Hook
// =============================================================================

/**
 * Hook for streaming job logs with polling.
 */
export function useJobLogsStream(
  jobName: string | null | undefined,
  options: UseJobLogsStreamOptions = {}
): UseJobLogsStreamResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const {
    pollInterval = 2000,
    maxLines = 1000,
    tailLines = 100,
    sinceSeconds = 3600,
  } = options;

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [streaming, setStreaming] = useState(false);

  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const seenTimestampsRef = useRef<Set<string>>(new Set());

  const fetchLogs = useCallback(async () => {
    if (!workspace || !jobName) return;

    setLoading(true);

    try {
      const params = new URLSearchParams({
        tailLines: String(tailLines),
        sinceSeconds: String(sinceSeconds),
      });

      const response = await fetch(
        `/api/workspaces/${workspace}/arena/jobs/${jobName}/logs?${params}`
      );

      if (!response.ok) {
        throw new Error(`Failed to fetch logs: ${response.statusText}`);
      }

      const data = await response.json();
      const newLogs: LogEntry[] = data.logs || [];

      setLogs((prevLogs) => {
        // Deduplicate by timestamp+message
        const combined = [...prevLogs];

        for (const log of newLogs) {
          const key = `${log.timestamp}:${log.message}`;
          if (!seenTimestampsRef.current.has(key)) {
            seenTimestampsRef.current.add(key);
            combined.push(log);
          }
        }

        // Sort by timestamp
        combined.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

        // Trim to max lines
        if (combined.length > maxLines) {
          const removed = combined.splice(0, combined.length - maxLines);
          // Clean up seen timestamps for removed logs
          for (const log of removed) {
            const key = `${log.timestamp}:${log.message}`;
            seenTimestampsRef.current.delete(key);
          }
        }

        return combined;
      });

      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [workspace, jobName, tailLines, sinceSeconds, maxLines]);

  // Use ref to track streaming state for the start function
  const streamingRef = useRef(false);

  const stop = useCallback(() => {
    streamingRef.current = false;
    setStreaming(false);
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const clear = useCallback(() => {
    setLogs([]);
    seenTimestampsRef.current.clear();
  }, []);

  const refresh = useCallback(() => {
    fetchLogs();
  }, [fetchLogs]);

  // Start function uses ref to avoid dependency on streaming state
  const start = useCallback(() => {
    if (streamingRef.current) return; // Already streaming

    streamingRef.current = true;
    setStreaming(true);

    // Fetch immediately
    fetchLogs();
    // Set up polling
    intervalRef.current = setInterval(fetchLogs, pollInterval);
  }, [fetchLogs, pollInterval]);

  // Auto-start when job name changes
  useEffect(() => {
    if (jobName && workspace) {
      // Clear previous logs
      setLogs([]);
      seenTimestampsRef.current.clear();
      // Start streaming
      streamingRef.current = true;
      setStreaming(true);
      fetchLogs();
      intervalRef.current = setInterval(fetchLogs, pollInterval);
    }

    return () => {
      streamingRef.current = false;
      setStreaming(false);
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [jobName, workspace, fetchLogs, pollInterval]);

  // Clean up on unmount
  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);

  return {
    logs,
    loading,
    error,
    streaming,
    start,
    stop,
    clear,
    refresh,
  };
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Parse log level from message content.
 */
export function parseLogLevel(message: string): LogEntry["level"] {
  const lowerMessage = message.toLowerCase();
  if (lowerMessage.includes("error") || lowerMessage.includes("fatal")) {
    return "error";
  }
  if (lowerMessage.includes("warn")) {
    return "warn";
  }
  if (lowerMessage.includes("debug") || lowerMessage.includes("trace")) {
    return "debug";
  }
  return "info";
}

/**
 * Format timestamp for display.
 */
export function formatLogTimestamp(timestamp: string): string {
  try {
    const date = new Date(timestamp);
    // Check if the date is valid
    if (Number.isNaN(date.getTime())) {
      return timestamp;
    }
    return date.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    });
  } catch {
    return timestamp;
  }
}
