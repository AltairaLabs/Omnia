/**
 * Tests for the workspace-scoped, demo-aware useCosts hook.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockUseWorkspace = vi.fn();
const mockUseRuntimeConfig = vi.fn();
const mockUseWorkspaceCosts = vi.fn();

vi.mock("@/contexts/workspace-context", () => ({ useWorkspace: () => mockUseWorkspace() }));
vi.mock("@/hooks/core", () => ({ useRuntimeConfig: () => mockUseRuntimeConfig() }));
vi.mock("./use-workspace-costs", () => ({
  useWorkspaceCosts: (...a: unknown[]) => mockUseWorkspaceCosts(...a),
}));

import { useCosts } from "./costs";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useCosts", () => {
  it("returns mock data in demo mode without fetching", () => {
    mockUseRuntimeConfig.mockReturnValue({ config: { demoMode: true } });
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "default" } });
    mockUseWorkspaceCosts.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const { result } = renderHook(() => useCosts());
    expect(result.current.data?.available).toBe(true);
    expect(result.current.isLoading).toBe(false);
    // useWorkspaceCosts is still called (hooks must run unconditionally) but disabled.
    expect(mockUseWorkspaceCosts).toHaveBeenCalledWith("default", { enabled: false });
  });

  it("delegates to useWorkspaceCosts for the current workspace otherwise", () => {
    mockUseRuntimeConfig.mockReturnValue({ config: { demoMode: false } });
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "team-a" } });
    const query = { data: { available: true }, isLoading: false, error: null };
    mockUseWorkspaceCosts.mockReturnValue(query);

    const { result } = renderHook(() => useCosts());
    expect(mockUseWorkspaceCosts).toHaveBeenCalledWith("team-a", { enabled: true });
    expect(result.current).toBe(query);
  });

  it("passes undefined workspace name through when none selected", () => {
    mockUseRuntimeConfig.mockReturnValue({ config: { demoMode: false } });
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null });
    mockUseWorkspaceCosts.mockReturnValue({ data: undefined, isLoading: false, error: null });

    renderHook(() => useCosts());
    expect(mockUseWorkspaceCosts).toHaveBeenCalledWith(undefined, { enabled: true });
  });
});
