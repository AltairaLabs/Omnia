/**
 * Tests for useSkillSourceContent hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useSkillSourceContent } from "./use-skill-source-content";
import { createQueryWrapper } from "@/test/query-wrapper";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: { name: "test-ws" } })),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("useSkillSourceContent", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.resetAllMocks());

  const renderH = <T,>(cb: () => T) =>
    renderHook(cb, { wrapper: createQueryWrapper() });

  it("returns empty initial state when no source name", () => {
    const { result } = renderH(() => useSkillSourceContent(undefined));
    expect(result.current.tree).toEqual([]);
    expect(result.current.fileCount).toBe(0);
  });

  it("fetches and exposes content tree on success", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        sourceName: "skills-git",
        tree: [{ name: "SKILL.md", path: "SKILL.md", isDirectory: false, size: 5 }],
        fileCount: 1,
        directoryCount: 0,
      }),
    });
    const { result } = renderH(() => useSkillSourceContent("skills-git"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tree).toHaveLength(1);
    expect(result.current.fileCount).toBe(1);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-ws/skills/skills-git/content"
    );
  });

  it("treats 404 as empty (source not yet synced)", async () => {
    mockFetch.mockResolvedValue({ ok: false, status: 404, statusText: "Not Found" });
    const { result } = renderH(() => useSkillSourceContent("skills-git"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tree).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("surfaces non-404 errors", async () => {
    mockFetch.mockResolvedValue({ ok: false, status: 500, statusText: "Server Error" });
    const { result } = renderH(() => useSkillSourceContent("skills-git"));
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.tree).toEqual([]);
  });

  it("surfaces fetch rejection as an error", async () => {
    mockFetch.mockRejectedValue(new Error("network"));
    const { result } = renderH(() => useSkillSourceContent("skills-git"));
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error?.message).toBe("network");
  });

  it("refetches when refetch() is called", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        sourceName: "skills-git",
        tree: [],
        fileCount: 0,
        directoryCount: 0,
      }),
    });
    const { result } = renderH(() => useSkillSourceContent("skills-git"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      result.current.refetch();
    });
    expect(mockFetch).toHaveBeenCalledTimes(2);
  });
});
