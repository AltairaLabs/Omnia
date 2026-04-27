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

interface UseArenaSourceContentResult {
  /** Content tree structure */
  tree: ArenaSourceContentNode[];
  /** Total number of files */
  fileCount: number;
  /** Total number of directories */
  directoryCount: number;
  /** Whether content is being loaded */
  loading: boolean;
  /** Error if the fetch failed */
  error: Error | null;
  /** Refetch content */
  refetch: () => void;
}

async function fetchArenaSourceContent(
  workspace: string,
  sourceName: string,
): Promise<ArenaSourceContentResponse> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/sources/${sourceName}/content`,
  );
  // 404 = source not ready / no content yet — surface as empty, not an error.
  if (response.status === 404) return EMPTY_CONTENT;
  if (!response.ok) {
    throw new Error(`Failed to fetch content: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch content tree for an ArenaSource.
 * Returns the folder/file structure for browsing and selection.
 *
 * @param sourceName - Name of the ArenaSource to fetch content for
 */
export function useArenaSourceContent(
  sourceName: string | undefined,
): UseArenaSourceContentResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: ["arena-source-content", workspace, sourceName],
    queryFn: () => fetchArenaSourceContent(workspace!, sourceName!),
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
