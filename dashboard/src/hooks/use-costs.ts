"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { CostData, CostOptions } from "@/lib/data/types";

export function useCosts(options: CostOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["costs", options, service.name],
    queryFn: async (): Promise<CostData> => {
      return service.getCosts(options);
    },
    // Refetch every 5 minutes for cost data
    refetchInterval: 5 * 60 * 1000,
  });
}
