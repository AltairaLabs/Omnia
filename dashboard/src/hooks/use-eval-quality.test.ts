/**
 * Tests for eval quality hooks.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockGetEvalResultsSummary, mockGetEvalResults } = vi.hoisted(() => {
  return {
    mockGetEvalResultsSummary: vi.fn(),
    mockGetEvalResults: vi.fn(),
  };
});

vi.mock("@/lib/data/session-api-service", () => ({
  SessionApiService: class MockSessionApiService {
    getEvalResultsSummary = mockGetEvalResultsSummary;
    getEvalResults = mockGetEvalResults;
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

import { useEvalSummary, useRecentEvalFailures } from "./use-eval-quality";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useEvalSummary", () => {
  beforeEach(() => {
    mockGetEvalResultsSummary.mockReset();
    mockGetEvalResultsSummary.mockResolvedValue([
      {
        evalId: "tone",
        evalType: "llm_judge",
        total: 100,
        passed: 85,
        failed: 15,
        passRate: 85.0,
        avgScore: 0.85,
      },
      {
        evalId: "safety",
        evalType: "rule",
        total: 50,
        passed: 48,
        failed: 2,
        passRate: 96.0,
      },
    ]);
  });

  it("fetches eval summaries for the current workspace", async () => {
    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(result.current.isSuccess).toBe(true);
    expect(mockGetEvalResultsSummary).toHaveBeenCalledWith("test-workspace", undefined);
    expect(result.current.data).toHaveLength(2);
    expect(result.current.data![0].evalId).toBe("tone");
  });

  it("passes filter params to service", async () => {
    const params = { agentName: "agent-1", createdAfter: "2026-01-01T00:00:00Z" };
    const { result } = renderHook(() => useEvalSummary(params), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(mockGetEvalResultsSummary).toHaveBeenCalledWith("test-workspace", params);
  });
});

describe("useRecentEvalFailures", () => {
  beforeEach(() => {
    mockGetEvalResults.mockReset();
    mockGetEvalResults.mockResolvedValue({
      evalResults: [
        {
          id: "e1",
          sessionId: "s1",
          agentName: "agent-1",
          evalId: "tone",
          evalType: "llm_judge",
          passed: false,
          score: 0.3,
          createdAt: new Date().toISOString(),
        },
      ],
      total: 1,
    });
  });

  it("fetches recent failures with passed=false default", async () => {
    const { result } = renderHook(() => useRecentEvalFailures(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(result.current.isSuccess).toBe(true);
    expect(mockGetEvalResults).toHaveBeenCalledWith("test-workspace", {
      passed: false,
      limit: 10,
    });
    expect(result.current.data?.evalResults).toHaveLength(1);
    expect(result.current.data?.evalResults[0].passed).toBe(false);
  });

  it("merges custom params with defaults", async () => {
    const { result } = renderHook(
      () => useRecentEvalFailures({ agentName: "agent-2", limit: 5 }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(mockGetEvalResults).toHaveBeenCalledWith("test-workspace", {
      passed: false,
      limit: 5,
      agentName: "agent-2",
    });
  });
});
