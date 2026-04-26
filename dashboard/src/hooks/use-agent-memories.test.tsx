import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useAgentMemories } from "./use-agent-memories";

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
      json: async () => ({
        memories: [
          { id: "a-1", tier: "agent", scope: { agent_id: "support" } },
          { id: "a-2", tier: "agent", scope: { agent_id: "support" } },
        ],
        total: 2,
      }),
    }),
  );
});

describe("useAgentMemories", () => {
  it("fetches agent memories for the given agent id", async () => {
    const { result } = renderHook(
      () => useAgentMemories({ agentId: "support" }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.total).toBe(2);
    expect(fetch).toHaveBeenCalledWith(
      "/api/workspaces/default/agent-memories?agent=support",
    );
  });

  it("does not fetch when agentId is empty", () => {
    const { result } = renderHook(
      () => useAgentMemories({ agentId: undefined }),
      { wrapper },
    );
    expect(result.current.fetchStatus).toBe("idle");
    expect(fetch).not.toHaveBeenCalled();
  });
});
