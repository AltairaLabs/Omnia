"use client";

import { useState, useMemo, useCallback } from "react";
import { LayoutGrid, List, Plus } from "lucide-react";
import { Header } from "@/components/layout";
import { AgentCard, AgentTable, DeployWizard } from "@/components/agents";
import { NamespaceFilter } from "@/components/filters";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgents } from "@/hooks";
import type { AgentRuntimePhase } from "@/types";

type ViewMode = "cards" | "table";
type FilterPhase = "all" | AgentRuntimePhase;

/**
 * Render loading skeleton based on view mode.
 */
function renderLoadingSkeleton(viewMode: ViewMode) {
  if (viewMode === "cards") {
    return (
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {["sk-1", "sk-2", "sk-3", "sk-4", "sk-5", "sk-6"].map((id) => (
          <Skeleton key={id} className="h-[180px] rounded-lg" />
        ))}
      </div>
    );
  }
  return <Skeleton className="h-[400px] rounded-lg" />;
}

export default function AgentsPage() {
  const [viewMode, setViewMode] = useState<ViewMode>("cards");
  const [filterPhase, setFilterPhase] = useState<FilterPhase>("all");
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);
  const [wizardOpen, setWizardOpen] = useState(false);

  const { data: agents, isLoading } = useAgents();

  // Extract unique namespaces
  const allNamespaces = useMemo(() => {
    if (!agents) return [];
    return [...new Set(agents.map((a) => a.metadata.namespace).filter((ns): ns is string => !!ns))];
  }, [agents]);

  // Initialize selected namespaces when data loads
  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Filter by namespace first, then by phase
  const namespaceFilteredAgents = useMemo(() => {
    if (!agents) return [];
    if (selectedNamespaces.length === 0) return agents;
    return agents.filter((a) => a.metadata.namespace && selectedNamespaces.includes(a.metadata.namespace));
  }, [agents, selectedNamespaces]);

  const filteredAgents =
    filterPhase === "all"
      ? namespaceFilteredAgents
      : namespaceFilteredAgents.filter((a) => a.status?.phase === filterPhase);

  const phaseCounts = namespaceFilteredAgents.reduce(
    (acc, agent) => {
      const phase = agent.status?.phase;
      if (phase === "Running") acc.running++;
      else if (phase === "Pending") acc.pending++;
      else if (phase === "Failed") acc.failed++;
      return acc;
    },
    { running: 0, pending: 0, failed: 0 }
  );

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Agents"
        description="Manage your AI agent deployments"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Tabs
              value={filterPhase}
              onValueChange={(v) => setFilterPhase(v as FilterPhase)}
            >
              <TabsList>
                <TabsTrigger value="all">
                  All ({namespaceFilteredAgents.length})
                </TabsTrigger>
                <TabsTrigger value="Running">
                  Running ({phaseCounts?.running ?? 0})
                </TabsTrigger>
                <TabsTrigger value="Pending">
                  Pending ({phaseCounts?.pending ?? 0})
                </TabsTrigger>
                <TabsTrigger value="Failed">
                  Failed ({phaseCounts?.failed ?? 0})
                </TabsTrigger>
              </TabsList>
            </Tabs>
            <NamespaceFilter
              namespaces={allNamespaces}
              selectedNamespaces={selectedNamespaces}
              onSelectionChange={handleNamespaceChange}
            />
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center rounded-md border bg-muted p-1">
              <Button
                variant={viewMode === "cards" ? "secondary" : "ghost"}
                size="icon"
                className="h-7 w-7"
                onClick={() => setViewMode("cards")}
              >
                <LayoutGrid className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === "table" ? "secondary" : "ghost"}
                size="icon"
                className="h-7 w-7"
                onClick={() => setViewMode("table")}
              >
                <List className="h-4 w-4" />
              </Button>
            </div>
            <Button size="sm" onClick={() => setWizardOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              New Agent
            </Button>
          </div>
        </div>

        {/* Deploy Wizard */}
        <DeployWizard open={wizardOpen} onOpenChange={setWizardOpen} />

        {/* Content */}
        {isLoading && renderLoadingSkeleton(viewMode)}
        {!isLoading && viewMode === "cards" && (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {filteredAgents?.map((agent) => (
              <AgentCard key={agent.metadata.uid} agent={agent} />
            ))}
          </div>
        )}
        {!isLoading && viewMode !== "cards" && (
          <AgentTable agents={filteredAgents ?? []} />
        )}

        {!isLoading && filteredAgents?.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">No agents found</p>
            <Button variant="outline" className="mt-4" onClick={() => setWizardOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create your first agent
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
