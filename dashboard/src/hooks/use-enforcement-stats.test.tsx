import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useEnforcementStats } from "./use-enforcement-stats";

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
  // Mock-to-contract: the shape mirrors the Go EnforcementStats json tags
  // ({ piiBlocked, redactions }) exactly, so a field-name drift fails here.
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        piiBlocked: 7,
        redactions: 12,
      }),
    }),
  );
});

describe("useEnforcementStats", () => {
  it("fetches enforcement stats for the current workspace", async () => {
    const { result } = renderHook(() => useEnforcementStats(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.piiBlocked).toBe(7);
    expect(result.current.data?.redactions).toBe(12);
    expect(fetch).toHaveBeenCalledWith(
      "/api/workspaces/default/privacy/enforcement-stats",
    );
  });

  it("defaults missing fields to zero", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({}),
      }),
    );
    const { result } = renderHook(() => useEnforcementStats(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.piiBlocked).toBe(0);
    expect(result.current.data?.redactions).toBe(0);
  });

  it("returns zeroed stats on auth/not-found errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        statusText: "Not Found",
      }),
    );
    const { result } = renderHook(() => useEnforcementStats(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.piiBlocked).toBe(0);
    expect(result.current.data?.redactions).toBe(0);
  });

  it("surfaces a query error on server failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
    const { result } = renderHook(() => useEnforcementStats(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(Error);
  });
});
