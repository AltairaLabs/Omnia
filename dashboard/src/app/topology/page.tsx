"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { TopologyGraph } from "@/components/topology";
import { Skeleton } from "@/components/ui/skeleton";
import { Bot, FileText, Package, Wrench } from "lucide-react";
import { useAgents, usePromptPacks, useToolRegistries } from "@/hooks";

export default function TopologyPage() {
  const router = useRouter();

  const { data: agents, isLoading: agentsLoading } = useAgents();
  const { data: promptPacks, isLoading: promptPacksLoading } = usePromptPacks();
  const { data: toolRegistries, isLoading: toolRegistriesLoading } = useToolRegistries();

  const isLoading = agentsLoading || promptPacksLoading || toolRegistriesLoading;

  const handleNodeClick = useCallback(
    (type: string, name: string, namespace: string) => {
      switch (type) {
        case "agent":
          router.push(`/agents/${name}?namespace=${namespace}`);
          break;
        case "promptpack":
          router.push(`/promptpacks/${name}?namespace=${namespace}`);
          break;
        case "tools":
          router.push(`/tools/${name}?namespace=${namespace}`);
          break;
      }
    },
    [router]
  );

  // Calculate stats
  const totalTools = toolRegistries?.reduce(
    (sum, r) => sum + (r.status?.discoveredToolsCount || 0),
    0
  ) ?? 0;

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Topology"
        description="Visualize relationships between Agents, PromptPacks, and Tools"
      />

      <div className="flex-1 p-6 space-y-4">
        {/* Legend */}
        <div className="flex items-center gap-6 text-sm">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-blue-500" />
            <Bot className="h-4 w-4 text-blue-600" />
            <span className="text-muted-foreground">Agents ({agents?.length ?? 0})</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-purple-500" />
            <FileText className="h-4 w-4 text-purple-600" />
            <span className="text-muted-foreground">PromptPacks ({promptPacks?.length ?? 0})</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-orange-500" />
            <Package className="h-4 w-4 text-orange-600" />
            <span className="text-muted-foreground">ToolRegistries ({toolRegistries?.length ?? 0})</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-teal-500" />
            <Wrench className="h-4 w-4 text-teal-600" />
            <span className="text-muted-foreground">Tools ({totalTools})</span>
          </div>
        </div>

        {/* Graph */}
        <div className="flex-1 min-h-[600px] border rounded-lg bg-card">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <Skeleton className="w-full h-full" />
            </div>
          ) : (
            <TopologyGraph
              agents={agents || []}
              promptPacks={promptPacks || []}
              toolRegistries={toolRegistries || []}
              onNodeClick={handleNodeClick}
              className="w-full h-[600px]"
            />
          )}
        </div>
      </div>
    </div>
  );
}
