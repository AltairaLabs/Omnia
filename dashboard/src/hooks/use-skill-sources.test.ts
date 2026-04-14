/**
 * Tests for useSkillSources / useSkillSource hooks. Issue #829.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  useSkillSources,
  useSkillSource,
  useSkillSourceMutations,
} from "./use-skill-sources";
import type { SkillSourceSpec } from "@/types/skill-source";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockWorkspace = {
  name: "test-workspace",
  displayName: "Test",
  environment: "development" as const,
  namespace: "test-ns",
  role: "editor" as const,
  permissions: {
    read: true,
    write: true,
    delete: true,
    manageMembers: false,
  },
};

const mockSource = {
  metadata: { name: "skills-git" },
  spec: {
    type: "git" as const,
    interval: "1h",
    git: { url: "https://example.com/skills.git" },
  },
  status: { phase: "Ready" as const, skillCount: 3 },
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(
      QueryClientProvider,
      { client: queryClient },
      children
    );
  }
  return Wrapper;
}

describe("useSkillSources", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty list when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
       
    } as any);

    const { result } = renderHook(() => useSkillSources(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.sources).toEqual([]);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("fetches skill sources for the current workspace", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => [mockSource],
    });

    const { result } = renderHook(() => useSkillSources(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/skills");
    expect(result.current.sources).toHaveLength(1);
    expect(result.current.sources[0].metadata.name).toBe("skills-git");
  });

  it("surfaces fetch errors", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);
    mockFetch.mockResolvedValue({
      ok: false,
      statusText: "Internal Server Error",
      json: async () => ({}),
    });

    const { result } = renderHook(() => useSkillSources(), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error?.message).toContain("Internal Server Error");
  });
});

describe("useSkillSource", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("fetches a single skill source by name", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSource,
    });

    const { result } = renderHook(() => useSkillSource("skills-git"), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/skills/skills-git"
    );
    expect(result.current.source?.metadata.name).toBe("skills-git");
  });

  it("returns not-found error when response is 404", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);
    mockFetch.mockResolvedValue({
      ok: false,
      status: 404,
      statusText: "Not Found",
      json: async () => ({}),
    });

    const { result } = renderHook(() => useSkillSource("missing"), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error?.message).toBe("Skill source not found");
  });

  it("does not fetch when name is undefined", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);

    const { result } = renderHook(() => useSkillSource(undefined), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.source).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

describe("useSkillSourceMutations", () => {
  const spec: SkillSourceSpec = {
    type: "configmap",
    interval: "1h",
    configMap: { name: "skills-cm" },
  };

  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  async function setupWithWorkspace() {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
       
    } as any);
  }

  it("createSource POSTs to the collection endpoint", async () => {
    await setupWithWorkspace();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSource,
    });
    const { result } = renderHook(() => useSkillSourceMutations(), {
      wrapper: createWrapper(),
    });
    await result.current.createSource("skills-cm", spec);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/skills",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("updateSource PUTs to the item endpoint", async () => {
    await setupWithWorkspace();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockSource,
    });
    const { result } = renderHook(() => useSkillSourceMutations(), {
      wrapper: createWrapper(),
    });
    await result.current.updateSource("skills-cm", spec);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/skills/skills-cm",
      expect.objectContaining({ method: "PUT" })
    );
  });

  it("deleteSource DELETEs the item endpoint", async () => {
    await setupWithWorkspace();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
    const { result } = renderHook(() => useSkillSourceMutations(), {
      wrapper: createWrapper(),
    });
    await result.current.deleteSource("skills-cm");
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/skills/skills-cm",
      expect.objectContaining({ method: "DELETE" })
    );
  });

  it("surfaces server error messages", async () => {
    await setupWithWorkspace();
    mockFetch.mockResolvedValue({
      ok: false,
      json: async () => ({ error: "Forbidden" }),
    });
    const { result } = renderHook(() => useSkillSourceMutations(), {
      wrapper: createWrapper(),
    });
    await expect(result.current.createSource("x", spec)).rejects.toThrow(
      "Forbidden"
    );
  });

  it("rejects when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
       
    } as any);
    const { result } = renderHook(() => useSkillSourceMutations(), {
      wrapper: createWrapper(),
    });
    await expect(result.current.createSource("x", spec)).rejects.toThrow(
      "No workspace selected"
    );
  });
});
