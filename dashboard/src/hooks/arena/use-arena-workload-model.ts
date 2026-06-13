"use client";

import { useEffect, useMemo, useState } from "react";
import { useProjectEditorStore } from "@/stores";
import { useArenaProjectFiles } from "@/hooks/arena";
import {
  parseArenaProject,
  referencedFiles,
  arenaProjectToWorkload,
  type WorkloadModel,
} from "@/components/workload-graph";

interface TreeNode {
  path: string;
  isDirectory: boolean;
  children?: TreeNode[];
}
interface OpenFileLike {
  path: string;
  content: string;
}

function collectPaths(nodes: TreeNode[]): string[] {
  const out: string[] = [];
  const walk = (n: TreeNode) => {
    if (!n.isDirectory) out.push(n.path);
    for (const c of n.children ?? []) walk(c);
  };
  for (const n of nodes) walk(n);
  return out;
}

function findConfigPath(paths: string[]): string | undefined {
  return paths.find((p) => /\.arena\.ya?ml$/i.test(p));
}

export interface ArenaWorkloadModelState {
  model: WorkloadModel | null;
  loading: boolean;
  parseError: string | null;
}

export function useArenaWorkloadModel(projectId: string | undefined): ArenaWorkloadModelState {
  const openFiles = useProjectEditorStore((s) => (s as { openFiles: OpenFileLike[] }).openFiles);
  const fileTree = useProjectEditorStore((s) => (s as { fileTree: TreeNode[] }).fileTree);
  const { getFileContent } = useArenaProjectFiles();

  const [fetched, setFetched] = useState<Record<string, string>>({});

  const openMap = useMemo(() => {
    const m: Record<string, string> = {};
    for (const f of openFiles) m[f.path] = f.content;
    return m;
  }, [openFiles]);

  const configPath = useMemo(() => findConfigPath(collectPaths(fileTree)), [fileTree]);
  const configContent = configPath ? (openMap[configPath] ?? fetched[configPath]) : undefined;

  // Fetch the config itself if it isn't open and hasn't been fetched yet.
  useEffect(() => {
    if (!projectId || !configPath || openMap[configPath] || fetched[configPath]) return;
    let cancelled = false;
    getFileContent(projectId, configPath)
      .then((r) => {
        if (!cancelled) setFetched((p) => ({ ...p, [configPath]: r.content }));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [projectId, configPath, openMap, fetched, getFileContent]);

  // Fetch any referenced files that aren't open and aren't cached.
  useEffect(() => {
    if (!projectId || !configPath || !configContent) return;
    const refs = referencedFiles(configPath, configContent);
    const missing = refs.filter((p) => !openMap[p] && !(p in fetched));
    if (missing.length === 0) return;
    let cancelled = false;
    Promise.all(
      missing.map((p) =>
        getFileContent(projectId, p)
          .then((r) => [p, r.content] as const)
          .catch(() => [p, ""] as const),
      ),
    ).then((pairs) => {
      if (cancelled) return;
      setFetched((prev) => {
        const next = { ...prev };
        for (const [p, c] of pairs) next[p] = c;
        return next;
      });
    });
    return () => {
      cancelled = true;
    };
  }, [projectId, configPath, configContent, openMap, fetched, getFileContent]);

  // Pure derivation of the freshly-parsed model from the current inputs.
  const derived = useMemo<{ model: WorkloadModel | null; loading: boolean; parseError: string | null }>(() => {
    if (!configPath || configContent == null) {
      return { model: null, loading: true, parseError: null };
    }
    const readFile = (p: string): string | undefined => openMap[p] ?? fetched[p];
    const { parsed, error } = parseArenaProject({ configPath, configContent, readFile });
    if (!parsed) return { model: null, loading: false, parseError: error };
    return { model: arenaProjectToWorkload(parsed), loading: false, parseError: null };
  }, [configPath, configContent, openMap, fetched]);

  // Sticky last-good model: when the config is mid-edit (parse error) we keep
  // showing the previous valid graph rather than blanking the canvas. This is
  // React's "adjust state during render" pattern — no effect, no ref.
  const [sticky, setSticky] = useState<WorkloadModel | null>(null);
  if (derived.model && derived.model !== sticky) {
    setSticky(derived.model);
  }

  return {
    model: derived.model ?? sticky,
    loading: derived.loading,
    parseError: derived.parseError,
  };
}
