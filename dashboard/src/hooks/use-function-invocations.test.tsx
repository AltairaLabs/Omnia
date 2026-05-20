/**
 * Tests for useFunctionInvocations.
 *
 * Covers:
 *  - query key shape (so cache entries don't bleed between filters)
 *  - enablement: hook is disabled when workspace is empty
 *  - happy path: rows flow from the service into hook state
 *  - error path: the hook reports error state on rejection
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useFunctionInvocations } from "./use-function-invocations";
import type { FunctionInvocation } from "@/lib/data/function-invocations-service";

const SAMPLE_ROW: FunctionInvocation = {
  id: "inv-1",
  namespace: "ns-a",
  functionName: "summarizer",
  inputHash: "abc",
  status: "success",
  durationMs: 42,
  costUsd: 0.001,
  createdAt: "2026-05-20T10:00:00Z",
};

const fetchSpy = vi.hoisted(() => vi.fn());

vi.mock("@/lib/data/function-invocations-service", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/data/function-invocations-service")
  >("@/lib/data/function-invocations-service");
  return {
    ...actual,
    fetchFunctionInvocations: fetchSpy,
  };
});

function newWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = "QueryWrapper";
  return Wrapper;
}

beforeEach(() => {
  fetchSpy.mockReset();
});

describe("useFunctionInvocations", () => {
  it("does not fetch when workspace is empty", () => {
    fetchSpy.mockResolvedValueOnce([]);
    const { result } = renderHook(
      () => useFunctionInvocations({ workspace: "" }),
      { wrapper: newWrapper() },
    );
    expect(result.current.isFetching).toBe(false);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("returns rows from the service on the happy path", async () => {
    fetchSpy.mockResolvedValueOnce([SAMPLE_ROW]);
    const { result } = renderHook(
      () =>
        useFunctionInvocations({
          workspace: "ws",
          functionName: "summarizer",
          fromIso: "2026-05-19T00:00:00Z",
          limit: 100,
        }),
      { wrapper: newWrapper() },
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(result.current.data).toEqual([SAMPLE_ROW]);

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const call = fetchSpy.mock.calls[0][0];
    expect(call.workspace).toBe("ws");
    expect(call.functionName).toBe("summarizer");
    expect(call.from).toBeInstanceOf(Date);
    expect(call.from.toISOString()).toBe("2026-05-19T00:00:00.000Z");
    expect(call.limit).toBe(100);
  });

  it("surfaces service errors on the error path", async () => {
    fetchSpy.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(
      () => useFunctionInvocations({ workspace: "ws" }),
      { wrapper: newWrapper() },
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toEqual(new Error("boom"));
  });

  it("invalidates cache when filters change (no row bleed across views)", async () => {
    // First fetch — summarizer
    fetchSpy.mockResolvedValueOnce([{ ...SAMPLE_ROW, functionName: "summarizer" }]);
    const { result, rerender } = renderHook(
      ({ fn }: { fn: string }) =>
        useFunctionInvocations({ workspace: "ws", functionName: fn }),
      { wrapper: newWrapper(), initialProps: { fn: "summarizer" } },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0].functionName).toBe("summarizer");

    // Second fetch — classifier; the rerender must trigger a NEW query
    // (different key) rather than show stale summarizer rows.
    fetchSpy.mockResolvedValueOnce([{ ...SAMPLE_ROW, functionName: "classifier" }]);
    rerender({ fn: "classifier" });
    await waitFor(() => {
      expect(result.current.data?.[0].functionName).toBe("classifier");
    });
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});
