"use client";

import { ScrollArea } from "@/components/ui/scroll-area";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { ToolCallBadge } from "./tool-call-badge";
import { JsonBlock } from "@/components/ui/json-block";
import { Wrench } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ToolCall } from "@/types/session";

interface ToolCallsTabProps {
  readonly toolCalls: ToolCall[];
}

function RenderValue({ value }: Readonly<{ value: unknown }>) {
  return <JsonBlock data={value} className="bg-transparent rounded-none border-0 h-full" />;
}

function tryParseJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function ToolCallsTab({ toolCalls }: ToolCallsTabProps) {
  const selectedToolCallId = useDebugPanelStore((s) => s.selectedToolCallId);
  const selectToolCall = useDebugPanelStore((s) => s.selectToolCall);

  const selectedTc = toolCalls.find((tc) => tc.callId === selectedToolCallId || tc.id === selectedToolCallId);

  if (toolCalls.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground" data-testid="toolcalls-empty">
        No tool calls in this session
      </div>
    );
  }

  return (
    <div className="flex h-full" data-testid="toolcalls-tab">
      {/* Left list */}
      <ScrollArea className="w-64 shrink-0 border-r">
        <div className="p-1">
          {toolCalls.map((tc) => (
            <button
              key={tc.id}
              type="button"
              onClick={() => selectToolCall(tc.callId || tc.id)}
              className={cn(
                "flex items-center gap-2 w-full text-left px-3 py-2 rounded text-sm transition-colors",
                "hover:bg-muted/50",
                (selectedToolCallId === tc.callId || selectedToolCallId === tc.id) && "bg-muted"
              )}
              data-testid={`toolcall-item-${tc.callId || tc.id}`}
            >
              <Wrench className="h-3.5 w-3.5 text-orange-500 shrink-0" />
              <span className="font-mono truncate flex-1">{tc.name}</span>
              <span className="flex items-center gap-1 shrink-0">
                {tc.status && <ToolCallBadge status={tc.status} />}
                {tc.durationMs !== undefined && (
                  <span className="text-xs text-muted-foreground">{tc.durationMs}ms</span>
                )}
              </span>
            </button>
          ))}
        </div>
      </ScrollArea>

      {/* Right detail */}
      <div className="flex-1 flex flex-col min-h-0">
        {selectedTc ? (
          <div className="flex flex-col h-full p-4 gap-4" data-testid="toolcall-detail">
            {(selectedTc.labels?.handler_type || selectedTc.labels?.registry_name) && (
              <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm shrink-0" data-testid="toolcall-registry-info">
                {selectedTc.labels?.handler_type && (
                  <div className="contents">
                    <span className="font-medium text-muted-foreground">Handler</span>
                    <span className="font-mono">{selectedTc.labels.handler_name} ({selectedTc.labels.handler_type})</span>
                  </div>
                )}
                {selectedTc.labels?.registry_name && (
                  <div className="contents">
                    <span className="font-medium text-muted-foreground">Registry</span>
                    <span className="font-mono">{selectedTc.labels.registry_name}</span>
                  </div>
                )}
              </div>
            )}
            <div className="grid grid-cols-2 gap-4 flex-1 min-h-0">
              <div className="min-w-0 flex flex-col">
                <h4 className="text-sm font-medium text-muted-foreground mb-2 shrink-0">Arguments</h4>
                <div className="flex-1 min-h-0 overflow-auto rounded border bg-white dark:bg-zinc-950">
                  <RenderValue value={selectedTc.arguments} />
                </div>
              </div>
              {selectedTc.result !== undefined && (
                <div className="min-w-0 flex flex-col">
                  <h4 className={cn(
                    "text-sm font-medium mb-2 shrink-0",
                    selectedTc.errorMessage ? "text-destructive" : "text-muted-foreground"
                  )}>
                    {selectedTc.errorMessage ? "Error" : "Result"}
                  </h4>
                  <div className="flex-1 min-h-0 overflow-auto rounded border bg-white dark:bg-zinc-950">
                    <RenderValue value={typeof selectedTc.result === "string" ? tryParseJSON(selectedTc.result) : selectedTc.result} />
                  </div>
                </div>
              )}
              {!selectedTc.result && selectedTc.errorMessage && (
                <div className="min-w-0 flex flex-col">
                  <h4 className="text-sm font-medium mb-2 shrink-0 text-destructive">Error</h4>
                  <div className="flex-1 min-h-0 overflow-auto rounded border bg-white dark:bg-zinc-950">
                    <RenderValue value={selectedTc.errorMessage} />
                  </div>
                </div>
              )}
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-sm text-muted-foreground" data-testid="toolcall-no-selection">
            Select a tool call to view details
          </div>
        )}
      </div>
    </div>
  );
}
