/**
 * Tests for useEvalScoreTrends / useEvalMetrics — session-api flavour.
 *
 * Previously mocked Prometheus client functions; after the observability
 * split (see CLAUDE.md → Observability Boundaries) these hooks fetch from
 * `/api/workspaces/{name}/eval-results/aggregate` and `.../discover`, so
 * the tests mock the structured service module instead.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

import { useEvalScoreTrends, useEvalMetrics, EVAL_TREND_RANGES } from "./use-eval-trends";

// --- Mocks ----------------------------------------------------------------

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

vi.mock("@/lib/data/eval-results-service", () => ({
  fetchEvalAggregate: vi.fn(),
  fetchEvalDescriptors: vi.fn(),
  classifyEvalType: (t: string) => (t === "assertion" ? "boolean" : "gauge"),
}));

import { useWorkspace } from "@/contexts/workspace-context";
import {
  fetchEvalAggregate,
  fetchEvalDescriptors,
} from "@/lib/data/eval-results-service";

const mockedUseWorkspace = vi.mocked(useWorkspace);
const mockedFetchAggregate = vi.mocked(fetchEvalAggregate);
const mockedFetchDescriptors = vi.mocked(fetchEvalDescriptors);

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

// --- EVAL_TREND_RANGES surface --------------------------------------------

describe("EVAL_TREND_RANGES", () => {
  it("exposes the expected ranges with a groupBy mapping", () => {
    expect(EVAL_TREND_RANGES["24h"].seconds).toBe(86400);
    expect(EVAL_TREND_RANGES["24h"].groupBy).toBe("time:hour");
    expect(EVAL_TREND_RANGES["7d"].groupBy).toBe("time:day");
  });
});

// --- useEvalScoreTrends ---------------------------------------------------

describe("useEvalScoreTrends", () => {
  it("returns empty trends when no workspace is selected", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx(null));

    const { result } = renderHook(() => useEvalScoreTrends(), { wrapper: makeWrapper() });

    // The query is disabled without a workspace; data stays undefined and
    // service calls don't happen.
    await waitFor(() => {
      expect(result.current.isFetching).toBe(false);
    });
    expect(mockedFetchDescriptors).not.toHaveBeenCalled();
    expect(mockedFetchAggregate).not.toHaveBeenCalled();
  });

  it("discovers evals then fetches aggregate per eval and merges by timestamp", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "acc", evalType: "llm_judge" },
      { evalId: "lat", evalType: "llm_judge" },
    ]);
    mockedFetchAggregate.mockImplementation(async ({ evalId }) => {
      if (evalId === "acc") {
        return [
          { key: "2026-05-01T00:00:00Z", value: 0.85, count: 2 },
          { key: "2026-05-02T00:00:00Z", value: 0.9, count: 2 },
        ];
      }
      return [{ key: "2026-05-01T00:00:00Z", value: 0.5, count: 1 }];
    });

    const { result } = renderHook(
      () => useEvalScoreTrends({ timeRange: "7d" }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
    // Sorted ascending by timestamp; first row merges both evals' values.
    expect(result.current.data?.[0].values).toEqual({ acc: 0.85, lat: 0.5 });
    expect(result.current.data?.[1].values).toEqual({ acc: 0.9 });

    // Discovery was called once; aggregate once per discovered eval.
    expect(mockedFetchDescriptors).toHaveBeenCalledOnce();
    expect(mockedFetchAggregate).toHaveBeenCalledTimes(2);
    // Time bucket choice follows EVAL_TREND_RANGES["7d"] → "time:day".
    expect(mockedFetchAggregate.mock.calls[0][0].groupBy).toBe("time:day");
    expect(mockedFetchAggregate.mock.calls[0][0].metric).toBe("avg_score");
  });

  it("uses caller-supplied metric names without calling discovery", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchAggregate.mockResolvedValue([
      { key: "2026-05-01T00:00:00Z", value: 0.9, count: 1 },
    ]);

    const { result } = renderHook(
      () => useEvalScoreTrends({ metricNames: ["only-this"] }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockedFetchDescriptors).not.toHaveBeenCalled();
    expect(mockedFetchAggregate).toHaveBeenCalledOnce();
    expect(mockedFetchAggregate.mock.calls[0][0].evalId).toBe("only-this");
  });

  it("propagates the filter through to aggregate calls", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchAggregate.mockResolvedValue([]);

    renderHook(
      () =>
        useEvalScoreTrends({
          metricNames: ["acc"],
          filter: { agent: "chatbot", promptpackName: "v2" },
        }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(mockedFetchAggregate).toHaveBeenCalledOnce());
    expect(mockedFetchAggregate.mock.calls[0][0]).toMatchObject({
      agentName: "chatbot",
      promptpackName: "v2",
      evalId: "acc",
    });
  });

  it("strips the omnia_eval_ prefix from merged value keys for back-compat", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "omnia_eval_acc", evalType: "llm_judge" },
    ]);
    mockedFetchAggregate.mockResolvedValue([
      { key: "2026-05-01T00:00:00Z", value: 0.7, count: 1 },
    ]);

    const { result } = renderHook(() => useEvalScoreTrends(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0].values).toEqual({ acc: 0.7 });
  });

  it("returns empty trends when discovery yields no evals", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([]);

    const { result } = renderHook(() => useEvalScoreTrends(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
    expect(mockedFetchAggregate).not.toHaveBeenCalled();
  });
});

// --- useEvalMetrics -------------------------------------------------------

describe("useEvalMetrics", () => {
  it("returns empty when no workspace", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx(null));

    const { result } = renderHook(() => useEvalMetrics(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(mockedFetchDescriptors).not.toHaveBeenCalled();
  });

  it("builds EvalMetricInfo[] from discovery + current + sparkline aggregates", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "acc", evalType: "llm_judge" },
      { evalId: "exact-match", evalType: "assertion" },
    ]);
    // Each eval results in TWO aggregate calls: current value + sparkline.
    mockedFetchAggregate.mockImplementation(async ({ groupBy, evalId }) => {
      if (groupBy === "eval_id") {
        return [{ key: evalId ?? "", value: evalId === "acc" ? 0.92 : 1.0, count: 1 }];
      }
      // sparkline
      return [
        { key: "2026-05-01T00:00:00Z", value: 0.9, count: 1 },
        { key: "2026-05-01T01:00:00Z", value: 0.95, count: 1 },
      ];
    });

    const { result } = renderHook(() => useEvalMetrics(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);

    const byName = new Map(result.current.data!.map((m) => [m.name, m]));

    expect(byName.get("acc")?.value).toBeCloseTo(0.92);
    expect(byName.get("acc")?.metricType).toBe("gauge");
    expect(byName.get("acc")?.sparkline).toEqual([{ value: 0.9 }, { value: 0.95 }]);

    expect(byName.get("exact-match")?.value).toBeCloseTo(1.0);
    // assertion eval_type → boolean classification
    expect(byName.get("exact-match")?.metricType).toBe("boolean");
  });

  it("returns empty list when discovery yields nothing", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([]);

    const { result } = renderHook(() => useEvalMetrics(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});
