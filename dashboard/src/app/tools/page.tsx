"use client";

import { useState, useMemo, useCallback } from "react";
import { Plus } from "lucide-react";
import { Header } from "@/components/layout";
import { ToolRegistryCard } from "@/components/tools";
import { NamespaceFilter } from "@/components/filters";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { useToolRegistries, useSharedToolRegistries } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

type FilterPhase = "all" | ToolRegistryPhase;

/** Extended tool registry type with shared flag */
interface ToolRegistryWithSource extends ToolRegistry {
  _isShared?: boolean;
}

export default function ToolsPage() {
  const [filterPhase, setFilterPhase] = useState<FilterPhase>("all");
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);

  const { isLoading: isWorkspaceLoading } = useWorkspace();
  const { data: workspaceRegistries, isLoading: isLoadingRegistries } = useToolRegistries();
  const { data: sharedRegistries, isLoading: isLoadingShared } = useSharedToolRegistries();

  // Show loading when workspace, workspace registries, or shared registries are loading
  const isLoading = isWorkspaceLoading || isLoadingRegistries || isLoadingShared;

  // Combine shared and workspace registries, marking shared ones
  const registries = useMemo((): ToolRegistryWithSource[] => {
    const shared: ToolRegistryWithSource[] = (sharedRegistries || []).map((r) => ({
      ...r,
      _isShared: true,
    }));
    const workspace: ToolRegistryWithSource[] = (workspaceRegistries || []).map((r) => ({
      ...r,
      _isShared: false,
    }));
    return [...shared, ...workspace];
  }, [sharedRegistries, workspaceRegistries]);

  // Extract unique namespaces
  const allNamespaces = useMemo(() => {
    if (!registries) return [];
    return [...new Set(registries.map((r) => r.metadata.namespace).filter((ns): ns is string => !!ns))];
  }, [registries]);

  // Initialize selected namespaces when data loads
  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Filter by namespace first, then by phase
  const namespaceFilteredRegistries = useMemo(() => {
    if (!registries) return [];
    if (selectedNamespaces.length === 0) return registries;
    return registries.filter((r) => r.metadata.namespace && selectedNamespaces.includes(r.metadata.namespace));
  }, [registries, selectedNamespaces]);

  // Filter by phase and sort alphabetically by name
  const filteredRegistries = useMemo(() => {
    const filtered = filterPhase === "all"
      ? namespaceFilteredRegistries
      : namespaceFilteredRegistries.filter((r) => r.status?.phase === filterPhase);
    // Sort alphabetically by name for stable ordering
    return [...filtered].sort((a, b) =>
      (a.metadata.name || "").localeCompare(b.metadata.name || "")
    );
  }, [namespaceFilteredRegistries, filterPhase]);

  const phaseCounts = namespaceFilteredRegistries.reduce(
    (acc, registry) => {
      const phase = registry.status?.phase;
      if (phase === "Ready") acc.ready++;
      else if (phase === "Degraded") acc.degraded++;
      else if (phase === "Pending") acc.pending++;
      else if (phase === "Failed") acc.failed++;
      return acc;
    },
    { ready: 0, degraded: 0, pending: 0, failed: 0 }
  );

  // Calculate total tools across all registries
  const totalTools = registries?.reduce(
    (sum, r) => sum + (r.status?.discoveredToolsCount || 0),
    0
  ) ?? 0;

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Tools"
        description={`${totalTools} tools across ${registries?.length ?? 0} registries`}
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
                  All ({namespaceFilteredRegistries.length})
                </TabsTrigger>
                <TabsTrigger value="Ready">
                  Ready ({phaseCounts?.ready ?? 0})
                </TabsTrigger>
                <TabsTrigger value="Degraded">
                  Degraded ({phaseCounts?.degraded ?? 0})
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

          <Button size="sm">
            <Plus className="mr-2 h-4 w-4" />
            New ToolRegistry
          </Button>
        </div>

        {/* Content */}
        {isLoading ? (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {["sk-1", "sk-2", "sk-3", "sk-4"].map((id) => (
              <Skeleton key={id} className="h-[220px] rounded-lg" />
            ))}
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {filteredRegistries?.map((registry) => (
              <ToolRegistryCard
                key={registry.metadata.uid}
                registry={registry}
                isShared={registry._isShared}
              />
            ))}
          </div>
        )}

        {!isLoading && filteredRegistries?.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">No ToolRegistries found</p>
            <Button variant="outline" className="mt-4">
              <Plus className="mr-2 h-4 w-4" />
              Create your first ToolRegistry
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
