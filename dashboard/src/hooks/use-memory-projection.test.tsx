import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemoryProjection } from "./use-memory-projection";

const mockWorkspace = { name: "ws-1" };
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: mockWorkspace }),
}));

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

beforeEach(() => {
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
