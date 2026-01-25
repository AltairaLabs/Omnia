"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSourceContentNode, ArenaSourceContentResponse } from "@/types/arena";

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

/**
 * Hook to fetch content tree for an ArenaSource.
 * Returns the folder/file structure for browsing and selection.
 *
 * @param sourceName - Name of the ArenaSource to fetch content for
 */
export function useArenaSourceContent(sourceName: string | undefined): UseArenaSourceContentResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [tree, setTree] = useState<ArenaSourceContentNode[]>([]);
  const [fileCount, setFileCount] = useState(0);
  const [directoryCount, setDirectoryCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !sourceName) {
      setTree([]);
      setFileCount(0);
      setDirectoryCount(0);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/sources/${sourceName}/content`
      );

      if (!response.ok) {
        if (response.status === 404) {
          // Source not ready or no content - not an error, just empty
          setTree([]);
          setFileCount(0);
          setDirectoryCount(0);
          setLoading(false);
          return;
        }
        throw new Error(`Failed to fetch content: ${response.statusText}`);
      }

      const data: ArenaSourceContentResponse = await response.json();
      setTree(data.tree);
      setFileCount(data.fileCount);
      setDirectoryCount(data.directoryCount);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setTree([]);
      setFileCount(0);
      setDirectoryCount(0);
    } finally {
      setLoading(false);
    }
  }, [workspace, sourceName]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    tree,
    fileCount,
    directoryCount,
    loading,
    error,
    refetch: fetchData,
  };
}
