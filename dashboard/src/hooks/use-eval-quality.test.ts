/**
 * Tests for useEvalSummary — session-api flavour.
 *
 * Previously mocked Prometheus client functions; after the observability
 * split (CLAUDE.md → Observability Boundaries) this hook fetches from
 * `/api/workspaces/{name}/eval-results/aggregate` and `.../discover`, so
 * tests mock the structured service module instead.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

import { useEvalSummary } from "./use-eval-quality";

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

describe("useEvalSummary", () => {
  it("returns empty when no workspace is selected", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx(null));

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(mockedFetchDescriptors).not.toHaveBeenCalled();
    expect(mockedFetchAggregate).not.toHaveBeenCalled();
  });

  it("returns one summary per discovered eval, sorted by eval_id", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "tone", evalType: "llm_judge" },
      { evalId: "safety", evalType: "llm_judge" },
    ]);
    mockedFetchAggregate.mockImplementation(async ({ evalId }) => {
      if (evalId === "safety") {
        return [{ key: "safety", value: 0.96, count: 12 }];
      }
      return [{ key: "tone", value: 0.85, count: 12 }];
    });

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const data = result.current.data!;
    expect(data).toHaveLength(2);
    // Sorted alphabetically by eval_id.
    expect(data[0]).toMatchObject({ evalId: "safety", score: 0.96, metricType: "gauge" });
    expect(data[1]).toMatchObject({ evalId: "tone", score: 0.85, metricType: "gauge" });
  });

  it("returns empty array when no evals exist in this workspace", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([]);

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
    expect(mockedFetchAggregate).not.toHaveBeenCalled();
  });

  it("defaults score to 0 when an eval has no scored rows yet", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "tone", evalType: "llm_judge" },
    ]);
    mockedFetchAggregate.mockResolvedValue([]); // no rows back

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([
      { evalId: "tone", score: 0, metricType: "gauge" },
    ]);
  });

  it("classifies assertion-type evals as boolean", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "exact-match", evalType: "assertion" },
    ]);
    mockedFetchAggregate.mockResolvedValue([
      { key: "exact-match", value: 1.0, count: 4 },
    ]);

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0].metricType).toBe("boolean");
  });

  it("forwards filter agent/promptpackName to the aggregate call", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockResolvedValue([
      { evalId: "tone", evalType: "llm_judge" },
    ]);
    mockedFetchAggregate.mockResolvedValue([
      { key: "tone", value: 0.7, count: 3 },
    ]);

    renderHook(
      () => useEvalSummary({ agent: "chatbot", promptpackName: "v2" }),
      { wrapper: makeWrapper() },
    );

    await waitFor(() => expect(mockedFetchAggregate).toHaveBeenCalledOnce());
    expect(mockedFetchAggregate.mock.calls[0][0]).toMatchObject({
      agentName: "chatbot",
      promptpackName: "v2",
      evalId: "tone",
      groupBy: "eval_id",
      metric: "avg_score",
    });
  });

  it("propagates discovery errors via the query's error state", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDescriptors.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useEvalSummary(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain("boom");
  });
});
