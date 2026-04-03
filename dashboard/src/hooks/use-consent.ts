"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { useAuth } from "@/hooks/use-auth";
import { ConsentService } from "@/lib/data/consent-service";
import type { ConsentRequest } from "@/lib/data/types";

const service = new ConsentService();

export function useConsent() {
  const { currentWorkspace } = useWorkspace();
  const { user } = useAuth();

  return useQuery({
    queryKey: ["consent", currentWorkspace?.name, user?.id],
    queryFn: async () => {
      if (!currentWorkspace || !user?.id) {
        return { grants: [] as string[], defaults: [] as string[], denied: [] as string[] };
      }
      return service.getConsent(currentWorkspace.name, user.id);
    },
    enabled: !!currentWorkspace && !!user?.id,
    staleTime: 30000,
  });
}

export function useUpdateConsent() {
  const { currentWorkspace } = useWorkspace();
  const { user } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: ConsentRequest) => {
      if (!currentWorkspace || !user?.id) {
        throw new Error("No workspace or user");
      }
      return service.updateConsent(currentWorkspace.name, user.id, request);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["consent"] });
    },
  });
}
