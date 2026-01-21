/**
 * Tests for useArenaConfigs, useArenaConfig, and useArenaConfigMutations hooks.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaConfigs, useArenaConfig, useArenaConfigMutations } from "./use-arena-configs";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockWorkspace = {
  name: "test-workspace",
  displayName: "Test",
  environment: "development" as const,
  namespace: "test-ns",
  role: "editor" as const,
  permissions: { read: true, write: true, delete: true, manageMembers: false },
};

const mockConfig = {
  metadata: { name: "test-config", creationTimestamp: "2026-01-15T10:00:00Z" },
  spec: {
    sourceRef: { name: "test-source" },
    scenarios: { include: ["**/*.yaml"] },
    defaults: { temperature: 0.7 },
  },
  status: { phase: "Ready", scenarioCount: 10 },
};

const mockScenario = {
  name: "test-scenario",
  displayName: "Test Scenario",
  description: "A test scenario",
  tags: ["test"],
  path: "scenarios/test.yaml",
};

const mockJob = {
  metadata: { name: "test-job" },
  spec: { configRef: { name: "test-config" }, type: "evaluation" },
  status: { phase: "Completed" },
};

describe("useArenaConfigs", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty configs when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.configs).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches configs when workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockConfigs = [mockConfig];

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockConfigs),
    });

    const { result } = renderHook(() => useArenaConfigs());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.configs).toEqual(mockConfigs);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/configs");
  });

  it("handles fetch error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      statusText: "Internal Server Error",
    });

    const { result } = renderHook(() => useArenaConfigs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.configs).toEqual([]);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch configs");
  });

  it("refetch function triggers new fetch", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([mockConfig]),
    });

    const { result } = renderHook(() => useArenaConfigs());

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

describe("useArenaConfig", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null config when no workspace or name", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfig(undefined));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toBeNull();
    expect(result.current.scenarios).toEqual([]);
    expect(result.current.linkedJobs).toEqual([]);
  });

  it("fetches config with scenarios and jobs", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([mockScenario]),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([mockJob]),
      });

    const { result } = renderHook(() => useArenaConfig("test-config"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toEqual(mockConfig);
    expect(result.current.scenarios).toEqual([mockScenario]);
    expect(result.current.linkedJobs).toEqual([mockJob]);
    expect(result.current.error).toBeNull();
  });

  it("handles 404 error for config not found", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

    const { result } = renderHook(() => useArenaConfig("nonexistent-config"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toBeNull();
    expect(result.current.error?.message).toBe("Config not found");
  });

  it("handles failed scenarios and jobs requests gracefully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 500,
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

    const { result } = renderHook(() => useArenaConfig("test-config"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toEqual(mockConfig);
    expect(result.current.scenarios).toEqual([]);
    expect(result.current.linkedJobs).toEqual([]);
    expect(result.current.error).toBeNull();
  });
});

describe("useArenaConfigMutations", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("throws error when creating config without workspace", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    await expect(
      result.current.createConfig("test", { sourceRef: { name: "source" } })
    ).rejects.toThrow("No workspace selected");
  });

  it("creates a config successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockConfig),
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    const created = await result.current.createConfig("test-config", {
      sourceRef: { name: "test-source" },
    });

    expect(created).toEqual(mockConfig);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/configs",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          metadata: { name: "test-config" },
          spec: { sourceRef: { name: "test-source" } },
        }),
      }
    );
  });

  it("handles create error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve("Config already exists"),
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    await expect(
      result.current.createConfig("test", { sourceRef: { name: "source" } })
    ).rejects.toThrow("Config already exists");
  });

  it("updates a config successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const updatedConfig = { ...mockConfig, spec: { ...mockConfig.spec, defaults: { temperature: 0.5 } } };

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(updatedConfig),
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    const updated = await result.current.updateConfig("test-config", {
      sourceRef: { name: "test-source" },
      defaults: { temperature: 0.5 },
    });

    expect(updated).toEqual(updatedConfig);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/configs/test-config",
      expect.objectContaining({
        method: "PUT",
      })
    );
  });

  it("deletes a config successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    await result.current.deleteConfig("test-config");

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/configs/test-config",
      { method: "DELETE" }
    );
  });

  it("handles delete error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve("Config in use by jobs"),
    });

    const { result } = renderHook(() => useArenaConfigMutations());

    await expect(result.current.deleteConfig("test-config")).rejects.toThrow(
      "Config in use by jobs"
    );
  });
});
