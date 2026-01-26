"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  ArenaTemplateSource,
  TemplateMetadata,
  TemplateRenderInput,
  TemplateRenderOutput,
  TemplatePreviewResponse,
} from "@/types/arena-template";

const NO_WORKSPACE_ERROR = "No workspace selected";

// =============================================================================
// Template Sources List Hook
// =============================================================================

interface UseTemplateSourcesResult {
  sources: ArenaTemplateSource[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch template sources for the current workspace.
 */
export function useTemplateSources(): UseTemplateSourcesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [sources, setSources] = useState<ArenaTemplateSource[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setSources([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources`
      );
      if (!response.ok) {
        throw new Error(`Failed to fetch template sources: ${response.statusText}`);
      }
      const data = await response.json();
      setSources(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSources([]);
    } finally {
      setLoading(false);
    }
  }, [workspace]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    sources,
    loading,
    error,
    refetch: fetchData,
  };
}

// =============================================================================
// Single Template Source Hook
// =============================================================================

interface UseTemplateSourceResult {
  source: ArenaTemplateSource | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch a single template source.
 */
export function useTemplateSource(name: string | undefined): UseTemplateSourceResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [source, setSource] = useState<ArenaTemplateSource | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !name) {
      setSource(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${name}`
      );

      if (!response.ok) {
        if (response.status === 404) {
          throw new Error("Template source not found");
        }
        throw new Error(`Failed to fetch template source: ${response.statusText}`);
      }

      const sourceData = await response.json();
      setSource(sourceData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSource(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, name]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    source,
    loading,
    error,
    refetch: fetchData,
  };
}

// =============================================================================
// Template Source Mutations Hook
// =============================================================================

interface UseTemplateSourceMutationsResult {
  createSource: (
    name: string,
    spec: ArenaTemplateSource["spec"]
  ) => Promise<ArenaTemplateSource>;
  updateSource: (
    name: string,
    spec: ArenaTemplateSource["spec"]
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
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createSource = useCallback(
    async (
      name: string,
      spec: ArenaTemplateSource["spec"]
    ): Promise<ArenaTemplateSource> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ metadata: { name }, spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create template source");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const updateSource = useCallback(
    async (
      name: string,
      spec: ArenaTemplateSource["spec"]
    ): Promise<ArenaTemplateSource> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update template source");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const deleteSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${name}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete template source");
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const syncSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${name}/sync`,
          { method: "POST" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to sync template source");
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  return {
    createSource,
    updateSource,
    deleteSource,
    syncSource,
    loading,
    error,
  };
}

// =============================================================================
// Templates List Hook
// =============================================================================

interface UseTemplatesResult {
  templates: TemplateMetadata[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch templates from a specific source.
 */
export function useTemplates(sourceName: string | undefined): UseTemplatesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [templates, setTemplates] = useState<TemplateMetadata[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !sourceName) {
      setTemplates([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates`
      );

      if (!response.ok) {
        throw new Error(`Failed to fetch templates: ${response.statusText}`);
      }

      const data = await response.json();
      setTemplates(data.templates || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setTemplates([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, sourceName]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    templates,
    loading,
    error,
    refetch: fetchData,
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

/**
 * Hook to fetch all templates from all sources.
 */
export function useAllTemplates(): UseAllTemplatesResult {
  const { sources, loading: sourcesLoading, error: sourcesError, refetch: refetchSources } =
    useTemplateSources();

  // Compute templates from sources using useMemo
  const templates = useMemo(() => {
    const allTemplates: Array<TemplateMetadata & { sourceName: string }> = [];
    for (const source of sources) {
      if (source.status?.phase === "Ready" && source.status.templates) {
        for (const template of source.status.templates) {
          allTemplates.push({
            ...template,
            sourceName: source.metadata.name,
          });
        }
      }
    }
    return allTemplates;
  }, [sources]);

  return {
    templates,
    loading: sourcesLoading,
    error: sourcesError,
    refetch: refetchSources,
  };
}

// =============================================================================
// Single Template Hook
// =============================================================================

interface UseTemplateResult {
  template: TemplateMetadata | null;
  sourceName: string | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch a single template.
 */
export function useTemplate(
  sourceName: string | undefined,
  templateName: string | undefined
): UseTemplateResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [template, setTemplate] = useState<TemplateMetadata | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !sourceName || !templateName) {
      setTemplate(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates/${templateName}`
      );

      if (!response.ok) {
        if (response.status === 404) {
          throw new Error("Template not found");
        }
        throw new Error(`Failed to fetch template: ${response.statusText}`);
      }

      const data = await response.json();
      setTemplate(data.template || data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setTemplate(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, sourceName, templateName]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    template,
    sourceName: sourceName || null,
    loading,
    error,
    refetch: fetchData,
  };
}

// =============================================================================
// Template Rendering Hook
// =============================================================================

interface UseTemplateRenderingResult {
  preview: (
    sourceName: string,
    templateName: string,
    input: Omit<TemplateRenderInput, "projectName">
  ) => Promise<TemplatePreviewResponse>;
  render: (
    sourceName: string,
    templateName: string,
    input: TemplateRenderInput
  ) => Promise<TemplateRenderOutput>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to handle template preview and rendering.
 */
export function useTemplateRendering(): UseTemplateRenderingResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const preview = useCallback(
    async (
      sourceName: string,
      templateName: string,
      input: Omit<TemplateRenderInput, "projectName">
    ): Promise<TemplatePreviewResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates/${templateName}/preview`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(input),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to preview template");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const render = useCallback(
    async (
      sourceName: string,
      templateName: string,
      input: TemplateRenderInput
    ): Promise<TemplateRenderOutput> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/template-sources/${sourceName}/templates/${templateName}/render`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(input),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to render template");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  return {
    preview,
    render,
    loading,
    error,
  };
}
