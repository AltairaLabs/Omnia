/**
 * /functions — Functions Phase 1 catalog page (#1103 PR 6).
 *
 * Lists function-mode AgentRuntimes. Reuses useAgents() and filters
 * down to spec.mode === "function" rather than calling a dedicated
 * endpoint — the function-mode rows are a strict subset of the
 * existing AgentRuntime list.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo, useCallback } from "react";
import { Header } from "@/components/layout";
import { NamespaceFilter } from "@/components/filters";
import { FunctionCard } from "@/components/functions/function-card";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgents } from "@/hooks/agents";
import { useWorkspace } from "@/contexts/workspace-context";
import { isFunctionMode } from "@/types/agent-runtime";

function renderLoadingSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {["sk-1", "sk-2", "sk-3", "sk-4", "sk-5", "sk-6"].map((id) => (
        <Skeleton key={id} className="h-[160px] rounded-lg" />
      ))}
    </div>
  );
}

export default function FunctionsPage() {
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);

  const { isLoading: isWorkspaceLoading } = useWorkspace();
  const { data: agents, isLoading: isAgentsLoading } = useAgents();
  const isLoading = isWorkspaceLoading || isAgentsLoading;

  const handleNamespaceChange = useCallback((namespaces: string[]) => {
    setSelectedNamespaces(namespaces);
  }, []);

  // Only function-mode runtimes appear on this page; agent-mode rows
  // belong to /agents and would just add noise here.
  const functions = useMemo(
    () => (agents ?? []).filter((a) => isFunctionMode(a.spec)),
    [agents],
  );

  // Build the namespace filter dropdown from the function set only —
  // showing namespaces that contain no functions would be misleading.
  const namespaces = useMemo(
    () => [
      ...new Set(
        functions
          .map((a) => a.metadata.namespace)
          .filter((ns): ns is string => Boolean(ns)),
      ),
    ],
    [functions],
  );

  const visible = useMemo(() => {
    const filtered =
      selectedNamespaces.length === 0
        ? functions
        : functions.filter(
            (a) => a.metadata.namespace && selectedNamespaces.includes(a.metadata.namespace),
          );
    return [...filtered].sort((a, b) =>
      (a.metadata.name || "").localeCompare(b.metadata.name || ""),
    );
  }, [functions, selectedNamespaces]);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Functions"
        description="One-shot PromptPack invocations with structured input and output schemas."
      />

      <div className="flex-1 p-6 space-y-6">
        <div className="flex items-center justify-between">
          <NamespaceFilter
            namespaces={namespaces}
            selectedNamespaces={selectedNamespaces}
            onSelectionChange={handleNamespaceChange}
          />
        </div>

        {isLoading && renderLoadingSkeleton()}

        {!isLoading && visible.length > 0 && (
          <div
            className="grid gap-4 md:grid-cols-2 lg:grid-cols-3"
            data-testid="functions-grid"
          >
            {visible.map((fn) => (
              <FunctionCard key={fn.metadata.uid ?? fn.metadata.name} fn={fn} />
            ))}
          </div>
        )}

        {!isLoading && visible.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">
              No function-mode AgentRuntimes found.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Set <code className="font-mono">spec.mode: function</code> on an
              AgentRuntime to register it here.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
