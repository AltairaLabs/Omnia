"use client";

import { cn } from "@/lib/utils";
import {
  useDebugPanelStore,
  type DebugPanelTab,
} from "@/stores/debug-panel-store";
import { ListOrdered, Wrench, FileJson } from "lucide-react";

interface DebugPanelTabsProps {
  readonly className?: string;
  readonly toolCallCount?: number;
}

const TABS: Array<{
  id: DebugPanelTab;
  label: string;
  icon: React.ReactNode;
}> = [
  {
    id: "timeline",
    label: "Timeline",
    icon: <ListOrdered className="h-4 w-4" />,
  },
  {
    id: "toolcalls",
    label: "Tool Calls",
    icon: <Wrench className="h-4 w-4" />,
  },
  {
    id: "raw",
    label: "Raw",
    icon: <FileJson className="h-4 w-4" />,
  },
];

export function DebugPanelTabs({ className, toolCallCount }: DebugPanelTabsProps) {
  const activeTab = useDebugPanelStore((state) => state.activeTab);
  const setActiveTab = useDebugPanelStore((state) => state.setActiveTab);

  return (
    <div
      className={cn(
        "flex items-center gap-1 px-2 border-b bg-muted/30",
        className
      )}
      data-testid="debug-panel-tabs"
    >
      {TABS.map((tab) => (
        <button
          key={tab.id}
          onClick={() => setActiveTab(tab.id)}
          className={cn(
            "flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-t-md transition-colors",
            "hover:bg-muted/50",
            activeTab === tab.id
              ? "bg-background text-foreground border-b-2 border-primary -mb-px"
              : "text-muted-foreground"
          )}
          data-testid={`debug-tab-${tab.id}`}
        >
          {tab.icon}
          <span>{tab.label}</span>
          {tab.id === "toolcalls" && toolCallCount !== undefined && toolCallCount > 0 && (
            <span
              className="ml-1 px-1.5 py-0.5 text-xs rounded-full bg-muted text-muted-foreground"
            >
              {toolCallCount}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}
