"use client";

import { useMemo } from "react";
import { User, Bot, Wrench } from "lucide-react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { visibleEventsAt, toElapsedMs } from "@/lib/sessions/replay";
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

function MessageRow({ message }: { message: Message }) {
  const isUser = message.role === "user";
  const Icon = isUser ? User : Bot;
  return (
    <div className="flex gap-2 px-3 py-2">
      <Icon className="mt-1 h-4 w-4 flex-shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="text-xs uppercase tracking-wide text-muted-foreground">
          {isUser ? "User" : "Assistant"}
        </div>
        <div className="whitespace-pre-wrap break-words text-sm">{message.content}</div>
      </div>
    </div>
  );
}

function ToolCallRow({ toolCall }: { toolCall: ToolCall }) {
  return (
    <div className="flex gap-2 px-3 py-2">
      <Wrench className="mt-1 h-4 w-4 flex-shrink-0 text-orange-500" />
      <div className="min-w-0 flex-1">
        <div className="text-xs uppercase tracking-wide text-muted-foreground">
          Tool call — {toolCall.name}
        </div>
        <pre className="mt-1 overflow-x-auto rounded bg-muted p-2 text-xs">
          {JSON.stringify(toolCall.arguments, null, 2)}
        </pre>
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
  return (
    <ScrollArea className="h-full border rounded-md">
      <div className="divide-y">
        {rows.map((row) =>
          row.kind === "message" ? (
            <MessageRow key={`m:${row.message.id}`} message={row.message} />
          ) : (
            <ToolCallRow key={`t:${row.toolCall.id}`} toolCall={row.toolCall} />
          ),
        )}
      </div>
    </ScrollArea>
  );
}
