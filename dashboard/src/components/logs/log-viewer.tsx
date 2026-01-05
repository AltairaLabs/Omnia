"use client";

import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { Play, Pause, Trash2, Download, Search, X, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useLogs } from "@/hooks";
import { isDemoMode } from "@/lib/api/client";

export interface LogEntry {
  timestamp: Date;
  level: "info" | "warn" | "error" | "debug";
  message: string;
  container?: string;
}

interface LogViewerProps {
  agentName: string;
  namespace: string;
  containers?: string[];
  className?: string;
}

// Mock log messages for demo
const MOCK_LOG_TEMPLATES = [
  { level: "info", message: "Server started on port 8080" },
  { level: "info", message: "WebSocket connection established" },
  { level: "info", message: "Session created: sess_{id}" },
  { level: "debug", message: "Processing message from client" },
  { level: "info", message: "LLM request sent to provider" },
  { level: "debug", message: "Tokens used - input: {input}, output: {output}" },
  { level: "info", message: "Tool call: {tool}({args})" },
  { level: "info", message: "Tool response received in {ms}ms" },
  { level: "info", message: "Response streamed to client" },
  { level: "warn", message: "High latency detected: {ms}ms" },
  { level: "error", message: "Connection timeout after 30s" },
  { level: "info", message: "Session ended: sess_{id}" },
  { level: "debug", message: "Cleanup completed for session" },
  { level: "info", message: "Health check passed" },
  { level: "warn", message: "Memory usage at 75%" },
] as const;

const TOOL_NAMES = ["search_database", "get_user_info", "send_email", "fetch_data"];

function generateMockLog(container: string): LogEntry {
  const template = MOCK_LOG_TEMPLATES[Math.floor(Math.random() * MOCK_LOG_TEMPLATES.length)];
  const message = template.message
    .replace("{id}", Math.random().toString(36).slice(2, 10))
    .replace("{input}", String(Math.floor(Math.random() * 2000) + 500))
    .replace("{output}", String(Math.floor(Math.random() * 1000) + 100))
    .replace("{ms}", String(Math.floor(Math.random() * 2000) + 100))
    .replace("{tool}", TOOL_NAMES[Math.floor(Math.random() * TOOL_NAMES.length)])
    .replace("{args}", '{"query": "test"}');

  return {
    timestamp: new Date(),
    level: template.level,
    message,
    container,
  };
}

const levelColors = {
  info: "text-blue-600 dark:text-blue-400",
  warn: "text-yellow-600 dark:text-yellow-400",
  error: "text-red-600 dark:text-red-400",
  debug: "text-gray-500 dark:text-gray-400",
};

const levelBadgeColors = {
  info: "bg-blue-500/15 text-blue-700 dark:text-blue-400 border-blue-500/20",
  warn: "bg-yellow-500/15 text-yellow-700 dark:text-yellow-400 border-yellow-500/20",
  error: "bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/20",
  debug: "bg-gray-500/15 text-gray-700 dark:text-gray-400 border-gray-500/20",
};

// Helper to generate initial logs
function generateInitialLogs(containers: string[]): LogEntry[] {
  const initialLogs: LogEntry[] = [];
  for (let i = 0; i < 20; i++) {
    const container = containers[Math.floor(Math.random() * containers.length)];
    const log = generateMockLog(container);
    log.timestamp = new Date(Date.now() - (20 - i) * 1000);
    initialLogs.push(log);
  }
  return initialLogs;
}

export function LogViewer({
  agentName,
  namespace,
  containers = ["facade", "runtime"],
  className,
}: LogViewerProps) {
  // Fetch real logs when not in demo mode
  const { data: apiLogs, isLoading, refetch } = useLogs(namespace, agentName, {
    tailLines: 500,
    sinceSeconds: 3600,
    refetchInterval: 5000,
  });

  // Convert API logs to LogEntry format with Date objects
  const realLogs = useMemo(() => {
    if (!apiLogs || apiLogs.length === 0) return [];
    return apiLogs.map((log) => ({
      timestamp: log.timestamp ? new Date(log.timestamp) : new Date(),
      level: (log.level || "info") as LogEntry["level"],
      message: log.message || "",
      container: log.container,
    }));
  }, [apiLogs]);

  // Use lazy initial state to generate logs once on mount (only for demo mode)
  const [mockLogs, setMockLogs] = useState<LogEntry[]>(() =>
    isDemoMode ? generateInitialLogs(containers) : []
  );
  const [isStreaming, setIsStreaming] = useState(true);
  const [filter, setFilter] = useState("");
  const [selectedContainer, setSelectedContainer] = useState<string | "all">("all");
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(
    new Set(["info", "warn", "error", "debug"])
  );
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Use real logs if available, otherwise use mock logs
  const logs = realLogs.length > 0 ? realLogs : mockLogs;

  // Start/stop mock log streaming (only in demo mode)
  useEffect(() => {
    if (!isDemoMode) return;

    if (isStreaming) {
      // Stream new logs periodically
      intervalRef.current = setInterval(() => {
        const container = containers[Math.floor(Math.random() * containers.length)];
        setMockLogs((prev) => [...prev.slice(-500), generateMockLog(container)]);
      }, Math.random() * 2000 + 500);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [isStreaming, containers]);

  // Auto-scroll to bottom
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // Filter logs
  const filteredLogs = logs.filter((log) => {
    if (selectedContainer !== "all" && log.container !== selectedContainer) {
      return false;
    }
    if (!selectedLevels.has(log.level)) {
      return false;
    }
    if (filter && !log.message.toLowerCase().includes(filter.toLowerCase())) {
      return false;
    }
    return true;
  });

  const toggleLevel = useCallback((level: string) => {
    setSelectedLevels((prev) => {
      const next = new Set(prev);
      if (next.has(level)) {
        next.delete(level);
      } else {
        next.add(level);
      }
      return next;
    });
  }, []);

  const clearLogs = useCallback(() => {
    if (isDemoMode) {
      setMockLogs([]);
    }
    // For real mode, refetch to get fresh logs
    refetch();
  }, [refetch]);

  const downloadLogs = useCallback(() => {
    const content = filteredLogs
      .map(
        (log) =>
          `${log.timestamp.toISOString()} [${log.level.toUpperCase()}] [${log.container}] ${log.message}`
      )
      .join("\n");
    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${agentName}-${namespace}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }, [filteredLogs, agentName, namespace]);

  const formatTimestamp = (date: Date) => {
    return date.toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    });
  };

  return (
    <div className={cn("flex flex-col h-[600px] border rounded-lg", className)}>
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 px-4 py-3 border-b bg-muted/30">
        {/* Stream control - only show in demo mode */}
        {isDemoMode ? (
          <Button
            variant={isStreaming ? "default" : "outline"}
            size="sm"
            onClick={() => setIsStreaming(!isStreaming)}
          >
            {isStreaming ? (
              <>
                <Pause className="h-4 w-4 mr-1" />
                Pause
              </>
            ) : (
              <>
                <Play className="h-4 w-4 mr-1" />
                Resume
              </>
            )}
          </Button>
        ) : (
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isLoading}
          >
            <RefreshCw className={cn("h-4 w-4 mr-1", isLoading && "animate-spin")} />
            Refresh
          </Button>
        )}

        {/* Container selector */}
        <div className="flex items-center gap-1">
          <Badge
            variant="outline"
            className={cn(
              "cursor-pointer",
              selectedContainer === "all" && "bg-primary text-primary-foreground"
            )}
            onClick={() => setSelectedContainer("all")}
          >
            All
          </Badge>
          {containers.map((container) => (
            <Badge
              key={container}
              variant="outline"
              className={cn(
                "cursor-pointer",
                selectedContainer === container && "bg-primary text-primary-foreground"
              )}
              onClick={() => setSelectedContainer(container)}
            >
              {container}
            </Badge>
          ))}
        </div>

        {/* Level filters */}
        <div className="flex items-center gap-1 ml-2">
          {(["error", "warn", "info", "debug"] as const).map((level) => (
            <Badge
              key={level}
              variant="outline"
              className={cn(
                "cursor-pointer uppercase text-xs",
                selectedLevels.has(level) ? levelBadgeColors[level] : "opacity-40"
              )}
              onClick={() => toggleLevel(level)}
            >
              {level}
            </Badge>
          ))}
        </div>

        {/* Search */}
        <div className="relative flex-1 min-w-[200px] ml-2">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter logs..."
            className="pl-8 h-8"
          />
          {filter && (
            <button
              onClick={() => setFilter("")}
              className="absolute right-2 top-1/2 -translate-y-1/2"
            >
              <X className="h-4 w-4 text-muted-foreground hover:text-foreground" />
            </button>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={downloadLogs} title="Download logs">
            <Download className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="sm" onClick={clearLogs} title="Clear logs">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Auto-scroll indicator */}
      {!autoScroll && (
        <button
          onClick={() => setAutoScroll(true)}
          className="px-4 py-1 text-xs text-center bg-muted hover:bg-muted/80 border-b"
        >
          Auto-scroll paused. Click to resume.
        </button>
      )}

      {/* Log entries */}
      <ScrollArea
        className="flex-1 font-mono text-xs"
        ref={scrollRef}
        onMouseEnter={() => setAutoScroll(false)}
        onMouseLeave={() => setAutoScroll(true)}
      >
        <div className="p-2">
          {filteredLogs.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-muted-foreground">
              {logs.length === 0 ? "No logs yet..." : "No logs match the current filters"}
            </div>
          ) : (
            filteredLogs.map((log, index) => (
              <div
                key={index}
                className="flex gap-2 py-0.5 hover:bg-muted/50 rounded px-1"
              >
                <span className="text-muted-foreground shrink-0">
                  {formatTimestamp(log.timestamp)}
                </span>
                <span
                  className={cn(
                    "shrink-0 w-12 uppercase font-medium",
                    levelColors[log.level]
                  )}
                >
                  {log.level}
                </span>
                <span className="text-muted-foreground shrink-0 w-16">
                  [{log.container}]
                </span>
                <span className="break-all">{log.message}</span>
              </div>
            ))
          )}
        </div>
      </ScrollArea>

      {/* Status bar */}
      <div className="flex items-center justify-between px-4 py-2 border-t bg-muted/30 text-xs text-muted-foreground">
        <span>
          {filteredLogs.length} / {logs.length} entries
        </span>
        <span>
          {isDemoMode ? (
            isStreaming ? "Demo mode • Streaming..." : "Demo mode • Paused"
          ) : (
            isLoading ? "Loading..." : "Live logs • Auto-refresh 5s"
          )} • {agentName}.{namespace}
        </span>
      </div>
    </div>
  );
}
