"use client";

import { User, Bot, Loader2, Info } from "lucide-react";
import { cn } from "@/lib/utils";
import { ToolCallCard } from "./tool-call-card";
import type { ConsoleMessage as ConsoleMessageType } from "@/types/websocket";

interface ConsoleMessageProps {
  message: ConsoleMessageType;
  className?: string;
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function ConsoleMessage({ message, className }: ConsoleMessageProps) {
  const isUser = message.role === "user";
  const isSystem = message.role === "system";

  // System messages render as centered dividers
  if (isSystem) {
    return (
      <div className={cn("flex items-center justify-center gap-2 py-2", className)}>
        <div className="h-px flex-1 bg-border" />
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground px-2">
          <Info className="h-3 w-3" />
          <span>{message.content}</span>
          <span className="text-muted-foreground/60">
            {formatTime(message.timestamp)}
          </span>
        </div>
        <div className="h-px flex-1 bg-border" />
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex gap-3",
        isUser && "flex-row-reverse",
        className
      )}
    >
      {/* Avatar */}
      <div
        className={cn(
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-full",
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-secondary text-secondary-foreground"
        )}
      >
        {isUser ? (
          <User className="h-4 w-4" />
        ) : (
          <Bot className="h-4 w-4" />
        )}
      </div>

      {/* Content */}
      <div
        className={cn(
          "flex flex-col gap-2 max-w-[80%]",
          isUser && "items-end"
        )}
      >
        <div
          className={cn(
            "rounded-lg px-4 py-2",
            isUser
              ? "bg-primary text-primary-foreground"
              : "bg-secondary text-secondary-foreground"
          )}
        >
          {/* Message content */}
          <div className="whitespace-pre-wrap break-words">
            {message.content}
            {message.isStreaming && message.content.length > 0 && (
              <span className="inline-block w-2 h-4 ml-0.5 bg-current animate-pulse" />
            )}
          </div>

          {/* Streaming indicator for empty content */}
          {message.isStreaming && message.content.length === 0 && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span className="text-sm">Thinking...</span>
            </div>
          )}
        </div>

        {/* Tool calls */}
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div className="flex flex-col gap-2 w-full">
            {message.toolCalls.map((toolCall) => (
              <ToolCallCard key={toolCall.id} toolCall={toolCall} />
            ))}
          </div>
        )}

        {/* Timestamp */}
        <span className="text-xs text-muted-foreground">
          {formatTime(message.timestamp)}
        </span>
      </div>
    </div>
  );
}
