"use client";

import { cn } from "@/lib/utils";
import {
  useResultsPanelStore,
  type ResultsPanelTab,
} from "@/stores/results-panel-store";
import { AlertCircle, ScrollText, BarChart3, MessageSquare } from "lucide-react";

interface ResultsPanelTabsProps {
  readonly className?: string;
}

const TABS: Array<{
  id: ResultsPanelTab;
  label: string;
  icon: React.ReactNode;
}> = [
  {
    id: "problems",
    label: "Problems",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  {
    id: "logs",
    label: "Logs",
    icon: <ScrollText className="h-4 w-4" />,
  },
  {
    id: "results",
    label: "Results",
    icon: <BarChart3 className="h-4 w-4" />,
  },
  {
    id: "console",
    label: "Console",
    icon: <MessageSquare className="h-4 w-4" />,
  },
];

/**
 * Tab navigation for the results panel.
 */
export function ResultsPanelTabs({ className }: ResultsPanelTabsProps) {
  const activeTab = useResultsPanelStore((state) => state.activeTab);
  const setActiveTab = useResultsPanelStore((state) => state.setActiveTab);
  const problemsCount = useResultsPanelStore((state) => state.problemsCount);

  return (
    <div
      className={cn(
        "flex items-center gap-1 px-2 border-b bg-muted/30",
        className
      )}
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
        >
          {tab.icon}
          <span>{tab.label}</span>
          {tab.id === "problems" && problemsCount > 0 && (
            <span
              className={cn(
                "ml-1 px-1.5 py-0.5 text-xs rounded-full",
                "bg-destructive text-destructive-foreground"
              )}
            >
              {problemsCount}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}
