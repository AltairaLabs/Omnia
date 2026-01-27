/**
 * Tests for use-template-sources hooks
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { ArenaTemplateSource, TemplateMetadata } from "@/types/arena-template";

// Mock dependencies
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

import { useWorkspace } from "@/contexts/workspace-context";
import {
  useTemplateSources,
  useTemplateSource,
  useTemplateSourceMutations,
  useTemplates,
  useAllTemplates,
  useTemplate,
  useTemplateRendering,
} from "./use-template-sources";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Test data factories
function createMockSource(overrides: Partial<ArenaTemplateSource> = {}): ArenaTemplateSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaTemplateSource",
    metadata: { name: "test-source", namespace: "test-ns" },
    spec: { type: "git", git: { url: "https://github.com/test/repo" } },
    status: {
      phase: "Ready",
      templateCount: 2,
    },
    ...overrides,
  };
}

function createMockTemplate(overrides: Partial<TemplateMetadata> = {}): TemplateMetadata {
  return {
    name: "test-template",
    displayName: "Test Template",
    description: "A test template",
    category: "chatbot",
    tags: ["test"],
    path: "templates/test",
    ...overrides,
  };
}

const mockWorkspace = {
  name: "test-workspace",
  displayName: "Test Workspace",
  environment: "development" as const,
  namespace: "test-ns",
  role: "owner" as const,
  permissions: { read: true, write: true, delete: true, manageMembers: true },
};

describe("use-template-sources hooks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      workspaces: [],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  // ==========================================================================
  // useTemplateSources
  // ==========================================================================
  describe("useTemplateSources", () => {
    it("returns empty array when no workspace selected", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateSources());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });
      expect(result.current.sources).toEqual([]);
      expect(result.current.error).toBeNull();
    });

    it("fetches sources on mount", async () => {
      const sources = [createMockSource()];
      // The API returns the array directly, not wrapped in { sources }
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(sources),
      });

      const { result } = renderHook(() => useTemplateSources());

      expect(result.current.loading).toBe(true);

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.sources).toEqual(sources);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources"
      );
    });

    it("handles fetch error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const { result } = renderHook(() => useTemplateSources());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error).not.toBeNull();
      expect(result.current.sources).toEqual([]);
    });

    it("handles network error", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useTemplateSources());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.error?.message).toBe("Network error");
    });

    it("refetches when refetch is called", async () => {
      const sources = [createMockSource()];
      // The API returns the array directly
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(sources),
      });

      const { result } = renderHook(() => useTemplateSources());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(mockFetch).toHaveBeenCalledTimes(1);

      act(() => {
        result.current.refetch();
      });

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledTimes(2);
      });
    });
  });

  // ==========================================================================
  // useTemplateSource
  // ==========================================================================
  describe("useTemplateSource", () => {
    it("returns null when id is not provided", async () => {
      const { result } = renderHook(() => useTemplateSource(undefined));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });
      expect(result.current.source).toBeNull();
    });

    it("fetches single source by id", async () => {
      const source = createMockSource();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(source),
      });

      const { result } = renderHook(() => useTemplateSource("test-source"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.source).toEqual(source);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/test-source"
      );
    });

    it("handles 404 error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      const { result } = renderHook(() => useTemplateSource("nonexistent"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.source).toBeNull();
      expect(result.current.error?.message).toBe("Template source not found");
    });
  });

  // ==========================================================================
  // useTemplateSourceMutations
  // ==========================================================================
  describe("useTemplateSourceMutations", () => {
    it("creates a new source", async () => {
      const newSource = createMockSource({ metadata: { name: "new-source", namespace: "ns" } });
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(newSource),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      const created = await result.current.createSource("new-source", {
        type: "git",
        git: { url: "https://github.com/test/new" },
      });

      expect(created).toEqual(newSource);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources",
        expect.objectContaining({
          method: "POST",
          headers: { "Content-Type": "application/json" },
        })
      );
    });

    it("updates an existing source", async () => {
      const updatedSource = createMockSource();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(updatedSource),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      const updated = await result.current.updateSource("test-source", {
        type: "git",
        syncInterval: "2h",
      });

      expect(updated).toEqual(updatedSource);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/test-source",
        expect.objectContaining({
          method: "PUT",
        })
      );
    });

    it("deletes a source", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await result.current.deleteSource("test-source");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/test-source",
        expect.objectContaining({
          method: "DELETE",
        })
      );
    });

    it("throws on create error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        text: () => Promise.resolve("Invalid spec"),
        json: () => Promise.resolve({ error: "Invalid spec" }),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(
        result.current.createSource("test-source", { type: "git" })
      ).rejects.toThrow("Invalid spec");
    });

    it("throws when no workspace selected", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(
        result.current.createSource("test-source", { type: "git" })
      ).rejects.toThrow("No workspace selected");
    });

    it("syncs a source", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await result.current.syncSource("test-source");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/test-source/sync",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("throws on sync error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        text: () => Promise.resolve("Sync failed"),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(result.current.syncSource("test-source")).rejects.toThrow("Sync failed");
    });

    it("throws on update error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        text: () => Promise.resolve("Update failed"),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(
        result.current.updateSource("test-source", { type: "git" })
      ).rejects.toThrow("Update failed");
    });

    it("throws on delete error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        text: () => Promise.resolve("Delete failed"),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(result.current.deleteSource("test-source")).rejects.toThrow("Delete failed");
    });

    it("throws when no workspace selected for update", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(
        result.current.updateSource("test-source", { type: "git" })
      ).rejects.toThrow("No workspace selected");
    });

    it("throws when no workspace selected for delete", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(result.current.deleteSource("test-source")).rejects.toThrow("No workspace selected");
    });

    it("throws when no workspace selected for sync", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateSourceMutations());

      await expect(result.current.syncSource("test-source")).rejects.toThrow("No workspace selected");
    });
  });

  // ==========================================================================
  // useTemplates
  // ==========================================================================
  describe("useTemplates", () => {
    it("returns empty array when source id not provided", async () => {
      const { result } = renderHook(() => useTemplates(undefined));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });
      expect(result.current.templates).toEqual([]);
    });

    it("fetches templates from a source", async () => {
      const templates = [createMockTemplate()];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ templates }),
      });

      const { result } = renderHook(() => useTemplates("test-source"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.templates).toEqual(templates);
      expect(result.current.error).toBeNull();
    });
  });

  // ==========================================================================
  // useAllTemplates
  // ==========================================================================
  describe("useAllTemplates", () => {
    it("aggregates templates from all ready sources", async () => {
      const sources = [
        createMockSource({
          metadata: { name: "source-1", namespace: "ns" },
          status: { phase: "Ready", templateCount: 1 },
        }),
        createMockSource({
          metadata: { name: "source-2", namespace: "ns" },
          status: { phase: "Ready", templateCount: 1 },
        }),
      ];

      // Mock sources fetch
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(sources),
      });
      // Mock templates fetch for source-1
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ templates: [createMockTemplate({ name: "t1" })] }),
      });
      // Mock templates fetch for source-2
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ templates: [createMockTemplate({ name: "t2" })] }),
      });

      const { result } = renderHook(() => useAllTemplates());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.templates).toHaveLength(2);
      // Templates are fetched in parallel so order may vary, just check both exist
      const templateNames = result.current.templates.map(t => t.name);
      expect(templateNames).toContain("t1");
      expect(templateNames).toContain("t2");
    });

    it("skips templates from non-ready sources", async () => {
      const sources = [
        createMockSource({
          metadata: { name: "ready-source", namespace: "ns" },
          status: { phase: "Ready", templateCount: 1 },
        }),
        createMockSource({
          metadata: { name: "error-source", namespace: "ns" },
          status: { phase: "Error" },
        }),
      ];

      // Mock sources fetch
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(sources),
      });
      // Mock templates fetch for ready-source only (error-source won't be fetched)
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ templates: [createMockTemplate({ name: "t1" })] }),
      });

      const { result } = renderHook(() => useAllTemplates());

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.templates).toHaveLength(1);
      expect(result.current.templates[0].name).toBe("t1");
    });
  });

  // ==========================================================================
  // useTemplate
  // ==========================================================================
  describe("useTemplate", () => {
    it("returns null when source or template id not provided", async () => {
      const { result } = renderHook(() => useTemplate(undefined, undefined));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });
      expect(result.current.template).toBeNull();
    });

    it("fetches single template details", async () => {
      const template = createMockTemplate();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ template, sourceName: "test-source", sourcePhase: "Ready" }),
      });

      const { result } = renderHook(() => useTemplate("test-source", "test-template"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.template).toEqual(template);
      expect(result.current.sourceName).toBe("test-source");
    });

    it("handles 404 error for template", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      const { result } = renderHook(() => useTemplate("test-source", "nonexistent"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.template).toBeNull();
      expect(result.current.error?.message).toBe("Template not found");
    });

    it("handles generic fetch error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const { result } = renderHook(() => useTemplate("test-source", "test-template"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.template).toBeNull();
      expect(result.current.error).not.toBeNull();
    });
  });

  // ==========================================================================
  // useTemplateRendering
  // ==========================================================================
  describe("useTemplateRendering", () => {
    it("previews template rendering", async () => {
      const previewResult = {
        files: [{ path: "config.yaml", content: "name: test" }],
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(previewResult),
      });

      const { result } = renderHook(() => useTemplateRendering());

      const preview = await result.current.preview("source-1", "template-1", {
        variables: { name: "test" },
      });

      expect(preview.files).toHaveLength(1);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/source-1/templates/template-1/preview",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("renders template and creates project", async () => {
      const renderResult = {
        projectId: "new-project-123",
        files: ["config.yaml"],
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(renderResult),
      });

      const { result } = renderHook(() => useTemplateRendering());

      const rendered = await result.current.render("source-1", "template-1", {
        variables: { name: "test" },
        projectName: "my-project",
      });

      expect(rendered.projectId).toBe("new-project-123");
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/template-sources/source-1/templates/template-1/render",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("throws on preview error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        text: () => Promise.resolve("Invalid variables"),
        json: () => Promise.resolve({ error: "Invalid variables" }),
      });

      const { result } = renderHook(() => useTemplateRendering());

      await expect(
        result.current.preview("source-1", "template-1", { variables: {} })
      ).rejects.toThrow("Invalid variables");
    });

    it("throws when no workspace selected", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateRendering());

      await expect(
        result.current.preview("source-1", "template-1", { variables: {} })
      ).rejects.toThrow("No workspace selected");
    });

    it("throws on render error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
        text: () => Promise.resolve("Render failed"),
      });

      const { result } = renderHook(() => useTemplateRendering());

      await expect(
        result.current.render("source-1", "template-1", {
          variables: {},
          projectName: "my-project",
        })
      ).rejects.toThrow("Render failed");
    });

    it("throws when no workspace selected for render", async () => {
      vi.mocked(useWorkspace).mockReturnValue({
        currentWorkspace: null,
        workspaces: [],
        setCurrentWorkspace: vi.fn(),
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });

      const { result } = renderHook(() => useTemplateRendering());

      await expect(
        result.current.render("source-1", "template-1", {
          variables: {},
          projectName: "my-project",
        })
      ).rejects.toThrow("No workspace selected");
    });
  });
});
