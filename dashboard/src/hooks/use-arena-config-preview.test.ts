/**
 * Tests for useArenaConfigPreview hook and estimateWorkItems utility.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  useArenaConfigPreview,
  estimateWorkItems,
  type ArenaConfigPreview,
} from "./use-arena-config-preview";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// =============================================================================
// estimateWorkItems tests (pure function, no React rendering needed)
// =============================================================================

describe("estimateWorkItems", () => {
  const loadedConfig = (
    scenarioCount: number,
    configProviderCount: number
  ): ArenaConfigPreview => ({
    scenarioCount,
    configProviderCount,
    loaded: true,
    loading: false,
    error: null,
  });

  const unloadedConfig: ArenaConfigPreview = {
    scenarioCount: 0,
    configProviderCount: 0,
    loaded: false,
    loading: false,
    error: null,
  };

  describe("when config is not loaded", () => {
    it("returns 1 work item and 1 worker", () => {
      const result = estimateWorkItems(unloadedConfig, 0, 0);

      expect(result.workItems).toBe(1);
      expect(result.recommendedWorkers).toBe(1);
      expect(result.description).toBe("");
    });
  });

  describe("with providers specified", () => {
    it("returns scenarios x providers matrix", () => {
      const result = estimateWorkItems(loadedConfig(3, 2), 4, 0);

      expect(result.workItems).toBe(12); // 3 scenarios x 4 providers
      expect(result.recommendedWorkers).toBe(12);
      expect(result.description).toContain("3 scenarios");
      expect(result.description).toContain("4 providers");
    });

    it("handles 1 scenario and 1 provider", () => {
      const result = estimateWorkItems(loadedConfig(1, 0), 1, 0);

      expect(result.workItems).toBe(1);
      expect(result.description).toContain("1 scenario");
      expect(result.description).toContain("1 provider");
      // Should not have trailing "s"
      expect(result.description).not.toMatch(/1 scenarios/);
      expect(result.description).not.toMatch(/1 providers/);
    });

    it("caps workers at maxWorkerReplicas", () => {
      const result = estimateWorkItems(loadedConfig(10, 0), 5, 3);

      expect(result.workItems).toBe(50);
      expect(result.recommendedWorkers).toBe(3);
    });
  });

  describe("without providers (zero entries)", () => {
    it("returns 1 fallback work item", () => {
      const result = estimateWorkItems(loadedConfig(5, 2), 0, 0);

      expect(result.workItems).toBe(1);
      expect(result.recommendedWorkers).toBe(1);
      expect(result.description).toContain("no providers specified");
    });
  });

  describe("with providers but no scenarios in config", () => {
    it("returns providers count as work items", () => {
      const result = estimateWorkItems(loadedConfig(0, 0), 3, 0);

      expect(result.workItems).toBe(3);
      expect(result.recommendedWorkers).toBe(3);
      expect(result.description).toContain("3 providers");
      expect(result.description).toContain("scenarios enumerated at runtime");
    });

    it("handles 1 provider with no scenarios", () => {
      const result = estimateWorkItems(loadedConfig(0, 0), 1, 0);

      expect(result.workItems).toBe(1);
      expect(result.description).toContain("1 provider");
      expect(result.description).not.toMatch(/1 providers/);
    });
  });

  describe("edge cases", () => {
    it("does not cap when maxWorkerReplicas is 0 (unlimited)", () => {
      const result = estimateWorkItems(loadedConfig(5, 0), 4, 0);

      expect(result.workItems).toBe(20);
      expect(result.recommendedWorkers).toBe(20);
    });

    it("caps workers at maxWorkerReplicas when set", () => {
      const result = estimateWorkItems(loadedConfig(5, 0), 4, 2);

      expect(result.workItems).toBe(20);
      expect(result.recommendedWorkers).toBe(2);
    });
  });
});

// =============================================================================
// useArenaConfigPreview hook tests (requires React rendering)
// =============================================================================

describe("useArenaConfigPreview", () => {
  const mockWorkspaceContext = {
    currentWorkspace: {
      name: "test-workspace",
      displayName: "Test",
      environment: "development" as const,
      namespace: "test-ns",
      role: "owner" as const,
      permissions: { read: true, write: true, delete: true, manageMembers: true },
    },
    setCurrentWorkspace: vi.fn(),
    workspaces: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };

  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns initial state when sourceName is undefined", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    const { result } = renderHook(() =>
      useArenaConfigPreview(undefined, "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.loaded).toBe(false);
    expect(result.current.scenarioCount).toBe(0);
    expect(result.current.error).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns initial state when configPath is undefined", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", undefined)
    );

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.loaded).toBe(false);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns initial state when workspace is null", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      ...mockWorkspaceContext,
      currentWorkspace: null,
    });

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.loaded).toBe(false);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("fetches and parses config successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    const yamlContent = `
spec:
  scenarios:
    - file: scenario1.yaml
    - file: scenario2.yaml
  providers:
    - name: openai
    - name: anthropic
    - file: custom.yaml
`;

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ content: yamlContent }),
    });

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.arena.yaml")
    );

    await waitFor(() => {
      expect(result.current.loaded).toBe(true);
    });

    expect(result.current.scenarioCount).toBe(2);
    expect(result.current.configProviderCount).toBe(3);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("handles 404 response by returning unloaded state", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      statusText: "Not Found",
    });

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "missing.yaml")
    );

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.loaded).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("handles non-404 error response", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.error).toBe(
        "Failed to fetch config: Internal Server Error"
      );
    });

    expect(result.current.loaded).toBe(false);
    expect(result.current.loading).toBe(false);
  });

  it("handles fetch network error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    mockFetch.mockRejectedValueOnce(new Error("Network failure"));

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.error).toBe("Network failure");
    });

    expect(result.current.loaded).toBe(false);
    expect(result.current.loading).toBe(false);
  });

  it("handles non-Error thrown during fetch", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    mockFetch.mockRejectedValueOnce("string error");

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.error).toBe("string error");
    });

    expect(result.current.loaded).toBe(false);
  });

  it("handles config YAML with empty spec", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue(mockWorkspaceContext);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ content: "spec: {}" }),
    });

    const { result } = renderHook(() =>
      useArenaConfigPreview("my-source", "config.yaml")
    );

    await waitFor(() => {
      expect(result.current.loaded).toBe(true);
    });

    expect(result.current.scenarioCount).toBe(0);
    expect(result.current.configProviderCount).toBe(0);
  });
});
