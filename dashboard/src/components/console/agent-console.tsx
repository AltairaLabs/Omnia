"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Send, Trash2, Wifi, WifiOff, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useAgentConsole } from "@/hooks";
import { ConsoleMessage } from "./console-message";

interface AgentConsoleProps {
  agentName: string;
  namespace: string;
  className?: string;
}

export function AgentConsole({ agentName, namespace, className }: AgentConsoleProps) {
  const [input, setInput] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Always use mock mode for now (until K8s integration)
  const {
    sessionId,
    status,
    messages,
    error,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
  } = useAgentConsole({
    agentName,
    namespace,
  });

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  // Handle send
  const handleSend = useCallback(() => {
    if (input.trim() && status === "connected") {
      sendMessage(input);
      setInput("");
      textareaRef.current?.focus();
    }
  }, [input, status, sendMessage]);

  // Handle key press
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  // Status badge
  const statusBadge = {
    disconnected: (
      <Badge variant="outline" className="gap-1.5">
        <WifiOff className="h-3 w-3" />
        Disconnected
      </Badge>
    ),
    connecting: (
      <Badge variant="outline" className="gap-1.5 text-yellow-600 border-yellow-600/30 bg-yellow-500/10">
        <RefreshCw className="h-3 w-3 animate-spin" />
        Connecting
      </Badge>
    ),
    connected: (
      <Badge variant="outline" className="gap-1.5 text-green-600 border-green-600/30 bg-green-500/10">
        <Wifi className="h-3 w-3" />
        Connected
      </Badge>
    ),
    error: (
      <Badge variant="outline" className="gap-1.5 text-red-600 border-red-600/30 bg-red-500/10">
        <WifiOff className="h-3 w-3" />
        Error
      </Badge>
    ),
  }[status];

  return (
    <div className={cn("flex flex-col h-[600px] border rounded-lg", className)}>
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-muted/30">
        <div className="flex items-center gap-3">
          {statusBadge}
          {sessionId && (
            <span className="text-xs text-muted-foreground">
              Session: {sessionId.slice(0, 12)}...
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {status === "disconnected" && (
            <Button variant="outline" size="sm" onClick={connect}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Reconnect
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            onClick={clearMessages}
            disabled={messages.length === 0}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-sm">
          {error}
        </div>
      )}

      {/* Messages */}
      <ScrollArea className="flex-1 p-4" ref={scrollRef}>
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
            <p className="text-center">
              Start a conversation with <strong>{agentName}</strong>
            </p>
            <p className="text-sm mt-1">
              Type a message below and press Enter to send
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            {messages.map((message) => (
              <ConsoleMessage key={message.id} message={message} />
            ))}
          </div>
        )}
      </ScrollArea>

      {/* Input area */}
      <div className="p-4 border-t bg-muted/30">
        <div className="flex gap-2">
          <Textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={
              status === "connected"
                ? "Type a message... (Enter to send, Shift+Enter for new line)"
                : "Connect to start chatting..."
            }
            disabled={status !== "connected"}
            className="min-h-[44px] max-h-[120px] resize-none"
            rows={1}
          />
          <Button
            onClick={handleSend}
            disabled={!input.trim() || status !== "connected"}
            className="shrink-0"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
