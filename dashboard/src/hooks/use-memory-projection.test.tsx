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
});
