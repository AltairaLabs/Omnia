/**
 * Tests for useProviderMetrics — session-api flavour.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

import { useProviderMetrics } from "./use-provider-metrics";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

vi.mock("@/lib/data/provider-calls-service", () => ({
  fetchProviderCallsAggregate: vi.fn(),
}));

import { useWorkspace } from "@/contexts/workspace-context";
import { fetchProviderCallsAggregate } from "@/lib/data/provider-calls-service";

const mockedUseWorkspace = vi.mocked(useWorkspace);
const mockedFetch = vi.mocked(fetchProviderCallsAggregate);

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  }
  return Wrapper;
}

function workspaceCtx(name: string | null) {
  return {
    currentWorkspace: name ? { name } : null,
  } as unknown as ReturnType<typeof useWorkspace>;
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useProviderMetrics", () => {
  it("is disabled when no workspace is selected", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx(null));
    const { result } = renderHook(
      () => useProviderMetrics("provider-1", "openai"),
      { wrapper: makeWrapper() },
    );
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(mockedFetch).not.toHaveBeenCalled();
  });

  it("is disabled when providerType is missing", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    const { result } = renderHook(
      () => useProviderMetrics("provider-1", undefined),
      { wrapper: makeWrapper() },
    );
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(mockedFetch).not.toHaveBeenCalled();
  });

  it("builds sparklines + 24h totals from 8 parallel aggregate calls", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    const seriesRows = [
      { key: "2026-05-15T10:00:00Z", value: 1, count: 1 },
      { key: "2026-05-15T11:00:00Z", value: 2, count: 2 },
    ];
    // Distinct value per metric so we can tell the series apart.
    const scaleByMetric: Record<string, number> = {
      count: 1,
      sum_input_tokens: 100,
      sum_output_tokens: 200,
      sum_cost_usd: 0.01,
    };
    const totalByMetric: Record<string, number> = {
      count: 42,
      sum_input_tokens: 1000,
      sum_output_tokens: 2000,
      sum_cost_usd: 0.55,
    };
    mockedFetch.mockImplementation(async ({ groupBy, metric }) => {
      if (groupBy === "time:hour") {
        const scale = scaleByMetric[metric] ?? 0;
        return seriesRows.map((r) => ({ ...r, value: r.value * scale }));
      }
      // groupBy === "provider" returns the 24h total in a single row.
      return [{ key: "openai", value: totalByMetric[metric] ?? 0, count: 42 }];
    });

    const { result } = renderHook(
      () => useProviderMetrics("provider-1", "openai"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data.available).toBe(true);
    expect(data.requestRate).toHaveLength(2);
    expect(data.inputTokenRate).toHaveLength(2);
    expect(data.outputTokenRate).toHaveLength(2);
    expect(data.costRate).toHaveLength(2);
    // Last point = current.
    expect(data.currentRequestRate).toBe(2);
    expect(data.currentInputTokenRate).toBe(200);
    expect(data.currentOutputTokenRate).toBe(400);

    expect(data.totalRequests24h).toBe(42);
    expect(data.totalTokens24h).toBe(3000); // 1000 input + 2000 output
    expect(data.totalCost24h).toBeCloseTo(0.55);

    expect(mockedFetch).toHaveBeenCalledTimes(8);
  });

  it("returns EMPTY_METRICS and warns when aggregate calls reject", async () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetch.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(
      () => useProviderMetrics("provider-1", "openai"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data.available).toBe(false);
    expect(data.requestRate).toEqual([]);
    expect(data.totalCost24h).toBe(0);
    expect(warnSpy).toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});
