import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemoryProjection } from "./use-memory-projection";

const mockWorkspace = { name: "ws-1" };
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: mockWorkspace }),
}));

const mockDemo = vi.fn();
vi.mock("@/hooks/core", () => ({ useDemoMode: () => mockDemo() }));

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  mockDemo.mockReturnValue({ isDemoMode: false, loading: false });
  global.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ points: [{ id: "a" }], total: 1, model: "tsne" }),
  }) as unknown as typeof fetch;
});

describe("useMemoryProjection", () => {
  it("fetches the projection for the current workspace", async () => {
    const { result } = renderHook(() => useMemoryProjection(), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.points).toHaveLength(1);
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/workspaces/ws-1/memory/projection"),
    );
  });

  it("returns a deterministic mock projection in demo mode without fetching", async () => {
    mockDemo.mockReturnValue({ isDemoMode: true, loading: false });
    const fetchMock = vi.fn();
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useMemoryProjection(), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data!.points.length).toBeGreaterThan(0);
    expect(result.current.data!.embeddingModel).toBe("mock");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("polls while the projection is pending, then stops once ready", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ points: [], total: 5000, model: "tsne", status: "pending" }),
      })
      .mockResolvedValue({
        ok: true,
        json: async () => ({ points: [{ id: "a" }], total: 5000, model: "tsne", status: "ready" }),
      });
    global.fetch = fetchMock as unknown as typeof fetch;

    const { result } = renderHook(() => useMemoryProjection(), { wrapper });
    await waitFor(() => expect(result.current.data?.status).toBe("pending"));
    // refetchInterval (2s) refetches → backend now returns "ready" and polling stops.
    await waitFor(() => expect(result.current.data?.status).toBe("ready"), { timeout: 6000 });
    expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});
