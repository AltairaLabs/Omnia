/**
 * Mutation hook to record one-shot consent for a memory embedding dimension
 * change (#1309). Records intent only — the destructive reshape + re-embed is
 * performed by memory-api on its next restart.
 */

import { useMutation } from "@tanstack/react-query";
import { MemoryApiService } from "@/lib/data/memory-api-service";

const service = new MemoryApiService();

export function useChangeEmbeddingDimension(workspaceName: string) {
  return useMutation({
    mutationFn: (targetDim: number) =>
      service.changeEmbeddingDimension(workspaceName, targetDim),
  });
}
