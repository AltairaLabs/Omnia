"use client";

import { useCallback, useMemo, type ReactNode } from "react";
import { BookOpen } from "lucide-react";
import { useWorkspace } from "@/contexts/workspace-context";
import { useSkillSourceContent } from "@/hooks/use-skill-source-content";
import {
  ContentExplorer,
  type LoadedFile,
} from "@/components/file-browser/content-explorer";
import type { ArenaSourceContentNode } from "@/types/arena";

interface SkillSourceExplorerProps {
  readonly sourceName: string;
}

function skillFileIcon(name: string): ReactNode | undefined {
  if (name === "SKILL.md" || name.toLowerCase() === "skill.md") {
    return <BookOpen className="h-4 w-4 text-purple-500 flex-shrink-0" />;
  }
  return undefined;
}

/** Walk the tree depth-first and return the first SKILL.md path found. */
function findFirstSkillFile(
  tree: ArenaSourceContentNode[]
): string | undefined {
  for (const node of tree) {
    if (!node.isDirectory) {
      if (node.name === "SKILL.md" || node.name.toLowerCase() === "skill.md") {
        return node.path;
      }
      continue;
    }
    if (node.children) {
      const found = findFirstSkillFile(node.children);
      if (found) return found;
    }
  }
  return undefined;
}

/**
 * Read-only file explorer for SkillSource content.
 * Thin wrapper around the shared ContentExplorer; auto-opens the first
 * SKILL.md it finds in the synced tree.
 */
export function SkillSourceExplorer({ sourceName }: SkillSourceExplorerProps) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const { tree, fileCount, loading, error } = useSkillSourceContent(sourceName);

  const loadFile = useCallback(
    async (path: string): Promise<LoadedFile> => {
      if (!workspace) throw new Error("No active workspace");
      const response = await fetch(
        `/api/workspaces/${workspace}/skills/${sourceName}/file?path=${encodeURIComponent(path)}`
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

  const defaultFile = useMemo(() => findFirstSkillFile(tree), [tree]);

  return (
    <ContentExplorer
      tree={tree}
      fileCount={fileCount}
      loading={loading}
      error={error}
      loadFile={loadFile}
      defaultFile={defaultFile}
      getFileIcon={skillFileIcon}
      labels={{
        loading: "Loading skill content...",
        emptyTitle: "No content available",
        emptyDescription:
          "Skill content will appear here once the source has been synced.",
      }}
    />
  );
}
