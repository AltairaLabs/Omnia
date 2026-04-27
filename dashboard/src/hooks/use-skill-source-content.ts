"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  ArenaSourceContentNode,
  ArenaSourceContentResponse,
} from "@/types/arena";

const EMPTY_CONTENT: ArenaSourceContentResponse = {
  sourceName: "",
  tree: [],
  fileCount: 0,
  directoryCount: 0,
};

export interface UseSkillSourceContentResult {
  tree: ArenaSourceContentNode[];
  fileCount: number;
  directoryCount: number;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

async function fetchSkillSourceContent(
  workspace: string,
  sourceName: string,
): Promise<ArenaSourceContentResponse> {
  const response = await fetch(
    `/api/workspaces/${workspace}/skills/${sourceName}/content`,
  );
  // 404 = source not ready / no content yet — surface as empty, not an error.
  if (response.status === 404) return EMPTY_CONTENT;
  if (!response.ok) {
    throw new Error(`Failed to fetch content: ${response.statusText}`);
  }
  return response.json();
}

export function useSkillSourceContent(
  sourceName: string | undefined,
): UseSkillSourceContentResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: ["skill-source-content", workspace, sourceName],
    queryFn: () => fetchSkillSourceContent(workspace!, sourceName!),
    enabled: !!workspace && !!sourceName,
  });

  const data = query.data ?? EMPTY_CONTENT;

  return {
    tree: data.tree,
    fileCount: data.fileCount,
    directoryCount: data.directoryCount,
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}
