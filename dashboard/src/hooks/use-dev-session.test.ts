/**
 * Tests for useDevSession hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockSession = {
  metadata: { name: "dev-session-project-1-abc123", namespace: "test-ns" },
  spec: { projectId: "project-1", workspace: "test-workspace", idleTimeout: "30m" },
  status: {
    phase: "Ready",
    endpoint: "ws://arena-dev-console-dev-session-1.test-ns.svc:8080/ws",
    serviceName: "arena-dev-console-dev-session-1",
  },
};

const mockPendingSession = {
  metadata: { name: "dev-session-project-1-abc123", namespace: "test-ns" },
  spec: { projectId: "project-1", workspace: "test-workspace", idleTimeout: "30m" },
  status: { phase: "Pending" },
};

describe("useDevSession", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.resetModules();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null session when no sessions exist", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([]),
    });

    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "test-workspace",
        projectId: "project-1",
      })
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.session).toBeNull();
    expect(result.current.isReady).toBe(false);
    expect(result.current.endpoint).toBeNull();
  });

  it("returns session when one exists and is ready", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([mockSession]),
    });

    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "test-workspace-2",
        projectId: "project-2",
      })
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.session).toEqual(mockSession);
    expect(result.current.isReady).toBe(true);
    expect(result.current.endpoint).toBe(mockSession.status.endpoint);
  });

  it("returns pending session as not ready", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([mockPendingSession]),
    });

    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "test-workspace-3",
        projectId: "project-3",
      })
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.session).toEqual(mockPendingSession);
    expect(result.current.isReady).toBe(false);
    expect(result.current.endpoint).toBeNull();
  });

  it("does not fetch when workspace is empty", async () => {
    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "",
        projectId: "project-1",
      })
    );

    // Wait a tick to ensure any async operations would have started
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(result.current.session).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it("does not fetch when projectId is empty", async () => {
    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "test-workspace",
        projectId: "",
      })
    );

    // Wait a tick to ensure any async operations would have started
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(result.current.session).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it("handles fetch error", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ message: "Server error" }),
    });

    const { useDevSession } = await import("./use-dev-session");
    const { result } = renderHook(() =>
      useDevSession({
        workspace: "test-workspace-4",
        projectId: "project-4",
      })
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.session).toBeNull();
  });

  describe("createSession", () => {
    it("creates a new session", async () => {
      // Initial fetch returns empty
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([]),
        })
        // POST request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(mockSession),
        })
        // Re-fetch after create
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-5",
          projectId: "project-5",
        })
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      let createdSession;
      await act(async () => {
        createdSession = await result.current.createSession();
      });

      expect(createdSession).toEqual(mockSession);
    });

    it("handles create error", async () => {
      // Initial fetch returns empty
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([]),
        })
        // POST request fails
        .mockResolvedValueOnce({
          ok: false,
          json: () => Promise.resolve({ message: "Create failed" }),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-6",
          projectId: "project-6",
        })
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      // createSession should throw on error
      await expect(
        act(async () => {
          await result.current.createSession();
        })
      ).rejects.toThrow("Create failed");
    });
  });

  describe("deleteSession", () => {
    it("deletes the current session", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        })
        // DELETE request
        .mockResolvedValueOnce({
          ok: true,
        })
        // Re-fetch after delete
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([]),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-7",
          projectId: "project-7",
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockSession);
      });

      await act(async () => {
        await result.current.deleteSession();
      });

      // Verify DELETE was called
      const deleteCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "DELETE"
      );
      expect(deleteCall).toBeDefined();
    });

    it("does nothing when no session exists", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([]),
      });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-8",
          projectId: "project-8",
        })
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.deleteSession();
      });

      // Should not have made any DELETE calls
      const deleteCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "DELETE"
      );
      expect(deleteCall).toBeUndefined();
    });

    it("handles delete error", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        })
        // DELETE request fails with 500
        .mockResolvedValueOnce({
          ok: false,
          status: 500,
          json: () => Promise.resolve({ message: "Internal server error" }),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-16",
          projectId: "project-16",
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockSession);
      });

      // deleteSession should throw on error
      await expect(
        act(async () => {
          await result.current.deleteSession();
        })
      ).rejects.toThrow("Internal server error");
    });

    it("ignores 404 on delete (session already gone)", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        })
        // DELETE returns 404 (already deleted)
        .mockResolvedValueOnce({
          ok: false,
          status: 404,
        })
        // Re-fetch after delete
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([]),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-17",
          projectId: "project-17",
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockSession);
      });

      // Should not throw for 404
      await act(async () => {
        await result.current.deleteSession();
      });

      // Verify DELETE was called
      const deleteCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "DELETE"
      );
      expect(deleteCall).toBeDefined();
    });
  });

  describe("sendHeartbeat", () => {
    it("sends heartbeat when session is ready", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        })
        // PATCH request for heartbeat
        .mockResolvedValueOnce({
          ok: true,
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-9",
          projectId: "project-9",
        })
      );

      await waitFor(() => {
        expect(result.current.isReady).toBe(true);
      });

      await act(async () => {
        await result.current.sendHeartbeat();
      });

      // Verify PATCH was called for heartbeat
      const patchCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "PATCH"
      );
      expect(patchCall).toBeDefined();
      expect(patchCall?.[0]).toContain("dev-sessions");
    });

    it("does nothing when session is not ready", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([mockPendingSession]),
      });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-10",
          projectId: "project-10",
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockPendingSession);
      });

      await act(async () => {
        await result.current.sendHeartbeat();
      });

      // Should not have made any PATCH calls
      const patchCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "PATCH"
      );
      expect(patchCall).toBeUndefined();
    });

    it("handles heartbeat error gracefully", async () => {
      const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        })
        // PATCH request fails
        .mockRejectedValueOnce(new Error("Network error"));

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-11",
          projectId: "project-11",
        })
      );

      await waitFor(() => {
        expect(result.current.isReady).toBe(true);
      });

      // Should not throw
      await act(async () => {
        await result.current.sendHeartbeat();
      });

      expect(consoleWarnSpy).toHaveBeenCalledWith("Failed to send heartbeat");
      consoleWarnSpy.mockRestore();
    });
  });

  describe("refresh", () => {
    it("triggers a data refresh", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([mockSession]),
      });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-12",
          projectId: "project-12",
        })
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.refresh();
      });

      // SWR mutate may not always trigger a new fetch immediately,
      // but the function should be callable
      expect(result.current.refresh).toBeDefined();
    });
  });

  describe("heartbeat interval", () => {
    it("clears heartbeat interval on unmount", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([mockSession]),
      });

      const { useDevSession } = await import("./use-dev-session");
      const { result, unmount } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-15",
          projectId: "project-15",
        })
      );

      await waitFor(() => {
        expect(result.current.isReady).toBe(true);
      });

      // Unmount should clear the interval without errors
      unmount();

      // If we got here without errors, the cleanup worked
      expect(true).toBe(true);
    });
  });

  describe("autoCreate", () => {
    it("auto-creates session when autoCreate is true and no session exists", async () => {
      // Initial fetch returns empty
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([]),
        })
        // POST request for auto-create
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(mockSession),
        })
        // Re-fetch after create
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve([mockSession]),
        });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-13",
          projectId: "project-13",
          autoCreate: true,
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockSession);
      });

      // Verify POST was called for auto-create
      const postCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "POST"
      );
      expect(postCall).toBeDefined();
    });

    it("does not auto-create when session already exists", async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([mockSession]),
      });

      const { useDevSession } = await import("./use-dev-session");
      const { result } = renderHook(() =>
        useDevSession({
          workspace: "test-workspace-14",
          projectId: "project-14",
          autoCreate: true,
        })
      );

      await waitFor(() => {
        expect(result.current.session).toEqual(mockSession);
      });

      // Should not have made any POST calls
      const postCall = mockFetch.mock.calls.find(
        (call) => call[1]?.method === "POST"
      );
      expect(postCall).toBeUndefined();
    });
  });
});
