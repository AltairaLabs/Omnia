"use client";

import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { Download, Search, X, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useLogs, useDemoMode } from "@/hooks";

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
  defaultTailLines?: number;
}

const TAIL_LINE_OPTIONS = [50, 100, 200, 500, 1000];

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

export function LogViewer({
  agentName,
  namespace,
  containers = ["facade", "runtime"],
  className,
  defaultTailLines = 100,
}: LogViewerProps) {
  const { isDemoMode } = useDemoMode();
  const [tailLines, setTailLines] = useState(defaultTailLines);

  // Fetch logs via DataService (works in both demo and live modes)
  const { data: apiLogs, isLoading, refetch } = useLogs(namespace, agentName, {
    tailLines,
    sinceSeconds: 3600,
  });

  // Convert API logs to LogEntry format with Date objects
  const logs = useMemo(() => {
    if (!apiLogs || apiLogs.length === 0) return [];
    return apiLogs.map((log) => ({
      timestamp: log.timestamp ? new Date(log.timestamp) : new Date(),
      level: (log.level || "info") as LogEntry["level"],
      message: log.message || "",
      container: log.container,
    }));
  }, [apiLogs]);

  const [filter, setFilter] = useState("");
  const [selectedContainer, setSelectedContainer] = useState<string | "all">("all");
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(
    new Set(["info", "warn", "error", "debug"])
  );
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // Filter logs based on user selections
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
        {/* Refresh button */}
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isLoading}
        >
          <RefreshCw className={cn("h-4 w-4 mr-1", isLoading && "animate-spin")} />
          Refresh
        </Button>

        {/* Tail lines selector */}
        <Select
          value={String(tailLines)}
          onValueChange={(v) => setTailLines(Number(v))}
        >
          <SelectTrigger className="w-[100px] h-8">
            <SelectValue placeholder="Lines" />
          </SelectTrigger>
          <SelectContent>
            {TAIL_LINE_OPTIONS.map((n) => (
              <SelectItem key={n} value={String(n)}>
                {n} lines
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

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
          {isLoading && logs.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-muted-foreground">
              Loading logs...
            </div>
          ) : filteredLogs.length === 0 ? (
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
            "Demo data"
          ) : (
            isLoading ? "Loading..." : "Live logs"
          )} • {agentName}.{namespace}
        </span>
      </div>
    </div>
  );
}
