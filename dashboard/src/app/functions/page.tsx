/**
 * /functions — Functions catalog page.
 *
 * Lists function-mode AgentRuntimes (spec.mode === "function"), a strict
 * subset of the AgentRuntime list. Mirrors the agents list controls: phase
 * filter tabs, namespace filter, and a cards/table view toggle.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo, useCallback } from "react";
import { LayoutGrid, List } from "lucide-react";
import { usePersistedViewMode } from "@/hooks/use-persisted-view-mode";
import { Header } from "@/components/layout";
import { NamespaceFilter } from "@/components/filters";
import { FunctionCard } from "@/components/functions/function-card";
import { FunctionTable } from "@/components/functions/function-table";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgents } from "@/hooks/agents";
import { useWorkspace } from "@/contexts/workspace-context";
import { isFunctionMode } from "@/types/agent-runtime";
import type { AgentRuntimePhase } from "@/types";

type ViewMode = "cards" | "table";
type FilterPhase = "all" | AgentRuntimePhase;

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

export default function FunctionsPage() {
  const [viewMode, setViewMode] = usePersistedViewMode<ViewMode>(
    "omnia-functions-view-mode",
    "cards",
  );
  const [filterPhase, setFilterPhase] = useState<FilterPhase>("all");
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);

  const { isLoading: isWorkspaceLoading } = useWorkspace();
  const { data: agents, isLoading: isAgentsLoading } = useAgents();
  const isLoading = isWorkspaceLoading || isAgentsLoading;

  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Only function-mode runtimes appear here; agent-mode rows belong to /agents.
  const functions = useMemo(
    () => (agents ?? []).filter((a) => isFunctionMode(a.spec)),
    [agents],
  );

  const namespaces = useMemo(
    () => [
      ...new Set(
        functions.map((a) => a.metadata.namespace).filter((ns): ns is string => Boolean(ns)),
      ),
    ],
    [functions],
  );

  const namespaceFiltered = useMemo(() => {
    if (selectedNamespaces.length === 0) return functions;
    return functions.filter(
      (a) => a.metadata.namespace && selectedNamespaces.includes(a.metadata.namespace),
    );
  }, [functions, selectedNamespaces]);

  const visible = useMemo(() => {
    const filtered =
      filterPhase === "all"
        ? namespaceFiltered
        : namespaceFiltered.filter((a) => a.status?.phase === filterPhase);
    return [...filtered].sort((a, b) =>
      (a.metadata.name || "").localeCompare(b.metadata.name || ""),
    );
  }, [namespaceFiltered, filterPhase]);

  const phaseCounts = namespaceFiltered.reduce(
    (acc, fn) => {
      const phase = fn.status?.phase;
      if (phase === "Running") acc.running++;
      else if (phase === "Pending") acc.pending++;
      else if (phase === "Failed") acc.failed++;
      return acc;
    },
    { running: 0, pending: 0, failed: 0 },
  );

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Functions"
        description="One-shot PromptPack invocations with structured input and output schemas."
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Tabs value={filterPhase} onValueChange={(v) => setFilterPhase(v as FilterPhase)}>
              <TabsList>
                <TabsTrigger value="all">All ({namespaceFiltered.length})</TabsTrigger>
                <TabsTrigger value="Running">Running ({phaseCounts.running})</TabsTrigger>
                <TabsTrigger value="Pending">Pending ({phaseCounts.pending})</TabsTrigger>
                <TabsTrigger value="Failed">Failed ({phaseCounts.failed})</TabsTrigger>
              </TabsList>
            </Tabs>
            <NamespaceFilter
              namespaces={namespaces}
              selectedNamespaces={selectedNamespaces}
              onSelectionChange={handleNamespaceChange}
            />
          </div>

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
        </div>

        {/* Content */}
        {isLoading && renderLoadingSkeleton(viewMode)}

        {!isLoading && visible.length > 0 && viewMode === "cards" && (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3" data-testid="functions-grid">
            {visible.map((fn) => (
              <FunctionCard key={fn.metadata.uid ?? fn.metadata.name} fn={fn} />
            ))}
          </div>
        )}

        {!isLoading && visible.length > 0 && viewMode === "table" && (
          <FunctionTable functions={visible} />
        )}

        {!isLoading && visible.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">No function-mode AgentRuntimes found.</p>
            <p className="mt-2 text-sm text-muted-foreground">
              Set <code className="font-mono">spec.mode: function</code> on an AgentRuntime to
              register it here.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
