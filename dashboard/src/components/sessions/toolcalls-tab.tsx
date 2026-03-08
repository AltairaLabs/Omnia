"use client";

import { useMemo } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { ToolCallBadge } from "./tool-call-badge";
import { JsonBlock } from "@/components/ui/json-block";
import { Wrench } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/types/session";

/**
 * Extracted tool call info from a tool_call message, paired with its result.
 */
interface ExtractedToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
  result?: string;
  resultIsError?: boolean;
  status?: "success" | "error" | "pending";
  duration?: number;
  handlerName?: string;
  handlerType?: string;
  registryName?: string;
}

interface ToolCallsTabProps {
  readonly messages: Message[];
}

function RenderValue({ value }: Readonly<{ value: unknown }>) {
  return <JsonBlock data={value} />;
}

function tryParseJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

/**
 * Extract tool call data from a tool_call message.
 */
function extractToolCall(msg: Message): ExtractedToolCall {
  let name = "unknown";
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(msg.content);
    name = parsed.name || name;
    args = parsed.arguments || args;
  } catch {
    // Content is not valid JSON
  }

  const durationStr = msg.metadata?.duration_ms;
  const duration = durationStr ? Number.parseInt(durationStr, 10) : undefined;

  return {
    id: msg.toolCallId || msg.id,
    name,
    arguments: args,
    status: (msg.metadata?.status as ExtractedToolCall["status"]) || undefined,
    duration: duration && !Number.isNaN(duration) ? duration : undefined,
    handlerName: msg.metadata?.handler_name || undefined,
    handlerType: msg.metadata?.handler_type || undefined,
    registryName: msg.metadata?.registry_name || undefined,
  };
}

export function ToolCallsTab({ messages }: ToolCallsTabProps) {
  const selectedToolCallId = useDebugPanelStore((s) => s.selectedToolCallId);
  const selectToolCall = useDebugPanelStore((s) => s.selectToolCall);

  const toolCalls = useMemo(() => {
    // Build a map of toolCallId → result content from tool_result messages.
    const resultsByCallId = new Map<string, { content: string; isError: boolean }>();
    for (const m of messages) {
      const mType = m.metadata?.type;
      if ((mType === "tool_result" || mType === "tool_call_completed" || m.role === "tool") && m.toolCallId) {
        resultsByCallId.set(m.toolCallId, {
          content: m.content,
          isError: m.metadata?.is_error === "true" || m.metadata?.status === "error",
        });
      }
    }

    return messages
      .filter((m) => m.metadata?.type === "tool_call")
      .map((m) => {
        const tc = extractToolCall(m);
        const result = resultsByCallId.get(tc.id);
        if (result) {
          tc.result = result.content;
          tc.resultIsError = result.isError;
        }
        return tc;
      });
  }, [messages]);

  const selectedTc = toolCalls.find((tc) => tc.id === selectedToolCallId);

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
              onClick={() => selectToolCall(tc.id)}
              className={cn(
                "flex items-center gap-2 w-full text-left px-3 py-2 rounded text-sm transition-colors",
                "hover:bg-muted/50",
                selectedToolCallId === tc.id && "bg-muted"
              )}
              data-testid={`toolcall-item-${tc.id}`}
            >
              <Wrench className="h-3.5 w-3.5 text-orange-500 shrink-0" />
              <span className="font-mono truncate flex-1">{tc.name}</span>
              <span className="flex items-center gap-1 shrink-0">
                {tc.status && <ToolCallBadge status={tc.status} />}
                {tc.duration !== undefined && (
                  <span className="text-xs text-muted-foreground">{tc.duration}ms</span>
                )}
              </span>
            </button>
          ))}
        </div>
      </ScrollArea>

      {/* Right detail */}
      <ScrollArea className="flex-1">
        {selectedTc ? (
          <div className="p-4 space-y-4" data-testid="toolcall-detail">
            {(selectedTc.handlerType || selectedTc.registryName) && (
              <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm" data-testid="toolcall-registry-info">
                {selectedTc.handlerType && (
                  <div className="contents">
                    <span className="font-medium text-muted-foreground">Handler</span>
                    <span className="font-mono">{selectedTc.handlerName} ({selectedTc.handlerType})</span>
                  </div>
                )}
                {selectedTc.registryName && (
                  <div className="contents">
                    <span className="font-medium text-muted-foreground">Registry</span>
                    <span className="font-mono">{selectedTc.registryName}</span>
                  </div>
                )}
              </div>
            )}
            <div>
              <h4 className="text-sm font-medium text-muted-foreground mb-2">Arguments</h4>
              <RenderValue value={selectedTc.arguments} />
            </div>
            {selectedTc.result !== undefined && (
              <div>
                <h4 className={cn(
                  "text-sm font-medium mb-2",
                  selectedTc.resultIsError ? "text-destructive" : "text-muted-foreground"
                )}>
                  {selectedTc.resultIsError ? "Error" : "Result"}
                </h4>
                <RenderValue value={tryParseJSON(selectedTc.result)} />
              </div>
            )}
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-sm text-muted-foreground" data-testid="toolcall-no-selection">
            Select a tool call to view details
          </div>
        )}
      </ScrollArea>
    </div>
  );
}
