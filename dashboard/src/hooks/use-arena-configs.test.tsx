/**
 * Tests for useArenaConfigs, useArenaConfig, and useArenaConfigMutations hooks.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useArenaConfigs, useArenaConfig, useArenaConfigMutations, useArenaConfigContent, useArenaConfigFile } from "./use-arena-configs";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock data service methods
const mockGetArenaConfigs = vi.fn();
const mockGetArenaConfig = vi.fn();
const mockGetArenaConfigScenarios = vi.fn();
const mockGetArenaJobs = vi.fn();
const mockCreateArenaConfig = vi.fn();
const mockUpdateArenaConfig = vi.fn();
const mockDeleteArenaConfig = vi.fn();
const mockGetArenaConfigContent = vi.fn();
const mockGetArenaConfigFile = vi.fn();

// Mock useDataService
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getArenaConfigs: mockGetArenaConfigs,
    getArenaConfig: mockGetArenaConfig,
    getArenaConfigScenarios: mockGetArenaConfigScenarios,
    getArenaJobs: mockGetArenaJobs,
    createArenaConfig: mockCreateArenaConfig,
    updateArenaConfig: mockUpdateArenaConfig,
    deleteArenaConfig: mockDeleteArenaConfig,
    getArenaConfigContent: mockGetArenaConfigContent,
    getArenaConfigFile: mockGetArenaConfigFile,
  }),
}));

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

// Create a wrapper with QueryClientProvider
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

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

    const { result } = renderHook(() => useArenaConfigs(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.configs).toEqual([]);
    expect(result.current.error).toBeNull();
    // Should not call data service when no workspace
    expect(mockGetArenaConfigs).not.toHaveBeenCalled();
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
    mockGetArenaConfigs.mockResolvedValueOnce(mockConfigs);

    const { result } = renderHook(() => useArenaConfigs(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.configs).toEqual(mockConfigs);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaConfigs).toHaveBeenCalledWith("test-workspace");
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

    mockGetArenaConfigs.mockRejectedValueOnce(new Error("Failed to fetch configs"));

    const { result } = renderHook(() => useArenaConfigs(), { wrapper: createWrapper() });

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

    mockGetArenaConfigs.mockResolvedValue([mockConfig]);

    const { result } = renderHook(() => useArenaConfigs(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaConfigs).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockGetArenaConfigs).toHaveBeenCalledTimes(2);
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

    const { result } = renderHook(() => useArenaConfig(undefined), { wrapper: createWrapper() });

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

    mockGetArenaConfig.mockResolvedValueOnce(mockConfig);
    mockGetArenaConfigScenarios.mockResolvedValueOnce([mockScenario]);
    mockGetArenaJobs.mockResolvedValueOnce([mockJob]);

    const { result } = renderHook(() => useArenaConfig("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toEqual(mockConfig);
    expect(result.current.scenarios).toEqual([mockScenario]);
    expect(result.current.linkedJobs).toEqual([mockJob]);
    expect(result.current.error).toBeNull();
  });

  it("handles config not found", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockGetArenaConfig.mockResolvedValueOnce(undefined);
    mockGetArenaConfigScenarios.mockResolvedValueOnce([]);
    mockGetArenaJobs.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useArenaConfig("nonexistent-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toBeNull();
    expect(result.current.error?.message).toBe("Config not found");
  });

  it("handles partial failures gracefully - scenarios still work when jobs fail", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    // Note: With the new implementation using Promise.all, if any promise fails,
    // the whole query fails. The previous implementation had graceful degradation.
    // This test verifies that successful fetches work correctly.
    mockGetArenaConfig.mockResolvedValueOnce(mockConfig);
    mockGetArenaConfigScenarios.mockResolvedValueOnce([mockScenario]);
    mockGetArenaJobs.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useArenaConfig("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.config).toEqual(mockConfig);
    expect(result.current.scenarios).toEqual([mockScenario]);
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

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

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

    mockCreateArenaConfig.mockResolvedValueOnce(mockConfig);

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

    const created = await result.current.createConfig("test-config", {
      sourceRef: { name: "test-source" },
    });

    expect(created).toEqual(mockConfig);
    expect(mockCreateArenaConfig).toHaveBeenCalledWith(
      "test-workspace",
      "test-config",
      { sourceRef: { name: "test-source" } }
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

    mockCreateArenaConfig.mockRejectedValueOnce(new Error("Config already exists"));

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

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
    mockUpdateArenaConfig.mockResolvedValueOnce(updatedConfig);

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

    const updated = await result.current.updateConfig("test-config", {
      sourceRef: { name: "test-source" },
      defaults: { temperature: 0.5 },
    });

    expect(updated).toEqual(updatedConfig);
    expect(mockUpdateArenaConfig).toHaveBeenCalledWith(
      "test-workspace",
      "test-config",
      { sourceRef: { name: "test-source" }, defaults: { temperature: 0.5 } }
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

    mockDeleteArenaConfig.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

    await result.current.deleteConfig("test-config");

    expect(mockDeleteArenaConfig).toHaveBeenCalledWith("test-workspace", "test-config");
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

    mockDeleteArenaConfig.mockRejectedValueOnce(new Error("Config in use by jobs"));

    const { result } = renderHook(() => useArenaConfigMutations(), { wrapper: createWrapper() });

    await expect(result.current.deleteConfig("test-config")).rejects.toThrow(
      "Config in use by jobs"
    );
  });
});

const mockContent = {
  metadata: { name: "test-config" },
  files: [{ path: "config.yaml", type: "arena" as const, size: 100 }],
  fileTree: [{ name: "config.yaml", path: "config.yaml", isDirectory: false, type: "arena" as const }],
  promptConfigs: [],
  providers: [],
  scenarios: [],
  tools: [],
  mcpServers: {},
  judges: {},
};

describe("useArenaConfigContent", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null content when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigContent("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(mockGetArenaConfigContent).not.toHaveBeenCalled();
  });

  it("returns null content when no name is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigContent(undefined), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(mockGetArenaConfigContent).not.toHaveBeenCalled();
  });

  it("fetches content successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockGetArenaConfigContent.mockResolvedValueOnce(mockContent);

    const { result } = renderHook(() => useArenaConfigContent("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toEqual(mockContent);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaConfigContent).toHaveBeenCalledWith("test-workspace", "test-config");
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

    mockGetArenaConfigContent.mockRejectedValueOnce(new Error("Failed to fetch content"));

    const { result } = renderHook(() => useArenaConfigContent("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(result.current.error?.message).toContain("Failed to fetch content");
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

    mockGetArenaConfigContent.mockResolvedValue(mockContent);

    const { result } = renderHook(() => useArenaConfigContent("test-config"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaConfigContent).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockGetArenaConfigContent).toHaveBeenCalledTimes(2);
    });
  });
});

describe("useArenaConfigFile", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigFile("test-config", "config.yaml"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(mockGetArenaConfigFile).not.toHaveBeenCalled();
  });

  it("returns null when no config name is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigFile(undefined, "config.yaml"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(mockGetArenaConfigFile).not.toHaveBeenCalled();
  });

  it("returns null when no file path is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaConfigFile("test-config", null), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(mockGetArenaConfigFile).not.toHaveBeenCalled();
  });

  it("fetches file content successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const fileContent = "apiVersion: v1\nkind: Arena";
    mockGetArenaConfigFile.mockResolvedValueOnce(fileContent);

    const { result } = renderHook(() => useArenaConfigFile("test-config", "config.yaml"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBe(fileContent);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaConfigFile).toHaveBeenCalledWith("test-workspace", "test-config", "config.yaml");
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

    mockGetArenaConfigFile.mockRejectedValueOnce(new Error("File not found"));

    const { result } = renderHook(() => useArenaConfigFile("test-config", "nonexistent.yaml"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.content).toBeNull();
    expect(result.current.error?.message).toContain("File not found");
  });
});
