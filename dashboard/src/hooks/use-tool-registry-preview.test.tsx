import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useToolRegistryPreview } from "./use-tool-registry-preview";
import type { LabelSelectorValue } from "@/components/ui/k8s-label-selector";

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

// Mock tool registry data with labels
const mockToolRegistries = [
  {
    metadata: {
      name: "main-tools",
      namespace: "omnia-system",
      uid: "uid-1",
      labels: {
        category: "core",
        env: "production",
      },
    },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 10,
    },
  },
  {
    metadata: {
      name: "dev-tools",
      namespace: "omnia-system",
      uid: "uid-2",
      labels: {
        category: "development",
        env: "staging",
      },
    },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 5,
    },
  },
  {
    metadata: {
      name: "mcp-tools",
      namespace: "omnia-system",
      uid: "uid-3",
      labels: {
        category: "mcp",
        env: "production",
      },
    },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 8,
    },
  },
  {
    metadata: {
      name: "unlabeled-tools",
      namespace: "omnia-system",
      uid: "uid-4",
      // No labels
    },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 3,
    },
  },
];

// Mock useDataService
const mockGetToolRegistries = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getToolRegistries: mockGetToolRegistries,
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

describe("useToolRegistryPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetToolRegistries.mockResolvedValue(mockToolRegistries);
  });

  it("should return empty array when selector is undefined", async () => {
    const { result } = renderHook(
      () => useToolRegistryPreview(undefined),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchingRegistries).toHaveLength(0);
    expect(result.current.matchCount).toBe(0);
    expect(result.current.totalCount).toBe(4);
  });

  it("should return all registries when selector is empty", async () => {
    const selector: LabelSelectorValue = {};

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchingRegistries).toHaveLength(4);
    expect(result.current.matchCount).toBe(4);
    expect(result.current.totalToolsCount).toBe(26); // 10 + 5 + 8 + 3
  });

  it("should filter registries by matchLabels", async () => {
    const selector: LabelSelectorValue = {
      matchLabels: { env: "production" },
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(2);
    expect(result.current.matchingRegistries.map(r => r.metadata.name)).toEqual(
      expect.arrayContaining(["main-tools", "mcp-tools"])
    );
    expect(result.current.totalToolsCount).toBe(18); // 10 + 8
  });

  it("should filter registries by multiple matchLabels (AND logic)", async () => {
    const selector: LabelSelectorValue = {
      matchLabels: { env: "production", category: "core" },
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingRegistries[0].metadata.name).toBe("main-tools");
    expect(result.current.totalToolsCount).toBe(10);
  });

  it("should filter registries by matchExpressions with In operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "category", operator: "In", values: ["core", "mcp"] },
      ],
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(2);
    expect(result.current.totalToolsCount).toBe(18); // 10 + 8
  });

  it("should filter registries by matchExpressions with NotIn operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "category", operator: "NotIn", values: ["development"] },
      ],
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // main-tools, mcp-tools, and unlabeled-tools (no category label)
    expect(result.current.matchCount).toBe(3);
    expect(result.current.totalToolsCount).toBe(21); // 10 + 8 + 3
  });

  it("should filter registries by matchExpressions with Exists operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "category", operator: "Exists" },
      ],
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // All registries with category label (excludes unlabeled-tools)
    expect(result.current.matchCount).toBe(3);
    expect(result.current.totalToolsCount).toBe(23); // 10 + 5 + 8
  });

  it("should filter registries by matchExpressions with DoesNotExist operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "category", operator: "DoesNotExist" },
      ],
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Only unlabeled-tools has no category label
    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingRegistries[0].metadata.name).toBe("unlabeled-tools");
    expect(result.current.totalToolsCount).toBe(3);
  });

  it("should extract available labels from all registries", async () => {
    const { result } = renderHook(
      () => useToolRegistryPreview(undefined),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.availableLabels).toEqual({
      category: ["core", "development", "mcp"],
      env: ["production", "staging"],
    });
  });

  it("should return empty availableLabels when no registries have labels", async () => {
    mockGetToolRegistries.mockResolvedValueOnce([
      {
        metadata: { name: "test", namespace: "default", uid: "1" },
        spec: { handlers: [] },
        status: { phase: "Ready", discoveredToolsCount: 1 },
      },
    ]);

    const { result } = renderHook(
      () => useToolRegistryPreview(undefined),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.availableLabels).toEqual({});
  });

  it("should handle combined matchLabels and matchExpressions", async () => {
    const selector: LabelSelectorValue = {
      matchLabels: { env: "production" },
      matchExpressions: [
        { key: "category", operator: "In", values: ["core"] },
      ],
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Only main-tools matches both conditions
    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingRegistries[0].metadata.name).toBe("main-tools");
    expect(result.current.totalToolsCount).toBe(10);
  });

  it("should handle registries with no discoveredToolsCount", async () => {
    mockGetToolRegistries.mockResolvedValueOnce([
      {
        metadata: {
          name: "empty-tools",
          namespace: "omnia-system",
          uid: "uid-1",
          labels: { env: "test" },
        },
        spec: { handlers: [] },
        status: { phase: "Ready" }, // No discoveredToolsCount
      },
    ]);

    const selector: LabelSelectorValue = {
      matchLabels: { env: "test" },
    };

    const { result } = renderHook(
      () => useToolRegistryPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(1);
    expect(result.current.totalToolsCount).toBe(0);
  });
});
