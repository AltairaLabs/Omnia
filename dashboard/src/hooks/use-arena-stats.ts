"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaStats, ArenaJob } from "@/types/arena";

interface UseArenaStatsResult {
  stats: ArenaStats | null;
  recentJobs: ArenaJob[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

const EMPTY_STATS: ArenaStats = {
  sources: { total: 0, ready: 0, failed: 0, active: 0 },
  jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
};

/**
 * Hook to fetch Arena statistics and recent jobs for the current workspace.
 */
export function useArenaStats(): UseArenaStatsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [stats, setStats] = useState<ArenaStats | null>(null);
  const [recentJobs, setRecentJobs] = useState<ArenaJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setStats(EMPTY_STATS);
      setRecentJobs([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // Fetch stats and jobs in parallel
      const [statsResponse, jobsResponse] = await Promise.all([
        fetch(`/api/workspaces/${workspace}/arena/stats`),
        fetch(`/api/workspaces/${workspace}/arena/jobs?limit=5`),
      ]);

      if (!statsResponse.ok) {
        throw new Error(`Failed to fetch stats: ${statsResponse.statusText}`);
      }

      const statsData = await statsResponse.json();
      setStats(statsData);

      if (jobsResponse.ok) {
        const jobsData = await jobsResponse.json();
        // Sort by creation time, most recent first, and take top 5
        const sortedJobs = (jobsData as ArenaJob[])
          .sort((a, b) => {
            const timeA = a.metadata?.creationTimestamp || "";
            const timeB = b.metadata?.creationTimestamp || "";
            return timeB.localeCompare(timeA);
          })
          .slice(0, 5);
        setRecentJobs(sortedJobs);
      } else {
        setRecentJobs([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setStats(null);
      setRecentJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    stats,
    recentJobs,
    loading,
    error,
    refetch: fetchData,
  };
}
