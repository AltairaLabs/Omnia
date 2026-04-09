import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { useWorkspaceDetail, useWorkspacePatch } from "./use-workspace-detail";
import type { Workspace } from "@/types/workspace";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

const mockWorkspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws", namespace: "default" },
  spec: {
    displayName: "Test Workspace",
    description: "A test workspace",
    environment: "development",
    namespace: { name: "test-ns" },
    directGrants: [{ user: "alice@example.com", role: "owner" }],
  },
};

describe("useWorkspaceDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  it("fetches workspace data successfully", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => mockWorkspace,
    });

    const { result } = renderHook(() => useWorkspaceDetail("test-ws"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockWorkspace);
    expect(global.fetch).toHaveBeenCalledWith("/api/workspaces/test-ws?view=full");
  });

  it("does not fetch when name is null", () => {
    const { result } = renderHook(() => useWorkspaceDetail(null), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it("throws an error when the API returns a non-ok response", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      json: async () => ({ message: "Workspace not found" }),
    });

    const { result } = renderHook(() => useWorkspaceDetail("missing-ws"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
    expect((result.current.error as Error).message).toBe("Workspace not found");
  });
});

describe("useWorkspacePatch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  it("sends a PATCH request and returns updated workspace", async () => {
    const updated: Workspace = {
      ...mockWorkspace,
      spec: { ...mockWorkspace.spec, displayName: "Updated Name" },
    };

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => updated,
    });

    const { result } = renderHook(() => useWorkspacePatch("test-ws"), {
      wrapper: createWrapper(),
    });

    let data: Workspace | undefined;
    await act(async () => {
      data = await result.current.mutateAsync({ displayName: "Updated Name" });
    });

    expect(global.fetch).toHaveBeenCalledWith("/api/workspaces/test-ws", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ displayName: "Updated Name" }),
    });
    expect(data).toEqual(updated);
  });

  it("applies optimistic update to the cache before the request completes", async () => {
    // Pre-populate the cache with the original workspace
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    queryClient.setQueryData(["workspace-detail", "test-ws"], mockWorkspace);

    const wrapperWithClient = function ({ children }: { children: React.ReactNode }) {
      return createElement(QueryClientProvider, { client: queryClient }, children);
    };

    let resolvePatch!: (value: Response) => void;
    const patchPromise = new Promise<Response>((resolve) => {
      resolvePatch = resolve;
    });
    (global.fetch as ReturnType<typeof vi.fn>).mockReturnValueOnce(patchPromise);

    const { result } = renderHook(() => useWorkspacePatch("test-ws"), {
      wrapper: wrapperWithClient,
    });

    // Start the mutation but don't await it yet
    act(() => {
      result.current.mutate({ displayName: "Optimistic Name" });
    });

    // Optimistic update should already be applied
    await waitFor(() => {
      const cached = queryClient.getQueryData<Workspace>(["workspace-detail", "test-ws"]);
      expect(cached?.spec.displayName).toBe("Optimistic Name");
    });

    // Resolve the patch
    resolvePatch({
      ok: true,
      json: async () => ({ ...mockWorkspace, spec: { ...mockWorkspace.spec, displayName: "Optimistic Name" } }),
    } as Response);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("rolls back the cache on error", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    queryClient.setQueryData(["workspace-detail", "test-ws"], mockWorkspace);

    const wrapperWithClient = function ({ children }: { children: React.ReactNode }) {
      return createElement(QueryClientProvider, { client: queryClient }, children);
    };

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      json: async () => ({ message: "Server error" }),
    });

    const { result } = renderHook(() => useWorkspacePatch("test-ws"), {
      wrapper: wrapperWithClient,
    });

    await act(async () => {
      await expect(result.current.mutateAsync({ displayName: "Failed Update" })).rejects.toThrow("Server error");
    });

    // Cache should be rolled back to the original
    const cached = queryClient.getQueryData<Workspace>(["workspace-detail", "test-ws"]);
    expect(cached?.spec.displayName).toBe("Test Workspace");
  });
});
