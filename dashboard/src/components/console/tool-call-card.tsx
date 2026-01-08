"use client";

import { useState, useRef, useLayoutEffect } from "react";
import { ChevronDown, ChevronRight, Wrench, Check, X, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ToolCallWithResult } from "@/types/websocket";

interface ToolCallCardProps {
  toolCall: ToolCallWithResult;
  className?: string;
}

export function ToolCallCard({ toolCall, className }: Readonly<ToolCallCardProps>) {
  // Start expanded if already has result, otherwise collapsed
  const [isExpanded, setIsExpanded] = useState(toolCall.status !== "pending");
  const prevStatusRef = useRef(toolCall.status);

  // Auto-expand when transitioning from pending to success/error
  // Using useLayoutEffect to avoid visual flicker
  useLayoutEffect(() => {
    if (prevStatusRef.current === "pending" && toolCall.status !== "pending") {
      // Use requestAnimationFrame to defer the state update
      requestAnimationFrame(() => {
        setIsExpanded(true);
      });
    }
    prevStatusRef.current = toolCall.status;
  }, [toolCall.status]);

  const statusIcon = {
    pending: <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />,
    success: <Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />,
    error: <X className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />,
  }[toolCall.status];

  const statusColor = {
    pending: "border-muted-foreground/30 bg-muted/30",
    success: "border-green-500/30 bg-green-500/10",
    error: "border-red-500/30 bg-red-500/10",
  }[toolCall.status];

  return (
    <div
      className={cn(
        "rounded-md border text-sm",
        statusColor,
        className
      )}
    >
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted/50 transition-colors"
      >
        {isExpanded ? (
          <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
        )}
        <Wrench className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="font-medium text-foreground">{toolCall.name}</span>
        <span className="ml-auto">{statusIcon}</span>
      </button>

      {isExpanded && (
        <div className="border-t px-3 py-2 space-y-2 text-xs">
          {toolCall.arguments && Object.keys(toolCall.arguments).length > 0 && (
            <div>
              <p className="text-muted-foreground mb-1">Arguments:</p>
              <pre className="bg-muted/50 rounded p-2 overflow-x-auto">
                {JSON.stringify(toolCall.arguments, null, 2)}
              </pre>
            </div>
          )}

          {toolCall.status === "success" && toolCall.result !== undefined && (
            <div>
              <p className="text-muted-foreground mb-1">Result:</p>
              <pre className="bg-muted/50 rounded p-2 overflow-x-auto">
                {typeof toolCall.result === "string"
                  ? toolCall.result
                  : JSON.stringify(toolCall.result, null, 2)}
              </pre>
            </div>
          )}

          {toolCall.status === "error" && toolCall.error && (
            <div>
              <p className="text-red-600 dark:text-red-400 mb-1">Error:</p>
              <pre className="bg-red-500/10 rounded p-2 overflow-x-auto text-red-700 dark:text-red-300">
                {toolCall.error}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
