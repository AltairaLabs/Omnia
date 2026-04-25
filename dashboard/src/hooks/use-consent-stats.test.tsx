import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useConsentStats } from "./use-consent-stats";

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
        totalUsers: 100,
        optedOutAll: 5,
        grantsByCategory: { "memory:context": 90 },
      }),
    }),
  );
});

describe("useConsentStats", () => {
  it("fetches consent stats for the current workspace", async () => {
    const { result } = renderHook(() => useConsentStats(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.totalUsers).toBe(100);
    expect(result.current.data?.optedOutAll).toBe(5);
    expect(fetch).toHaveBeenCalledWith(
      "/api/workspaces/default/privacy/consent/stats",
    );
  });
});
