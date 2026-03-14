"use client";

import { useState, useRef, useLayoutEffect } from "react";
import { ChevronDown, ChevronRight, Wrench, Check, X, Loader2, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { JsonBlock } from "@/components/ui/json-block";
import { cn } from "@/lib/utils";
import type { ToolCallWithResult } from "@/types/websocket";

interface ToolCallCardProps {
  toolCall: ToolCallWithResult;
  className?: string;
  onApprove?: (callId: string) => void;
  onAlwaysApprove?: (callId: string) => void;
  onReject?: (callId: string, reason: string) => void;
}

export function ToolCallCard({ toolCall, className, onApprove, onAlwaysApprove, onReject }: Readonly<ToolCallCardProps>) {
  // Expanded only when consent is needed
  const [isExpanded, setIsExpanded] = useState(toolCall.status === "awaiting_consent");
  const prevStatusRef = useRef(toolCall.status);

  // Auto-expand when transitioning to awaiting_consent
  // Auto-collapse when transitioning away from it
  useLayoutEffect(() => {
    if (toolCall.status === "awaiting_consent" && prevStatusRef.current !== "awaiting_consent") {
      requestAnimationFrame(() => {
        setIsExpanded(true);
      });
    }
    prevStatusRef.current = toolCall.status;
  }, [toolCall.status]);

  const statusIcon = {
    pending: <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />,
    awaiting_consent: <AlertTriangle className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400" />,
    success: <Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />,
    error: <X className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />,
  }[toolCall.status];

  const statusColor = {
    pending: "border-muted-foreground/30 bg-muted/30",
    awaiting_consent: "border-amber-500/30 bg-amber-500/10",
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
              <JsonBlock data={toolCall.arguments} />
            </div>
          )}

          {toolCall.status === "awaiting_consent" && (
            <div>
              {toolCall.consent_message && (
                <p className="text-amber-700 dark:text-amber-300 mb-2">
                  {toolCall.consent_message}
                </p>
              )}
              <div className="flex gap-2">
                <Button
                  size="sm"
                  onClick={() => onAlwaysApprove?.(toolCall.id)}
                >
                  Always Allow
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => onApprove?.(toolCall.id)}
                >
                  Allow
                </Button>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => onReject?.(toolCall.id, "User denied")}
                >
                  Deny
                </Button>
              </div>
            </div>
          )}

          {toolCall.status === "success" && toolCall.result !== undefined && (
            <div>
              <p className="text-muted-foreground mb-1">Result:</p>
              {typeof toolCall.result === "string" ? (
                <pre className="bg-muted/50 rounded p-2 overflow-x-auto">
                  {toolCall.result}
                </pre>
              ) : (
                <JsonBlock data={toolCall.result} />
              )}
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
