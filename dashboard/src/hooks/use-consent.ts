"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { useAuth } from "@/hooks/use-auth";
import { ConsentService } from "@/lib/data/consent-service";
import type { ConsentRequest } from "@/lib/data/types";

const service = new ConsentService();

export function useConsent() {
  const { currentWorkspace } = useWorkspace();
  // Use the pseudonymous memory identity (authenticated user.id, or the
  // per-device deviceId for anonymous users) — NOT user.id, which collapses to
  // the literal "anonymous" and makes every anonymous user share one consent
  // record. Mirrors the memory views so consent is scoped to the same subject
  // as the memory it governs. See AltairaLabs/Omnia#1269.
  const { memoryUserId, hasMemoryIdentity } = useAuth();

  return useQuery({
    queryKey: ["consent", currentWorkspace?.name, memoryUserId],
    queryFn: async () => {
      if (!currentWorkspace || !memoryUserId) {
        return { grants: [] as string[], defaults: [] as string[], denied: [] as string[] };
      }
      return service.getConsent(currentWorkspace.name, memoryUserId);
    },
    enabled: !!currentWorkspace && hasMemoryIdentity,
    staleTime: 30000,
  });
}

export function useUpdateConsent() {
  const { currentWorkspace } = useWorkspace();
  const { memoryUserId } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: ConsentRequest) => {
      if (!currentWorkspace || !memoryUserId) {
        throw new Error("No workspace or user");
      }
      return service.updateConsent(currentWorkspace.name, memoryUserId, request);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["consent"] });
    },
  });
}
