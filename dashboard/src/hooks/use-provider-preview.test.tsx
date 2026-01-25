import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useProviderPreview } from "./use-provider-preview";
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

// Mock provider data with labels
const mockProviders = [
  {
    metadata: {
      name: "claude-prod",
      namespace: "omnia-system",
      uid: "uid-1",
      labels: {
        env: "production",
        tier: "primary",
        type: "claude",
      },
    },
    spec: { type: "claude", model: "claude-3" },
    status: { phase: "Ready" },
  },
  {
    metadata: {
      name: "claude-staging",
      namespace: "omnia-system",
      uid: "uid-2",
      labels: {
        env: "staging",
        tier: "primary",
        type: "claude",
      },
    },
    spec: { type: "claude", model: "claude-3" },
    status: { phase: "Ready" },
  },
  {
    metadata: {
      name: "openai-prod",
      namespace: "omnia-system",
      uid: "uid-3",
      labels: {
        env: "production",
        tier: "secondary",
        type: "openai",
      },
    },
    spec: { type: "openai", model: "gpt-4" },
    status: { phase: "Ready" },
  },
  {
    metadata: {
      name: "ollama-local",
      namespace: "omnia-system",
      uid: "uid-4",
      // No labels
    },
    spec: { type: "ollama", model: "llama2" },
    status: { phase: "Ready" },
  },
];

// Mock useDataService
const mockGetProviders = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getProviders: mockGetProviders,
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

describe("useProviderPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetProviders.mockResolvedValue(mockProviders);
  });

  it("should return all providers when selector is undefined", async () => {
    const { result } = renderHook(
      () => useProviderPreview(undefined),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchingProviders).toHaveLength(0);
    expect(result.current.matchCount).toBe(0);
    expect(result.current.totalCount).toBe(4);
  });

  it("should return all providers when selector is empty", async () => {
    const selector: LabelSelectorValue = {};

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchingProviders).toHaveLength(4);
    expect(result.current.matchCount).toBe(4);
  });

  it("should filter providers by matchLabels", async () => {
    const selector: LabelSelectorValue = {
      matchLabels: { env: "production" },
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(2);
    expect(result.current.matchingProviders.map(p => p.metadata.name)).toEqual(
      expect.arrayContaining(["claude-prod", "openai-prod"])
    );
  });

  it("should filter providers by multiple matchLabels (AND logic)", async () => {
    const selector: LabelSelectorValue = {
      matchLabels: { env: "production", type: "claude" },
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingProviders[0].metadata.name).toBe("claude-prod");
  });

  it("should filter providers by matchExpressions with In operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "type", operator: "In", values: ["claude", "openai"] },
      ],
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.matchCount).toBe(3);
  });

  it("should filter providers by matchExpressions with NotIn operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "env", operator: "NotIn", values: ["production"] },
      ],
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // staging provider and ollama-local (no env label)
    expect(result.current.matchCount).toBe(2);
    expect(result.current.matchingProviders.map(p => p.metadata.name)).toEqual(
      expect.arrayContaining(["claude-staging", "ollama-local"])
    );
  });

  it("should filter providers by matchExpressions with Exists operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "tier", operator: "Exists" },
      ],
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // All providers with tier label (excludes ollama-local)
    expect(result.current.matchCount).toBe(3);
  });

  it("should filter providers by matchExpressions with DoesNotExist operator", async () => {
    const selector: LabelSelectorValue = {
      matchExpressions: [
        { key: "tier", operator: "DoesNotExist" },
      ],
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Only ollama-local has no tier label
    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingProviders[0].metadata.name).toBe("ollama-local");
  });

  it("should extract available labels from all providers", async () => {
    const { result } = renderHook(
      () => useProviderPreview(undefined),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.availableLabels).toEqual({
      env: ["production", "staging"],
      tier: ["primary", "secondary"],
      type: ["claude", "openai"],
    });
  });

  it("should return empty availableLabels when no providers have labels", async () => {
    mockGetProviders.mockResolvedValueOnce([
      {
        metadata: { name: "test", namespace: "default", uid: "1" },
        spec: { type: "mock" },
        status: { phase: "Ready" },
      },
    ]);

    const { result } = renderHook(
      () => useProviderPreview(undefined),
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
        { key: "tier", operator: "In", values: ["primary"] },
      ],
    };

    const { result } = renderHook(
      () => useProviderPreview(selector),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Only claude-prod matches both conditions
    expect(result.current.matchCount).toBe(1);
    expect(result.current.matchingProviders[0].metadata.name).toBe("claude-prod");
  });
});
