import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useProviderMutations } from "./use-provider-mutations";

// Mock workspace context - use a mutable ref so tests can change it
let mockCurrentWorkspace: { name: string } | null = { name: "test-workspace" };

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
  }),
}));

describe("useProviderMutations", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
    mockCurrentWorkspace = { name: "test-workspace" };
  });

  describe("createProvider", () => {
    it("sends POST request with correct URL and body", async () => {
      const mockProvider = { metadata: { name: "new-provider" }, spec: { type: "claude" } };
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockProvider),
      });

      const { result } = renderHook(() => useProviderMutations());

      let created;
      await act(async () => {
        created = await result.current.createProvider("new-provider", { type: "claude" });
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/providers",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name: "new-provider" }, spec: { type: "claude" } }),
        }
      );
      expect(created).toEqual(mockProvider);
      expect(result.current.loading).toBe(false);
      expect(result.current.error).toBeNull();
    });

    it("sets error on failed request", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: false,
        text: () => Promise.resolve("Conflict"),
      });

      const { result } = renderHook(() => useProviderMutations());

      await act(async () => {
        try {
          await result.current.createProvider("existing", { type: "claude" });
        } catch {
          // expected
        }
      });

      await waitFor(() => {
        expect(result.current.error?.message).toBe("Conflict");
      });
      expect(result.current.loading).toBe(false);
    });

    it("uses default message when response text is empty", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: false,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useProviderMutations());

      await act(async () => {
        try {
          await result.current.createProvider("test", { type: "claude" });
        } catch {
          // expected
        }
      });

      await waitFor(() => {
        expect(result.current.error?.message).toBe("Failed to create provider");
      });
    });

    it("throws when no workspace is selected", async () => {
      mockCurrentWorkspace = null;
      const { result } = renderHook(() => useProviderMutations());

      await expect(
        act(async () => {
          await result.current.createProvider("test", { type: "claude" });
        })
      ).rejects.toThrow("No workspace selected");

      expect(global.fetch).not.toHaveBeenCalled();
    });
  });

  describe("updateProvider", () => {
    it("sends PUT request with correct URL and body", async () => {
      const mockProvider = { metadata: { name: "my-provider" }, spec: { type: "claude", model: "opus" } };
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockProvider),
      });

      const { result } = renderHook(() => useProviderMutations());

      let updated;
      await act(async () => {
        updated = await result.current.updateProvider("my-provider", { type: "claude", model: "opus" });
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/providers/my-provider",
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ spec: { type: "claude", model: "opus" } }),
        }
      );
      expect(updated).toEqual(mockProvider);
    });

    it("sets error on failed update", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: false,
        text: () => Promise.resolve("Not Found"),
      });

      const { result } = renderHook(() => useProviderMutations());

      await act(async () => {
        try {
          await result.current.updateProvider("missing", { type: "claude" });
        } catch {
          // expected
        }
      });

      await waitFor(() => {
        expect(result.current.error?.message).toBe("Not Found");
      });
    });

    it("uses default message when response text is empty", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
        ok: false,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useProviderMutations());

      await act(async () => {
        try {
          await result.current.updateProvider("test", { type: "claude" });
        } catch {
          // expected
        }
      });

      await waitFor(() => {
        expect(result.current.error?.message).toBe("Failed to update provider");
      });
    });

    it("throws when no workspace is selected", async () => {
      mockCurrentWorkspace = null;
      const { result } = renderHook(() => useProviderMutations());

      await expect(
        act(async () => {
          await result.current.updateProvider("test", { type: "claude" });
        })
      ).rejects.toThrow("No workspace selected");

      expect(global.fetch).not.toHaveBeenCalled();
    });
  });
});
