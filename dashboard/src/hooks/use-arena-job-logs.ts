"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

const terminalPhases = new Set(["Succeeded", "Failed", "Cancelled"]);

export interface UseArenaJobLogsOptions {
  tailLines?: number;
  sinceSeconds?: number;
  refetchInterval?: number;
  /** Job phase — when terminal, polling stops to preserve cached log data. */
  jobPhase?: string;
}

/**
 * Hook to fetch logs for an Arena job.
 *
 * In demo mode: MockDataService returns generated mock logs.
 * In live mode: ArenaService fetches real logs from K8s worker pods.
 *
 * When the job is in a terminal phase (Succeeded/Failed/Cancelled), polling
 * stops so that cached log data is preserved. For terminal jobs we also
 * omit sinceSeconds so the K8s API returns all available logs from the
 * pod's lifetime rather than a rolling time window.
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
  const isTerminal = !!options.jobPhase && terminalPhases.has(options.jobPhase);

  return useQuery({
    queryKey: ["arenaJobLogs", workspace, jobName, options.tailLines, options.sinceSeconds, options.jobPhase, service.name],
    queryFn: async () => {
      const logs = await service.getArenaJobLogs(workspace, jobName, {
        tailLines: options.tailLines || 200,
        // For terminal jobs, omit sinceSeconds to get all available logs
        // from the pod's lifetime instead of a rolling time window.
        sinceSeconds: isTerminal ? undefined : (options.sinceSeconds || 3600),
      });
      return logs;
    },
    refetchInterval: (() => {
      if (isTerminal) return false; // Stop polling for finished jobs — logs won't change
      if (service.isDemo) return 2000;
      return options.refetchInterval || 5000;
    })(),
  });
}
