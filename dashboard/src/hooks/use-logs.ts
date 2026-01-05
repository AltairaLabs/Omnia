import { useQuery } from "@tanstack/react-query";
import { fetchAgentLogs, isDemoMode } from "@/lib/api/client";

export interface UseLogsOptions {
  tailLines?: number;
  sinceSeconds?: number;
  container?: string;
  refetchInterval?: number;
}

export function useLogs(
  namespace: string,
  name: string,
  options: UseLogsOptions = {}
) {
  return useQuery({
    queryKey: ["logs", namespace, name, options],
    queryFn: async () => {
      const logs = await fetchAgentLogs(namespace, name, {
        tailLines: options.tailLines || 200,
        sinceSeconds: options.sinceSeconds || 3600,
        container: options.container,
      });
      return logs;
    },
    refetchInterval: options.refetchInterval || 5000, // Poll every 5 seconds by default
    // In demo mode, don't refetch (the LogViewer generates mock data)
    enabled: !isDemoMode,
  });
}
