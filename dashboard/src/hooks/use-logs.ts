"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

export interface UseLogsOptions {
  tailLines?: number;
  sinceSeconds?: number;
  container?: string;
  refetchInterval?: number;
}

/**
 * Hook to fetch logs for an agent.
 *
 * In demo mode: MockDataService returns generated mock logs.
 * In live mode: OperatorApiService fetches real logs from K8s pods.
 */
export function useLogs(
  namespace: string,
  name: string,
  options: UseLogsOptions = {}
) {
  const service = useDataService();

  return useQuery({
    queryKey: ["logs", namespace, name, options, service.name],
    queryFn: async () => {
      const logs = await service.getAgentLogs(namespace, name, {
        tailLines: options.tailLines || 200,
        sinceSeconds: options.sinceSeconds || 3600,
        container: options.container,
      });
      return logs;
    },
    // In demo mode, refetch less frequently since logs are generated
    refetchInterval: service.isDemo ? 2000 : (options.refetchInterval || 5000),
  });
}
