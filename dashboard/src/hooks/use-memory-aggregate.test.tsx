import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useMemoryAggregate } from "./use-memory-aggregate";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "default" },
    workspaces: [],
    setCurrentWorkspace: vi.fn(),
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { key: "institutional", value: 5, count: 5 },
        { key: "agent", value: 3, count: 3 },
        { key: "user", value: 8, count: 8 },
      ],
    }),
  );
});

describe("useMemoryAggregate", () => {
  it("fetches aggregate rows for the current workspace", async () => {
    const { result } = renderHook(
      () => useMemoryAggregate({ groupBy: "tier" }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(3);
    expect(fetch).toHaveBeenCalledWith(
      "/api/workspaces/default/memory/aggregate?groupBy=tier",
    );
  });

  it("forwards metric / from / to / limit", async () => {
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    const { result } = renderHook(
      () =>
        useMemoryAggregate({
          groupBy: "day",
          metric: "count",
          from: "2026-04-01T00:00:00Z",
          to: "2026-04-25T00:00:00Z",
          limit: 50,
        }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("groupBy=day");
    expect(url).toContain("metric=count");
    expect(url).toContain("from=2026-04-01T00");
    expect(url).toContain("to=2026-04-25T00");
    expect(url).toContain("limit=50");
  });

  it("respects the enabled flag", () => {
    const { result } = renderHook(
      () => useMemoryAggregate({ groupBy: "tier", enabled: false }),
      { wrapper },
    );
    expect(result.current.isPending).toBe(true);
    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});
