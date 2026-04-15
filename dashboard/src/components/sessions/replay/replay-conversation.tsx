"use client";

import { useEffect, useMemo, useRef } from "react";
import { Wrench } from "lucide-react";
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
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

function MessageBubble({
  message,
  elapsedMs,
}: {
  readonly message: Message;
  readonly elapsedMs: number;
}) {
  const isUser = message.role === "user";
  return (
    <div className={cn("flex w-full", isUser ? "justify-end" : "justify-start")}>
      <div className="flex max-w-[80%] flex-col gap-0.5">
        <div
          className={cn(
            "text-[10px] uppercase tracking-wide text-muted-foreground",
            isUser ? "text-right" : "text-left",
          )}
        >
          {isUser ? "You" : "Assistant"} · {formatElapsed(elapsedMs)}
        </div>
        <div
          className={cn(
            "rounded-2xl px-3 py-2 text-sm shadow-sm whitespace-pre-wrap break-words",
            isUser
              ? "bg-primary text-primary-foreground rounded-br-sm"
              : "bg-muted text-foreground rounded-bl-sm",
          )}
        >
          {message.content || <span className="italic opacity-60">(empty)</span>}
        </div>
      </div>
    </div>
  );
}

function ToolCallNotice({
  toolCall,
  elapsedMs,
}: {
  readonly toolCall: ToolCall;
  readonly elapsedMs: number;
}) {
  return (
    <div className="flex w-full justify-center">
      <div className="flex items-center gap-2 rounded-full border border-orange-500/30 bg-orange-500/10 px-3 py-1 text-xs text-orange-700 dark:text-orange-300">
        <Wrench className="h-3 w-3" />
        <span className="font-medium">{toolCall.name}</span>
        <span className="font-mono opacity-60">· {formatElapsed(elapsedMs)}</span>
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
      <div className="flex flex-col gap-3 p-4">
        {rows.map((row) =>
          row.kind === "message" ? (
            <MessageBubble
              key={`m:${row.message.id}`}
              message={row.message}
              elapsedMs={row.elapsedMs}
            />
          ) : (
            <ToolCallNotice
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
