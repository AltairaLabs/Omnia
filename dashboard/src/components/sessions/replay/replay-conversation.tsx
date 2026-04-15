"use client";

import { useEffect, useMemo, useRef } from "react";
import { User, Bot, Wrench } from "lucide-react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { visibleEventsAt, toElapsedMs } from "@/lib/sessions/replay";
import { cn } from "@/lib/utils";
import type { Message, ToolCall } from "@/types/session";

interface ReplayConversationProps {
  readonly startedAt: string;
  readonly currentTimeMs: number;
  readonly messages: readonly Message[];
  readonly toolCalls: readonly ToolCall[];
}

type Row =
  | { kind: "message"; message: Message; elapsedMs: number }
  | { kind: "tool_call"; toolCall: ToolCall; elapsedMs: number };

function buildRows(
  startedAt: string,
  messages: readonly Message[],
  toolCalls: readonly ToolCall[],
): Row[] {
  const rows: Row[] = [
    ...messages.map<Row>((m) => ({
      kind: "message",
      message: m,
      elapsedMs: toElapsedMs(startedAt, m.timestamp),
    })),
    ...toolCalls.map<Row>((tc) => ({
      kind: "tool_call",
      toolCall: tc,
      elapsedMs: toElapsedMs(startedAt, tc.createdAt),
    })),
  ];
  rows.sort((a, b) => a.elapsedMs - b.elapsedMs);
  return rows;
}

function formatElapsed(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const millis = Math.floor(ms % 1000);
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}.${String(millis).padStart(3, "0")}`;
}

/** Server-side handler types per api/v1alpha1/toolregistry_types.go. */
const SERVER_HANDLER_TYPES = new Set(["http", "openapi", "grpc", "mcp"]);

function toolOrigin(tc: ToolCall): "client" | "server" | "unknown" {
  const handlerType = tc.labels?.handler_type;
  if (handlerType === "client") return "client";
  if (handlerType && SERVER_HANDLER_TYPES.has(handlerType)) return "server";
  return "unknown";
}

function shortArgs(args: Record<string, unknown>): string {
  const entries = Object.entries(args);
  if (entries.length === 0) return "";
  return entries
    .map(([k, v]) => {
      const val =
        typeof v === "string"
          ? JSON.stringify(v)
          : typeof v === "object" && v !== null
            ? "{…}"
            : String(v);
      return `${k}=${val}`;
    })
    .join(", ");
}

function shortResult(result: unknown): string | null {
  if (result === undefined || result === null) return null;
  if (typeof result === "string") {
    return result.length > 120 ? result.slice(0, 117) + "…" : result;
  }
  try {
    const json = JSON.stringify(result);
    return json.length > 120 ? json.slice(0, 117) + "…" : json;
  } catch {
    return String(result);
  }
}

function MessageRow({
  message,
  elapsedMs,
}: {
  readonly message: Message;
  readonly elapsedMs: number;
}) {
  const isUser = message.role === "user";
  const Icon = isUser ? User : Bot;
  return (
    <div
      className={cn(
        "flex gap-3 border-b px-4 py-3",
        isUser ? "bg-primary/5" : "bg-transparent",
      )}
    >
      <div className="flex w-12 flex-shrink-0 flex-col items-start gap-1 font-mono text-[10px] text-muted-foreground tabular-nums">
        <span>{formatElapsed(elapsedMs)}</span>
      </div>
      <Icon
        className={cn(
          "mt-0.5 h-4 w-4 flex-shrink-0",
          isUser ? "text-primary" : "text-blue-500",
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
          {isUser ? "User" : "Assistant"}
        </div>
        <div className="mt-0.5 whitespace-pre-wrap break-words text-sm">
          {message.content || <span className="italic text-muted-foreground">(empty)</span>}
        </div>
      </div>
    </div>
  );
}

function ToolCallRow({
  toolCall,
  elapsedMs,
}: {
  readonly toolCall: ToolCall;
  readonly elapsedMs: number;
}) {
  const origin = toolOrigin(toolCall);
  const result = shortResult(toolCall.result);
  const isError = toolCall.status === "error";
  return (
    <div
      className={cn(
        "flex gap-3 border-b px-4 py-3",
        isError ? "bg-destructive/5" : "bg-amber-500/5",
      )}
    >
      <div className="flex w-12 flex-shrink-0 flex-col items-start gap-1 font-mono text-[10px] text-muted-foreground tabular-nums">
        <span>{formatElapsed(elapsedMs)}</span>
      </div>
      <Wrench
        className={cn(
          "mt-0.5 h-4 w-4 flex-shrink-0",
          isError ? "text-destructive" : "text-orange-500",
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wide">
          <span className="text-muted-foreground">Tool</span>
          <span
            className={cn(
              "rounded-sm px-1.5 py-0.5",
              origin === "client" && "bg-blue-500/15 text-blue-700 dark:text-blue-300",
              origin === "server" && "bg-purple-500/15 text-purple-700 dark:text-purple-300",
              origin === "unknown" && "bg-muted text-muted-foreground",
            )}
          >
            {origin}
          </span>
          {toolCall.durationMs !== undefined && (
            <span className="font-mono text-muted-foreground">{toolCall.durationMs}ms</span>
          )}
          {isError && (
            <span className="rounded-sm bg-destructive/15 px-1.5 py-0.5 text-destructive">
              error
            </span>
          )}
        </div>
        <div className="mt-0.5 font-mono text-sm">
          <span className="font-semibold">{toolCall.name}</span>
          <span className="text-muted-foreground">({shortArgs(toolCall.arguments)})</span>
        </div>
        {result && !isError && (
          <div className="mt-1 font-mono text-xs text-muted-foreground">
            <span className="mr-1 opacity-60">→</span>
            {result}
          </div>
        )}
        {isError && toolCall.errorMessage && (
          <div className="mt-1 font-mono text-xs text-destructive">
            {toolCall.errorMessage}
          </div>
        )}
      </div>
    </div>
  );
}

export function ReplayConversation({
  startedAt,
  currentTimeMs,
  messages,
  toolCalls,
}: ReplayConversationProps) {
  const visible = useMemo(
    () => visibleEventsAt({ startedAt, messages, toolCalls }, currentTimeMs),
    [startedAt, messages, toolCalls, currentTimeMs],
  );
  const rows = useMemo(
    () => buildRows(startedAt, visible.messages, visible.toolCalls),
    [startedAt, visible.messages, visible.toolCalls],
  );

  const scrollEndRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    scrollEndRef.current?.scrollIntoView({ block: "end" });
  }, [rows.length]);

  if (rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center rounded-md border text-sm text-muted-foreground">
        Waiting for the first message…
      </div>
    );
  }

  return (
    <ScrollArea className="h-full rounded-md border">
      <div>
        {rows.map((row) =>
          row.kind === "message" ? (
            <MessageRow
              key={`m:${row.message.id}`}
              message={row.message}
              elapsedMs={row.elapsedMs}
            />
          ) : (
            <ToolCallRow
              key={`t:${row.toolCall.id}`}
              toolCall={row.toolCall}
              elapsedMs={row.elapsedMs}
            />
          ),
        )}
        <div ref={scrollEndRef} />
      </div>
    </ScrollArea>
  );
}
