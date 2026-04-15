"use client";

import { useCallback, type ReactNode } from "react";
import { Settings } from "lucide-react";
import { useArenaSourceContent } from "@/hooks/arena";
import { useWorkspace } from "@/contexts/workspace-context";
import {
  ContentExplorer,
  type LoadedFile,
} from "@/components/file-browser/content-explorer";

interface SourceExplorerProps {
  readonly sourceName: string;
  /** Root path to scope the explorer (e.g., "load-testing"). Omit for full source. */
  readonly rootPath?: string;
  /** Default file to open on mount (e.g., "config.arena.yaml"). */
  readonly defaultFile?: string;
}

function arenaFileIcon(name: string): ReactNode | undefined {
  if (name === "config.arena.yaml") {
    return <Settings className="h-4 w-4 text-purple-500 flex-shrink-0" />;
  }
  return undefined;
}

/**
 * Read-only file explorer for ArenaSource content.
 * Thin wrapper around the shared ContentExplorer.
 */
export function SourceExplorer({ sourceName, rootPath, defaultFile }: SourceExplorerProps) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const { tree, fileCount, loading, error } = useArenaSourceContent(sourceName);

  const loadFile = useCallback(
    async (path: string): Promise<LoadedFile> => {
      if (!workspace) throw new Error("No active workspace");
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/sources/${sourceName}/file?path=${encodeURIComponent(path)}`
      );
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(data?.error || `Failed to load file: ${response.statusText}`);
      }
      const data = await response.json();
      return { content: data.content, size: data.size };
    },
    [workspace, sourceName]
  );

  return (
    <ContentExplorer
      tree={tree}
      fileCount={fileCount}
      loading={loading}
      error={error}
      loadFile={loadFile}
      rootPath={rootPath}
      defaultFile={defaultFile}
      getFileIcon={arenaFileIcon}
      labels={{
        loading: "Loading source content...",
        emptyTitle: "No content available",
        emptyDescription:
          "Source content will appear here once it has been synced.",
      }}
    />
  );
}
