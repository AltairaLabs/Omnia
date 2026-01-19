/**
 * API route for workspace stats.
 *
 * GET /api/workspaces/:name/stats - Get aggregated stats for the workspace
 *
 * Computes stats from workspace-scoped resources (agents, promptpacks)
 * and shared resources (toolregistries).
 */

import { NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { listCrd, listSharedCrd } from "@/lib/k8s/crd-operations";
import { validateWorkspace, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { AgentRuntime, PromptPack, ToolRegistry } from "@/lib/data/types";

interface Stats {
  agents: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  promptPacks: {
    total: number;
    active: number;
    canary: number;
  };
  tools: {
    total: number;
    available: number;
    degraded: number;
  };
}

/**
 * GET /api/workspaces/:name/stats
 *
 * Get aggregated stats for the workspace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess("viewer", async (req, ctx, access) => {
  const { name: workspaceName } = await ctx.params;

  const validation = await validateWorkspace(workspaceName, access.role!);
  if (!validation.ok) return validation.response;

  // Fetch all resources in parallel
  const [agents, promptPacks, workspaceToolRegistries, sharedToolRegistries] = await Promise.all([
    listCrd<AgentRuntime>(validation.clientOptions, "agentruntimes").catch(() => []),
    listCrd<PromptPack>(validation.clientOptions, "promptpacks").catch(() => []),
    listCrd<ToolRegistry>(validation.clientOptions, "toolregistries").catch(() => []),
    listSharedCrd<ToolRegistry>("toolregistries", SYSTEM_NAMESPACE).catch(() => []),
  ]);

  // Combine workspace and shared tool registries (dedupe by name)
  const toolRegistryMap = new Map<string, ToolRegistry>();
  for (const tr of sharedToolRegistries) {
    if (tr.metadata?.name) {
      toolRegistryMap.set(tr.metadata.name, tr);
    }
  }
  for (const tr of workspaceToolRegistries) {
    if (tr.metadata?.name) {
      toolRegistryMap.set(tr.metadata.name, tr);
    }
  }
  const toolRegistries = Array.from(toolRegistryMap.values());

  // Calculate agent stats
  const agentStats = agents.reduce(
    (acc, agent) => {
      acc.total++;
      const phase = agent.status?.phase;
      if (phase === "Running") acc.running++;
      else if (phase === "Pending") acc.pending++;
      else if (phase === "Failed") acc.failed++;
      return acc;
    },
    { total: 0, running: 0, pending: 0, failed: 0 }
  );

  // Calculate promptpack stats
  const promptPackStats = promptPacks.reduce(
    (acc, pack) => {
      acc.total++;
      const phase = pack.status?.phase;
      if (phase === "Active") acc.active++;
      else if (phase === "Canary") acc.canary++;
      return acc;
    },
    { total: 0, active: 0, canary: 0 }
  );

  // Calculate tool registry stats
  const toolStats = toolRegistries.reduce(
    (acc, registry) => {
      acc.total++;
      const phase = registry.status?.phase;
      if (phase === "Ready") acc.available++;
      else if (phase === "Degraded") acc.degraded++;
      return acc;
    },
    { total: 0, available: 0, degraded: 0 }
  );

  const stats: Stats = {
    agents: agentStats,
    promptPacks: promptPackStats,
    tools: toolStats,
  };

  return NextResponse.json(stats);
});
