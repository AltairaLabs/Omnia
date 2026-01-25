"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaVersion, ArenaVersionsResponse } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaSourceVersionsResult {
  /** List of available versions */
  versions: ArenaVersion[];
  /** Current HEAD version hash */
  headVersion: string | null;
  /** Whether versions are being loaded */
  loading: boolean;
  /** Error if the fetch failed */
  error: Error | null;
  /** Refetch versions */
  refetch: () => void;
}

interface UseArenaSourceVersionMutationsResult {
  /** Switch to a different version */
  switchVersion: (versionHash: string) => Promise<void>;
  /** Whether a version switch is in progress */
  switching: boolean;
  /** Error if the switch failed */
  error: Error | null;
}

/**
 * Hook to fetch versions for an ArenaSource.
 * Returns the list of available versions and the current HEAD version.
 *
 * @param sourceName - Name of the ArenaSource to fetch versions for
 */
export function useArenaSourceVersions(sourceName: string | undefined): UseArenaSourceVersionsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [versions, setVersions] = useState<ArenaVersion[]>([]);
  const [headVersion, setHeadVersion] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !sourceName) {
      setVersions([]);
      setHeadVersion(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/sources/${sourceName}/versions`
      );

      if (!response.ok) {
        if (response.status === 404) {
          // Source not ready or no versions - not an error, just empty
          setVersions([]);
          setHeadVersion(null);
          setLoading(false);
          return;
        }
        throw new Error(`Failed to fetch versions: ${response.statusText}`);
      }

      const data: ArenaVersionsResponse = await response.json();
      setVersions(data.versions);
      setHeadVersion(data.head);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setVersions([]);
      setHeadVersion(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, sourceName]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    versions,
    headVersion,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to provide version switch mutations for an ArenaSource.
 *
 * @param sourceName - Name of the ArenaSource to switch versions for
 * @param onSuccess - Callback to execute after successful version switch
 */
export function useArenaSourceVersionMutations(
  sourceName: string | undefined,
  onSuccess?: () => void
): UseArenaSourceVersionMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [switching, setSwitching] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const switchVersion = useCallback(
    async (versionHash: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      if (!sourceName) {
        throw new Error("No source name provided");
      }

      setSwitching(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/sources/${sourceName}/versions`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ version: versionHash }),
          }
        );

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new Error(data.error || `Failed to switch version: ${response.statusText}`);
        }

        // Call onSuccess callback if provided
        onSuccess?.();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setSwitching(false);
      }
    },
    [workspace, sourceName, onSuccess]
  );

  return {
    switchVersion,
    switching,
    error,
  };
}
