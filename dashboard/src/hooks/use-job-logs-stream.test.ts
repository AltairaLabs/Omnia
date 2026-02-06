import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import {
  useJobLogsStream,
  parseLogLevel,
  formatLogTimestamp,
} from "./use-job-logs-stream";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: { name: "test-workspace" },
  })),
}));

describe("useJobLogsStream", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("should return initial state when no job name", () => {
    const { result } = renderHook(() => useJobLogsStream(null));

    expect(result.current.logs).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.streaming).toBe(false);
  });

  it("should have start, stop, clear, and refresh functions", () => {
    const { result } = renderHook(() => useJobLogsStream(null));

    expect(typeof result.current.start).toBe("function");
    expect(typeof result.current.stop).toBe("function");
    expect(typeof result.current.clear).toBe("function");
    expect(typeof result.current.refresh).toBe("function");
  });

  it("should auto-start streaming when job name is provided", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          logs: [
            { timestamp: "2024-01-01T10:00:00Z", message: "Log 1" },
          ],
        }),
    });
    vi.stubGlobal("fetch", mockFetch);

    renderHook(() => useJobLogsStream("test-job"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });
  });

  it("should fetch logs with correct URL parameters", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ logs: [] }),
    });
    vi.stubGlobal("fetch", mockFetch);

    renderHook(() =>
      useJobLogsStream("test-job", {
        tailLines: 50,
        sinceSeconds: 1800,
      })
    );

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/workspaces/test-workspace/arena/jobs/test-job/logs")
      );
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("tailLines=50")
      );
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("sinceSeconds=1800")
      );
    });
  });

  it("should handle fetch errors", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      statusText: "Internal Server Error",
    });
    vi.stubGlobal("fetch", mockFetch);

    const { result } = renderHook(() => useJobLogsStream("test-job"));

    await waitFor(() => {
      expect(result.current.error).not.toBeNull();
      expect(result.current.error?.message).toContain("Failed to fetch logs");
    });
  });

  it("should deduplicate logs by timestamp and message", async () => {
    const logs = [
      { timestamp: "2024-01-01T10:00:00Z", message: "Log 1" },
      { timestamp: "2024-01-01T10:00:01Z", message: "Log 2" },
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ logs }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const { result } = renderHook(() => useJobLogsStream("test-job"));

    await waitFor(() => {
      expect(result.current.logs.length).toBe(2);
    });

    // Simulate another fetch with same logs
    act(() => {
      result.current.refresh();
    });

    await waitFor(() => {
      // Should still be 2 due to deduplication
      expect(result.current.logs.length).toBe(2);
    });
  });

  it("should clear logs when clear is called", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          logs: [{ timestamp: "2024-01-01T10:00:00Z", message: "Log 1" }],
        }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const { result } = renderHook(() => useJobLogsStream("test-job"));

    await waitFor(() => {
      expect(result.current.logs.length).toBe(1);
    });

    act(() => {
      result.current.clear();
    });

    expect(result.current.logs).toEqual([]);
  });

  it("should stop streaming when stop is called", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ logs: [] }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const { result } = renderHook(() => useJobLogsStream("test-job"));

    await waitFor(() => {
      expect(result.current.streaming).toBe(true);
    });

    act(() => {
      result.current.stop();
    });

    expect(result.current.streaming).toBe(false);
  });

  it("should trim logs to maxLines", async () => {
    const logs = Array.from({ length: 10 }, (_, i) => ({
      timestamp: `2024-01-01T10:00:${String(i).padStart(2, "0")}Z`,
      message: `Log ${i}`,
    }));

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ logs }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const { result } = renderHook(() =>
      useJobLogsStream("test-job", { maxLines: 5 })
    );

    await waitFor(() => {
      expect(result.current.logs.length).toBe(5);
    });
  });
});

describe("parseLogLevel", () => {
  it("should return error for messages containing error", () => {
    expect(parseLogLevel("ERROR: something failed")).toBe("error");
    expect(parseLogLevel("error occurred")).toBe("error");
  });

  it("should return error for messages containing fatal", () => {
    expect(parseLogLevel("FATAL: critical error")).toBe("error");
  });

  it("should return warn for messages containing warn", () => {
    expect(parseLogLevel("WARNING: check this")).toBe("warn");
    expect(parseLogLevel("warn: deprecated")).toBe("warn");
  });

  it("should return debug for messages containing debug or trace", () => {
    expect(parseLogLevel("DEBUG: value is 5")).toBe("debug");
    expect(parseLogLevel("TRACE: entering function")).toBe("debug");
  });

  it("should return info for other messages", () => {
    expect(parseLogLevel("Server started")).toBe("info");
    expect(parseLogLevel("Processing request")).toBe("info");
  });
});

describe("formatLogTimestamp", () => {
  it("should format valid timestamp", () => {
    const timestamp = "2024-01-15T10:30:45.123Z";
    const formatted = formatLogTimestamp(timestamp);
    // The format includes hours, minutes, seconds, and fractional seconds
    expect(formatted).toMatch(/\d{2}:\d{2}:\d{2}/);
  });

  it("should return original timestamp for invalid input", () => {
    const invalidTimestamp = "not-a-date";
    const formatted = formatLogTimestamp(invalidTimestamp);
    expect(formatted).toBe(invalidTimestamp);
  });
});
