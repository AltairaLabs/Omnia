"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

export interface UseArenaJobLogsOptions {
  tailLines?: number;
  sinceSeconds?: number;
  refetchInterval?: number;
}

/**
 * Hook to fetch logs for an Arena job.
 *
 * In demo mode: MockDataService returns generated mock logs.
 * In live mode: ArenaService fetches real logs from K8s worker pods.
 *
 * @param workspace - Workspace name (not K8s namespace)
 * @param jobName - Arena job name
 * @param options - Log fetch options
 */
export function useArenaJobLogs(
  workspace: string,
  jobName: string,
  options: UseArenaJobLogsOptions = {}
) {
  const service = useDataService();

  return useQuery({
    queryKey: ["arenaJobLogs", workspace, jobName, options, service.name],
    queryFn: async () => {
      const logs = await service.getArenaJobLogs(workspace, jobName, {
        tailLines: options.tailLines || 200,
        sinceSeconds: options.sinceSeconds || 3600,
      });
      return logs;
    },
    // In demo mode, refetch less frequently since logs are generated
    refetchInterval: service.isDemo ? 2000 : (options.refetchInterval || 5000),
  });
}
