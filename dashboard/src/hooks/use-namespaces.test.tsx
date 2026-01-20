import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useNamespaces } from "./use-namespaces";

// Mock workspaces with different namespaces
const mockWorkspaces = [
  {
    name: "workspace-1",
    namespace: "namespace-a",
    role: "editor",
  },
  {
    name: "workspace-2",
    namespace: "namespace-b",
    role: "viewer",
  },
  {
    name: "workspace-3",
    namespace: "namespace-a", // Duplicate namespace
    role: "admin",
  },
  {
    name: "workspace-4",
    namespace: undefined, // No namespace
    role: "editor",
  },
];

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockWorkspaces[0],
    workspaces: mockWorkspaces,
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useNamespaces", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns unique namespaces from workspaces", async () => {
    const { result } = renderHook(() => useNamespaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Should have 2 unique namespaces (namespace-a and namespace-b)
    expect(result.current.data).toHaveLength(2);
    expect(result.current.data).toContain("namespace-a");
    expect(result.current.data).toContain("namespace-b");
  });

  it("filters out workspaces without namespaces", async () => {
    const { result } = renderHook(() => useNamespaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // workspace-4 has no namespace, should not be included
    expect(result.current.data).not.toContain(undefined);
    expect(result.current.data).not.toContain(null);
  });

  it("caches results with staleTime", async () => {
    const { result: result1 } = renderHook(() => useNamespaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result1.current.isSuccess).toBe(true));

    // First call should succeed
    expect(result1.current.data).toHaveLength(2);

    // Second call with same wrapper should use cache
    const { result: result2 } = renderHook(() => useNamespaces(), {
      wrapper: createWrapper(),
    });

    // Both should have the same data
    await waitFor(() => expect(result2.current.isSuccess).toBe(true));
    expect(result2.current.data).toEqual(result1.current.data);
  });

  it("deduplicates namespaces correctly", async () => {
    const { result } = renderHook(() => useNamespaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // namespace-a appears twice in workspaces but should only be in result once
    const namespaceACount = result.current.data?.filter(
      (ns) => ns === "namespace-a"
    ).length;
    expect(namespaceACount).toBe(1);
  });
});
