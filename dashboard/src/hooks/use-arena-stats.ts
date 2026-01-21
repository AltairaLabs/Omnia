"use client";

/**
 * Arena Stats hook for fetching Arena statistics and recent jobs.
 *
 * DEMO MODE SUPPORT:
 * These hooks use the DataService abstraction to support both demo mode (mock data)
 * and live mode (real K8s API). When adding new functionality:
 * 1. Add the method to DataService interface in src/lib/data/types.ts
 * 2. Implement in MockDataService (src/lib/data/mock-service.ts) for demo mode
 * 3. Implement in LiveDataService (src/lib/data/live-service.ts) for production
 * 4. Use useDataService() in hooks to get the appropriate implementation
 *
 * This ensures the UI works in demo mode without requiring K8s access.
 */

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import { useDataService } from "@/lib/data/provider";
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
  configs: { total: 0, ready: 0, scenarios: 0 },
  jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
};

/**
 * Hook to fetch Arena statistics and recent jobs for the current workspace.
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaStats(): UseArenaStatsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
      const [statsData, jobsData] = await Promise.all([
        service.getArenaStats(workspace),
        service.getArenaJobs(workspace, { sort: "recent", limit: 5 }),
      ]);

      setStats(statsData);
      setRecentJobs(jobsData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setStats(null);
      setRecentJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, service]);

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
