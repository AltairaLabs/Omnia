"use client";

import { useCallback, useEffect, useState } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  ArenaSourceContentNode,
  ArenaSourceContentResponse,
} from "@/types/arena";

export interface UseSkillSourceContentResult {
  tree: ArenaSourceContentNode[];
  fileCount: number;
  directoryCount: number;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

export function useSkillSourceContent(
  sourceName: string | undefined
): UseSkillSourceContentResult {
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
        `/api/workspaces/${workspace}/skills/${sourceName}/content`
      );
      if (!response.ok) {
        if (response.status === 404) {
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

  return { tree, fileCount, directoryCount, loading, error, refetch: fetchData };
}
