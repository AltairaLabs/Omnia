"use client";

import { useEffect } from "react";
import { Plus, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { useConsoleTabStore } from "@/hooks/use-console-tab-store";
import { AgentConsole } from "./agent-console";
import { AgentSelector } from "./agent-selector";
import { Button } from "@/components/ui/button";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";

/**
 * Multi-session tabbed console container.
 * Manages multiple concurrent agent conversations in tabs.
 */
export function ConsoleTabs() {
  const {
    tabs,
    activeTabId,
    createTab,
    closeTab,
    setActiveTab,
    updateTab,
  } = useConsoleTabStore();

  // Create an initial tab if none exist
  useEffect(() => {
    if (tabs.length === 0) {
      createTab();
    }
  }, [tabs.length, createTab]);

  const activeTab = tabs.find((t) => t.id === activeTabId);

  const handleAgentSelect = (namespace: string, agentName: string) => {
    if (activeTabId) {
      updateTab(activeTabId, {
        state: "active",
        namespace,
        agentName,
      });
    }
  };

  const handleCloseTab = (e: React.MouseEvent, tabId: string) => {
    e.stopPropagation();
    closeTab(tabId);
  };

  const getTabTitle = (tab: typeof tabs[0]): string => {
    if (tab.state === "selecting") {
      return "New Session";
    }
    return tab.agentName || "Session";
  };

  return (
    <div className="flex flex-col h-full rounded-lg border bg-background">
      {/* Tab bar */}
      <div className="flex items-center border-b bg-muted/30">
        <ScrollArea className="flex-1">
          <div className="flex items-center p-1 gap-1">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "group flex items-center gap-2 px-3 py-1.5 text-sm rounded-md transition-colors",
                  "hover:bg-background/80",
                  activeTabId === tab.id
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground"
                )}
              >
                <span className="truncate max-w-[120px]">{getTabTitle(tab)}</span>
                {tab.state === "active" && tab.namespace && (
                  <span className="text-xs text-muted-foreground">
                    ({tab.namespace})
                  </span>
                )}
                <button
                  onClick={(e) => handleCloseTab(e, tab.id)}
                  className={cn(
                    "ml-1 p-0.5 rounded hover:bg-muted",
                    "opacity-0 group-hover:opacity-100 transition-opacity",
                    activeTabId === tab.id && "opacity-100"
                  )}
                  aria-label={`Close ${getTabTitle(tab)}`}
                >
                  <X className="h-3 w-3" />
                </button>
              </button>
            ))}
          </div>
          <ScrollBar orientation="horizontal" />
        </ScrollArea>

        {/* New tab button */}
        <div className="px-2">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={() => createTab()}
            aria-label="New tab"
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-hidden">
        {activeTab ? (
          activeTab.state === "selecting" ? (
            <AgentSelector onSelect={handleAgentSelect} />
          ) : activeTab.agentName && activeTab.namespace ? (
            <AgentConsole
              key={activeTab.id}
              sessionId={activeTab.id}
              agentName={activeTab.agentName}
              namespace={activeTab.namespace}
              className="h-full"
            />
          ) : null
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground">
            <p>No active session. Click + to create a new tab.</p>
          </div>
        )}
      </div>
    </div>
  );
}
