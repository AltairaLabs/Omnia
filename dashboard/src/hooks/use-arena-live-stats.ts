"use client";

import { useEventSource } from "./use-event-source";
import type { ArenaLiveStats } from "@/lib/redis/arena-stats";

/**
 * Hook to consume live arena job stats via SSE.
 *
 * Connects to the live-stats SSE endpoint for the given job and returns
 * the latest stats snapshot. The connection is only active when `enabled`
 * is true (typically when the job is Running).
 *
 * @param workspace - Workspace name.
 * @param jobName - Arena job name.
 * @param enabled - Whether to connect (should be true only for running jobs).
 */
export function useArenaLiveStats(
  workspace: string,
  jobName: string,
  enabled: boolean
) {
  const url = enabled
    ? `/api/workspaces/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(jobName)}/live-stats`
    : null;

  return useEventSource<ArenaLiveStats>(url, { enabled });
}
