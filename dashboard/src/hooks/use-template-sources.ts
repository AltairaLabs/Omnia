"use client";

import { useCallback, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  ArenaTemplateSource,
  TemplateMetadata,
  TemplateRenderInput,
  TemplateRenderOutput,
  TemplatePreviewResponse,
} from "@/types/arena-template";

const NO_WORKSPACE_ERROR = "No workspace selected";

function templateSourcesKey(workspace: string | undefined) {
  return ["template-sources", workspace] as const;
}

function allTemplatesKey(
  workspace: string | undefined,
  readySourceNames: string[],
) {
  return ["all-templates", workspace, ...readySourceNames] as const;
}

// =============================================================================
// Template Sources List Hook
// =============================================================================

interface UseTemplateSourcesResult {
  sources: ArenaTemplateSource[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

async function fetchTemplateSources(
  workspace: string,
): Promise<ArenaTemplateSource[]> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/template-sources`,
  );
  if (!response.ok) {
    throw new Error(`Failed to fetch template sources: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch template sources for the current workspace.
 */
export function useTemplateSources(): UseTemplateSourcesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: templateSourcesKey(workspace),
    queryFn: () => fetchTemplateSources(workspace!),
    enabled: !!workspace,
  });

  return {
    sources: query.data ?? [],
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Template Source Mutations Hook
// =============================================================================

interface UseTemplateSourceMutationsResult {
  createSource: (
    name: string,
    spec: ArenaTemplateSource["spec"],
  ) => Promise<ArenaTemplateSource>;
  updateSource: (
    name: string,
    spec: ArenaTemplateSource["spec"],
  ) => Promise<ArenaTemplateSource>;
  deleteSource: (name: string) => Promise<void>;
  syncSource: (name: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to provide mutations for template sources.
 */
export function useTemplateSourceMutations(): UseTemplateSourceMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const invalidateSources = () =>
    queryClient.invalidateQueries({ queryKey: templateSourcesKey(workspace) });

  const createMutation = useMutation({
    mutationFn: async (
      args: { name: string; spec: ArenaTemplateSource["spec"] },
    ): Promise<ArenaTemplateSource> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name: args.name }, spec: args.spec }),
        },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to create template source");
      }
      return response.json();
    },
    onSuccess: () => {
      invalidateSources();
    },
  });

  const updateMutation = useMutation({
    mutationFn: async (
      args: { name: string; spec: ArenaTemplateSource["spec"] },
    ): Promise<ArenaTemplateSource> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${args.name}`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ spec: args.spec }),
        },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to update template source");
      }
      return response.json();
    },
    onSuccess: () => {
      invalidateSources();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (name: string): Promise<void> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${name}`,
        { method: "DELETE" },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to delete template source");
      }
    },
    onSuccess: () => {
      invalidateSources();
    },
  });

  const syncMutation = useMutation({
    mutationFn: async (name: string): Promise<void> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${name}/sync`,
        { method: "POST" },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to sync template source");
      }
    },
    onSuccess: () => {
      invalidateSources();
    },
  });

  return {
    createSource: (name, spec) => createMutation.mutateAsync({ name, spec }),
    updateSource: (name, spec) => updateMutation.mutateAsync({ name, spec }),
    deleteSource: (name) => deleteMutation.mutateAsync(name),
    syncSource: (name) => syncMutation.mutateAsync(name),
    loading:
      createMutation.isPending ||
      updateMutation.isPending ||
      deleteMutation.isPending ||
      syncMutation.isPending,
    error:
      (createMutation.error as Error | null) ??
      (updateMutation.error as Error | null) ??
      (deleteMutation.error as Error | null) ??
      (syncMutation.error as Error | null) ??
      null,
  };
}

// =============================================================================
// All Templates Hook (from all sources)
// =============================================================================

interface UseAllTemplatesResult {
  templates: Array<TemplateMetadata & { sourceName: string }>;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

async function fetchTemplatesFromSource(
  workspace: string,
  sourceName: string,
): Promise<{ sourceName: string; templates: TemplateMetadata[] }> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates`,
  );
  if (!response.ok) {
    console.warn(
      `Failed to fetch templates from ${sourceName}: ${response.statusText}`,
    );
    return { sourceName, templates: [] };
  }
  const data = await response.json();
  return { sourceName, templates: data.templates || [] };
}

async function fetchAllTemplates(
  workspace: string,
  readySourceNames: string[],
): Promise<Array<TemplateMetadata & { sourceName: string }>> {
  const results = await Promise.allSettled(
    readySourceNames.map((name) => fetchTemplatesFromSource(workspace, name)),
  );
  const out: Array<TemplateMetadata & { sourceName: string }> = [];
  for (const result of results) {
    if (result.status === "fulfilled") {
      for (const template of result.value.templates) {
        out.push({ ...template, sourceName: result.value.sourceName });
      }
    }
  }
  return out;
}

/**
 * Hook to fetch all templates from all sources.
 * Templates are fetched from the API endpoint for each ready source.
 */
export function useAllTemplates(): UseAllTemplatesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const {
    sources,
    loading: sourcesLoading,
    error: sourcesError,
  } = useTemplateSources();

  const readySourceNames = sources
    .filter((s) => s.status?.phase === "Ready")
    .map((s) => s.metadata.name);

  const query = useQuery({
    queryKey: allTemplatesKey(workspace, readySourceNames),
    queryFn: () => fetchAllTemplates(workspace!, readySourceNames),
    enabled: !!workspace && !sourcesLoading,
  });

  return {
    templates: query.data ?? [],
    loading: sourcesLoading || (query.isLoading && !!workspace),
    error: sourcesError || ((query.error as Error | null) ?? null),
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Template Rendering Hook
// =============================================================================

interface UseTemplateRenderingResult {
  preview: (
    sourceName: string,
    templateName: string,
    input: Omit<TemplateRenderInput, "projectName">,
  ) => Promise<TemplatePreviewResponse>;
  render: (
    sourceName: string,
    templateName: string,
    input: TemplateRenderInput,
  ) => Promise<TemplateRenderOutput>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to handle template preview and rendering.
 *
 * Both operations are user-triggered (called from event handlers), so they
 * stay imperative and share a single loading/error state.
 */
export function useTemplateRendering(): UseTemplateRenderingResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const wrap = useCallback(async function <T>(fn: () => Promise<T>): Promise<T> {
    setLoading(true);
    setError(null);
    try {
      return await fn();
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      throw e;
    } finally {
      setLoading(false);
    }
  }, []);

  const preview = useCallback(
    (
      sourceName: string,
      templateName: string,
      input: Omit<TemplateRenderInput, "projectName">,
    ): Promise<TemplatePreviewResponse> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates/${templateName}/preview`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(input),
          },
        );
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to preview template");
        }
        return response.json();
      }),
    [workspace, wrap],
  );

  const render = useCallback(
    (
      sourceName: string,
      templateName: string,
      input: TemplateRenderInput,
    ): Promise<TemplateRenderOutput> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates/${templateName}/render`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(input),
          },
        );
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to render template");
        }
        return response.json();
      }),
    [workspace, wrap],
  );

  return {
    preview,
    render,
    loading,
    error,
  };
}
