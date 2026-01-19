import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useToolRegistries, useToolRegistry } from "./use-tool-registries";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock tool registry data
const mockToolRegistries = [
  {
    metadata: {
      name: "github-tools",
      namespace: "omnia-system",
      uid: "uid-1",
    },
    spec: {
      handlers: [],
    },
    status: {
      phase: "Ready" as const,
    },
  },
  {
    metadata: {
      name: "slack-tools",
      namespace: "omnia-system",
      uid: "uid-2",
    },
    spec: {
      handlers: [],
    },
    status: {
      phase: "Degraded" as const,
    },
  },
  {
    metadata: {
      name: "jira-tools",
      namespace: "omnia-system",
      uid: "uid-3",
    },
    spec: {
      handlers: [],
    },
    status: {
      phase: "Ready" as const,
    },
  },
];

// Mock useDataService
const mockGetToolRegistries = vi.fn().mockResolvedValue(mockToolRegistries);
const mockGetToolRegistry = vi.fn().mockResolvedValue(mockToolRegistries[0]);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    isDemo: true,
    getToolRegistries: mockGetToolRegistries,
    getToolRegistry: mockGetToolRegistry,
  }),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useToolRegistries", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch all tool registries", async () => {
    const { result } = renderHook(() => useToolRegistries(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockToolRegistries);
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useToolRegistries(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should call getToolRegistries with workspace name", async () => {
    const { result } = renderHook(() => useToolRegistries(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // Tool registries are workspace-scoped
    expect(mockGetToolRegistries).toHaveBeenCalledWith("test-workspace");
  });

  it("should filter by phase on client-side", async () => {
    const { result } = renderHook(() => useToolRegistries({ phase: "Ready" }), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.every(r => r.status?.phase === "Ready")).toBe(true);
  });

  it("should filter by Degraded phase", async () => {
    const { result } = renderHook(() => useToolRegistries({ phase: "Degraded" }), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].metadata.name).toBe("slack-tools");
  });

  it("should handle empty response", async () => {
    mockGetToolRegistries.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useToolRegistries(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual([]);
  });
});

describe("useToolRegistry", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch a single tool registry", async () => {
    const { result } = renderHook(() => useToolRegistry("github-tools"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockToolRegistries[0]);
  });

  it("should call getToolRegistry with workspace and name", async () => {
    renderHook(() => useToolRegistry("github-tools"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetToolRegistry).toHaveBeenCalled();
    });

    // Tool registries are workspace-scoped
    expect(mockGetToolRegistry).toHaveBeenCalledWith("test-workspace", "github-tools");
  });

  it("should not fetch when name is empty", () => {
    renderHook(() => useToolRegistry(""), {
      wrapper: TestWrapper,
    });

    expect(mockGetToolRegistry).not.toHaveBeenCalled();
  });

  it("should return null when tool registry not found", async () => {
    mockGetToolRegistry.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useToolRegistry("non-existent"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });
});
