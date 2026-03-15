"use client";

import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { DebugPanelTabs } from "./debug-panel-tabs";
import { TimelineTab } from "./timeline-tab";
import { ToolCallsTab } from "./toolcalls-tab";
import { RawTab } from "./raw-tab";
import { Button } from "@/components/ui/button";
import { ChevronUp, ChevronDown, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Message, Session, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";

interface DebugPanelProps {
  readonly messages: Message[];
  readonly session: Session;
  readonly toolCalls?: ToolCall[];
  readonly providerCalls?: ProviderCall[];
  readonly runtimeEvents?: RuntimeEvent[];
  readonly className?: string;
}

export function DebugPanel({ messages, session, toolCalls, providerCalls, runtimeEvents, className }: DebugPanelProps) {
  const isOpen = useDebugPanelStore((s) => s.isOpen);
  const activeTab = useDebugPanelStore((s) => s.activeTab);
  const toggle = useDebugPanelStore((s) => s.toggle);
  const close = useDebugPanelStore((s) => s.close);

  const toolCallCount = toolCalls?.length ?? 0;

  if (!isOpen) {
    return (
      <div
        className={cn(
          "flex items-center justify-between px-4 py-1.5 border-t bg-muted/30 shrink-0",
          className
        )}
        data-testid="debug-panel-collapsed"
      >
        <span className="text-xs text-muted-foreground">
          Debug: Timeline, Tool Calls, Raw
        </span>
        <Button variant="ghost" size="sm" onClick={toggle} data-testid="debug-panel-expand">
          <ChevronUp className="h-4 w-4" />
        </Button>
      </div>
    );
  }

  return (
    <div
      className={cn("flex flex-col h-full border-t", className)}
      data-testid="debug-panel"
    >
      <div className="flex items-center justify-between shrink-0">
        <DebugPanelTabs toolCallCount={toolCallCount} />
        <div className="flex items-center gap-1 px-2">
          <Button variant="ghost" size="sm" onClick={toggle} data-testid="debug-panel-minimize">
            <ChevronDown className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="sm" onClick={close} data-testid="debug-panel-close">
            <X className="h-4 w-4" />
          </Button>
        </div>
      </div>
      <div className="flex-1 min-h-0">
        {activeTab === "timeline" && <TimelineTab messages={messages} toolCalls={toolCalls} providerCalls={providerCalls} runtimeEvents={runtimeEvents} />}
        {activeTab === "toolcalls" && <ToolCallsTab toolCalls={toolCalls || []} />}
        {activeTab === "raw" && <RawTab session={session} />}
      </div>
    </div>
  );
}
