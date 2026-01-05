"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchNamespaces } from "@/lib/api/client";

export function useNamespaces() {
  return useQuery({
    queryKey: ["namespaces"],
    queryFn: fetchNamespaces,
    staleTime: 60000, // Cache for 1 minute
  });
}
