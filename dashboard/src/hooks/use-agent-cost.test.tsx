/**
 * Tests for useAgentCost — session-api flavour.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

import { useAgentCost } from "./use-agent-cost";

vi.mock("@/lib/data/provider-calls-service", () => ({
  fetchProviderCallsAggregate: vi.fn(),
}));

import { fetchProviderCallsAggregate } from "@/lib/data/provider-calls-service";

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

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useAgentCost", () => {
  it("is disabled when workspace or agent is empty", async () => {
    const { result } = renderHook(() => useAgentCost("", ""), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(mockedFetch).not.toHaveBeenCalled();
  });

  it("fetches totals + 24h sparkline and shapes them into AgentCostData", async () => {
    // Build a 12-hour history (12 of the 24 buckets populated).
    const now = new Date("2026-05-15T12:30:00Z");
    vi.setSystemTime(now);
    const currentHour = Math.floor(now.getTime() / (60 * 60 * 1000)) * (60 * 60 * 1000);
    const sparklineRows = Array.from({ length: 12 }, (_, i) => ({
      key: new Date(currentHour - (11 - i) * 60 * 60 * 1000).toISOString(),
      value: 0.01 * (i + 1),
      count: 1,
    }));

    const totalsByMetric: Record<string, number> = {
      sum_cost_usd: 0.42,
      sum_input_tokens: 1000,
      sum_output_tokens: 2000,
      count: 7,
    };
    mockedFetch.mockImplementation(async ({ groupBy, metric }) => {
      if (groupBy === "time:hour") return sparklineRows;
      // group=agent collapses to one row.
      return [{ key: "chatbot", value: totalsByMetric[metric] ?? 0, count: 7 }];
    });

    const { result } = renderHook(
      () => useAgentCost("test-ws", "chatbot"),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data.available).toBe(true);
    expect(data.totalCost).toBeCloseTo(0.42);
    expect(data.inputTokens).toBe(1000);
    expect(data.outputTokens).toBe(2000);
    expect(data.requests).toBe(7);

    // Sparkline is always 24 points; first 12 are 0 (unfilled), last 12 are
    // the populated values (oldest-first inside the populated window).
    expect(data.timeSeries).toHaveLength(24);
    expect(data.timeSeries.slice(0, 12).every((p) => p.value === 0)).toBe(true);
    expect(data.timeSeries[12].value).toBeCloseTo(0.01);
    expect(data.timeSeries[23].value).toBeCloseTo(0.12);

    // Four totals + one sparkline = five aggregate calls.
    expect(mockedFetch).toHaveBeenCalledTimes(5);
    vi.useRealTimers();
  });

  it("returns the EMPTY_DATA shape and logs when any aggregate call throws", async () => {
    const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockedFetch.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(
      () => useAgentCost("test-ws", "chatbot"),
      { wrapper: makeWrapper() },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const data = result.current.data!;
    expect(data.available).toBe(false);
    expect(data.totalCost).toBe(0);
    expect(data.timeSeries).toEqual([]);
    expect(errSpy).toHaveBeenCalled();
    errSpy.mockRestore();
  });
});
