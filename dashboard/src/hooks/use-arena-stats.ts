"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaStats, ArenaJob } from "@/types/arena";

/** Polling interval when there are active (running/queued) jobs. */
const ACTIVE_POLL_INTERVAL = 10_000;

interface ArenaStatsData {
  stats: ArenaStats;
  recentJobs: ArenaJob[];
}

async function fetchArenaStats(workspace: string): Promise<ArenaStatsData> {
  const [statsResponse, jobsResponse] = await Promise.all([
    fetch(`/api/workspaces/${workspace}/arena/stats`),
    fetch(`/api/workspaces/${workspace}/arena/jobs?limit=5`),
  ]);

  if (!statsResponse.ok) {
    throw new Error(`Failed to fetch stats: ${statsResponse.statusText}`);
  }

  const stats: ArenaStats = await statsResponse.json();

  let recentJobs: ArenaJob[] = [];
  if (jobsResponse.ok) {
    const jobs: ArenaJob[] = await jobsResponse.json();
    recentJobs = jobs
      .toSorted((a, b) => {
        const timeA = a.metadata?.creationTimestamp || "";
        const timeB = b.metadata?.creationTimestamp || "";
        return timeB.localeCompare(timeA);
      })
      .slice(0, 5);
  }

  return { stats, recentJobs };
}

function hasActiveJobs(data: ArenaStatsData | undefined): boolean {
  if (!data) return false;
  const { running, queued } = data.stats.jobs;
  return running > 0 || queued > 0;
}

/**
 * Hook to fetch Arena statistics and recent jobs for the current workspace.
 * Auto-refreshes every 10s when there are running or queued jobs.
 */
export function useArenaStats() {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["arena-stats", workspace],
    queryFn: () => fetchArenaStats(workspace!),
    enabled: !!workspace,
    staleTime: ACTIVE_POLL_INTERVAL,
    refetchInterval: (query) => {
      return hasActiveJobs(query.state.data) ? ACTIVE_POLL_INTERVAL : false;
    },
  });

  return {
    stats: data?.stats ?? null,
    recentJobs: data?.recentJobs ?? [],
    loading: isLoading,
    error: error as Error | null,
    refetch,
  };
}
