"use client";

import { useEventSource } from "./use-event-source";
import type { RolloutConfig, RolloutStatus } from "@/types/agent-runtime";

/** Frame shape emitted by the rollout SSE route. */
export interface RolloutStreamFrame {
  spec: RolloutConfig | null;
  status: RolloutStatus | null;
}

/** SSE endpoint for an agent's live rollout, or null to disable the stream. */
export function rolloutStreamUrl(
  workspace: string | undefined,
  agentName: string | undefined,
  enabled = true,
): string | null {
  if (!enabled || !workspace || !agentName) {
    return null;
  }
  return `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(agentName)}/rollout/stream`;
}

/**
 * Live rollout state for an agent via SSE. The stream stays open while the page
 * is mounted, so it sees a rollout *start* without a page refresh. The server
 * holds the terminal (Promoted/Rolled-back) frame for a short window after the
 * operator clears it, so the outcome stays visible — the client just renders
 * whatever the stream reports.
 *
 * Returns null when the agent has no rollout.
 */
export function useAgentRolloutStream(
  workspace: string | undefined,
  agentName: string | undefined,
  enabled = true,
): RolloutStreamFrame | null {
  const { data } = useEventSource<RolloutStreamFrame>(
    rolloutStreamUrl(workspace, agentName, enabled),
  );
  return data;
}
