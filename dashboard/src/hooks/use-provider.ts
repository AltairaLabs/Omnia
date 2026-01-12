"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { Provider } from "@/types";

export function useProvider(name: string | undefined, namespace: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["provider", namespace, name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      if (!name) return null;
      const response = await service.getProvider(namespace, name);
      return (response as Provider) || null;
    },
    enabled: !!name,
  });
}
