"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

export function useNamespaces() {
  const service = useDataService();

  return useQuery({
    queryKey: ["namespaces", service.name],
    queryFn: () => service.getNamespaces(),
    staleTime: 60000, // Cache for 1 minute
  });
}
