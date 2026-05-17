/**
 * Tests for useEvalFilter — session-api flavour.
 *
 * Previously mocked Prometheus client functions; after the observability
 * split (CLAUDE.md → Observability Boundaries) this hook fetches from
 * `/api/workspaces/{name}/eval-results/discover`, so tests mock the
 * structured service module instead.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

vi.mock("@/lib/data/eval-results-service", () => ({
  fetchEvalDiscovery: vi.fn(),
}));

import { useWorkspace } from "@/contexts/workspace-context";
import { fetchEvalDiscovery } from "@/lib/data/eval-results-service";
import { useEvalFilter } from "./use-eval-filter";

const mockedUseWorkspace = vi.mocked(useWorkspace);
const mockedFetchDiscovery = vi.mocked(fetchEvalDiscovery);

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

describe("useEvalFilter", () => {
  it("populates agents and promptpacks from one discovery call", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDiscovery.mockResolvedValue({
      evals: [{ evalId: "tone", evalType: "llm_judge" }],
      agents: ["chatbot", "support-agent"],
      promptpacks: ["default-pack"],
    });

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual(["chatbot", "support-agent"]);
    expect(result.current.promptpacks).toEqual(["default-pack"]);
    expect(result.current.filter).toEqual({});
    expect(mockedFetchDiscovery).toHaveBeenCalledOnce();
    expect(mockedFetchDiscovery).toHaveBeenCalledWith("test-ws");
  });

  it("returns empty arrays when discovery yields nothing", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDiscovery.mockResolvedValue({ evals: [], agents: [], promptpacks: [] });

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual([]);
    expect(result.current.promptpacks).toEqual([]);
  });

  it("returns empty arrays and is idle when no workspace is selected", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx(null));

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mockedFetchDiscovery).not.toHaveBeenCalled();
    expect(result.current.agents).toEqual([]);
    expect(result.current.promptpacks).toEqual([]);
  });

  it("returns empty arrays when discovery throws", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDiscovery.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.agents).toEqual([]);
    expect(result.current.promptpacks).toEqual([]);
  });

  it("builds filter from selected values", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDiscovery.mockResolvedValue({
      evals: [],
      agents: ["chatbot"],
      promptpacks: ["my-pack"],
    });

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.setAgent("chatbot"));
    expect(result.current.selectedAgent).toBe("chatbot");
    expect(result.current.filter).toEqual({ agent: "chatbot" });

    act(() => result.current.setPromptPack("my-pack"));
    expect(result.current.filter).toEqual({
      agent: "chatbot",
      promptpackName: "my-pack",
    });
  });

  it("clears filter when selection is set to undefined", async () => {
    mockedUseWorkspace.mockReturnValue(workspaceCtx("test-ws"));
    mockedFetchDiscovery.mockResolvedValue({ evals: [], agents: [], promptpacks: [] });

    const { result } = renderHook(() => useEvalFilter(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.setAgent("chatbot"));
    expect(result.current.filter).toEqual({ agent: "chatbot" });

    act(() => result.current.setAgent(undefined));
    expect(result.current.filter).toEqual({});
  });
});
