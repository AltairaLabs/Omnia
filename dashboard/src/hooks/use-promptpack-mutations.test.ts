import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { usePromptPackMutations } from "./use-promptpack-mutations";
import type { PromptPackSpec } from "@/types/prompt-pack";

// Mock workspace context
const mockWorkspace = { name: "test-workspace", namespace: "test-ns" };
let currentWorkspace: typeof mockWorkspace | null = mockWorkspace;

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace,
    workspaces: currentWorkspace ? [currentWorkspace] : [],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

const mockSpec: PromptPackSpec = {
  source: { type: "configmap", configMapRef: { name: "my-configmap" } },
  version: "1.0.0",
  rollout: { type: "immediate" },
};

describe("usePromptPackMutations", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    currentWorkspace = mockWorkspace;
    global.fetch = vi.fn();
  });

  it("creates a PromptPack successfully", async () => {
    const mockResponse = {
      metadata: { name: "my-pack" },
      spec: mockSpec,
    };
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockResponse),
    });

    const { result } = renderHook(() => usePromptPackMutations());

    let response: unknown;
    await act(async () => {
      response = await result.current.createPromptPack("my-pack", mockSpec);
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/promptpacks",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name: "my-pack" }, spec: mockSpec }),
      })
    );
    expect(response).toEqual(mockResponse);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("handles API error", async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      text: () => Promise.resolve("conflict: already exists"),
    });

    const { result } = renderHook(() => usePromptPackMutations());

    await act(async () => {
      try {
        await result.current.createPromptPack("my-pack", mockSpec);
      } catch {
        // expected
      }
    });

    expect(result.current.error?.message).toBe("conflict: already exists");
    expect(result.current.loading).toBe(false);
  });

  it("throws when no workspace selected", async () => {
    currentWorkspace = null;

    const { result } = renderHook(() => usePromptPackMutations());

    await expect(
      act(async () => {
        await result.current.createPromptPack("my-pack", mockSpec);
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("sets loading during request", async () => {
    let resolveFetch: (value: unknown) => void;
    (global.fetch as ReturnType<typeof vi.fn>).mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      })
    );

    const { result } = renderHook(() => usePromptPackMutations());

    let promise: Promise<unknown>;
    act(() => {
      promise = result.current.createPromptPack("my-pack", mockSpec);
    });

    expect(result.current.loading).toBe(true);

    await act(async () => {
      resolveFetch!({
        ok: true,
        json: () => Promise.resolve({ metadata: { name: "my-pack" } }),
      });
      await promise!;
    });

    expect(result.current.loading).toBe(false);
  });
});
