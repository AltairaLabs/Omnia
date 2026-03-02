/**
 * Tests for useEvalFilter hook.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const mockQueryPrometheus = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  EvalQueries: {
    discoverAgents: () => 'group({__name__=~"omnia_eval_.*"}) by (agent)',
    discoverPromptPacks: () => 'group({__name__=~"omnia_eval_.*"}) by (promptpack_name)',
  },
}));

import { useEvalFilter } from "./use-eval-filter";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useEvalFilter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("discovers agents and promptpacks from Prometheus", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { agent: "chatbot" }, value: [1000, "1"] },
            { metric: { agent: "support-agent" }, value: [1000, "1"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { promptpack_name: "default-pack" }, value: [1000, "1"] },
          ],
        },
      });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual(["chatbot", "support-agent"]);
    expect(result.current.promptpacks).toEqual(["default-pack"]);
    expect(result.current.filter).toEqual({});
  });

  it("returns empty arrays when Prometheus returns no data", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual([]);
    expect(result.current.promptpacks).toEqual([]);
  });

  it("returns empty arrays when Prometheus returns error", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "error",
      error: "bad query",
    });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual([]);
    expect(result.current.promptpacks).toEqual([]);
  });

  it("builds filter from selected values", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { agent: "chatbot" }, value: [1000, "1"] },
        ],
      },
    });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      result.current.setAgent("chatbot");
    });

    expect(result.current.selectedAgent).toBe("chatbot");
    expect(result.current.filter).toEqual({ agent: "chatbot" });

    act(() => {
      result.current.setPromptPack("my-pack");
    });

    expect(result.current.filter).toEqual({ agent: "chatbot", promptpackName: "my-pack" });
  });

  it("clears filter when selection is set to undefined", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      result.current.setAgent("chatbot");
    });
    expect(result.current.filter).toEqual({ agent: "chatbot" });

    act(() => {
      result.current.setAgent(undefined);
    });
    expect(result.current.filter).toEqual({});
  });

  it("filters out empty label values", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { agent: "chatbot" }, value: [1000, "1"] },
            { metric: { agent: "" }, value: [1000, "1"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [] },
      });

    const { result } = renderHook(() => useEvalFilter(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual(["chatbot"]);
  });
});
