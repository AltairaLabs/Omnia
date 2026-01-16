"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  getSecretsService,
  type SecretSummary,
  type SecretWriteRequest,
} from "@/lib/data/secrets-service";

interface UseSecretsOptions {
  namespace?: string;
}

/**
 * Hook to list credential secrets.
 */
export function useSecrets(options: UseSecretsOptions = {}) {
  const service = getSecretsService();

  return useQuery({
    queryKey: ["secrets", options.namespace],
    queryFn: async (): Promise<SecretSummary[]> => {
      return service.listSecrets(options.namespace);
    },
  });
}

/**
 * Hook to get a single secret's metadata.
 */
export function useSecret(namespace: string, name: string | undefined) {
  const service = getSecretsService();

  return useQuery({
    queryKey: ["secret", namespace, name],
    queryFn: async (): Promise<SecretSummary | null> => {
      if (!name) return null;
      return service.getSecret(namespace, name);
    },
    enabled: !!name,
  });
}

/**
 * Hook to create or update a secret.
 */
export function useCreateSecret() {
  const queryClient = useQueryClient();
  const service = getSecretsService();

  return useMutation({
    mutationFn: async (request: SecretWriteRequest): Promise<SecretSummary> => {
      return service.createOrUpdateSecret(request);
    },
    onSuccess: (_, variables) => {
      // Invalidate both the list and specific secret queries
      queryClient.invalidateQueries({ queryKey: ["secrets"] });
      queryClient.invalidateQueries({
        queryKey: ["secret", variables.namespace, variables.name],
      });
    },
  });
}

/**
 * Hook to delete a secret.
 */
export function useDeleteSecret() {
  const queryClient = useQueryClient();
  const service = getSecretsService();

  return useMutation({
    mutationFn: async ({
      namespace,
      name,
    }: {
      namespace: string;
      name: string;
    }): Promise<boolean> => {
      return service.deleteSecret(namespace, name);
    },
    onSuccess: (_, variables) => {
      // Invalidate both the list and specific secret queries
      queryClient.invalidateQueries({ queryKey: ["secrets"] });
      queryClient.invalidateQueries({
        queryKey: ["secret", variables.namespace, variables.name],
      });
    },
  });
}
