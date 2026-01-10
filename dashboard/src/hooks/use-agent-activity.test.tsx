import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAgentActivity } from "./use-agent-activity";

// Mock prometheus module
const mockIsPrometheusAvailable = vi.fn().mockResolvedValue(true);
const mockQueryPrometheusRange = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
  queryPrometheusRange: (query: string, from: Date, to: Date, step: string) =>
    mockQueryPrometheusRange(query, from, to, step),
}));

// Mock useDemoMode
vi.mock("./use-runtime-config", () => ({
  useDemoMode: vi.fn().mockReturnValue({ isDemoMode: false, loading: false }),
}));

import { useDemoMode } from "./use-runtime-config";
const mockUseDemoMode = vi.mocked(useDemoMode);

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useAgentActivity", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });

    // Default mock response
    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [
          {
            values: [
              [1234567890, "100"],
              [1234571490, "150"],
              [1234575090, "200"],
            ],
          },
        ],
      },
    });
  });

  it("should be in loading state while demo mode is loading", () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: true });

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should return mock data in demo mode", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.available).toBe(true);
    expect(result.current.isDemo).toBe(true);
    expect(result.current.data.length).toBeGreaterThan(0);
  });

  it("should return consistent mock data across renders", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result: result1 } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false);
    });

    const firstData = result1.current.data;

    const { result: result2 } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false);
    });

    // Mock data is cached so it should be the same
    expect(result2.current.data.length).toBe(firstData.length);
  });

  it("should fetch data from Prometheus when not in demo mode", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(mockQueryPrometheusRange).toHaveBeenCalled();
    expect(result.current.isDemo).toBe(false);
  });

  it("should return unavailable when Prometheus is not available", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(false);

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.available).toBe(false);
    expect(result.current.data).toEqual([]);
  });

  it("should handle Prometheus error gracefully", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheusRange.mockRejectedValue(new Error("Query failed"));

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.available).toBe(false);
    consoleSpy.mockRestore();
  });

  it("should return activity data with correct structure", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const dataPoint = result.current.data[0];
    expect(dataPoint).toHaveProperty("time");
    expect(dataPoint).toHaveProperty("requests");
    expect(dataPoint).toHaveProperty("sessions");
    expect(typeof dataPoint.time).toBe("string");
    expect(typeof dataPoint.requests).toBe("number");
    expect(typeof dataPoint.sessions).toBe("number");
  });

  it("should process Prometheus results correctly", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);

    mockQueryPrometheusRange.mockImplementation((query: string) => {
      if (query.includes("requests")) {
        return Promise.resolve({
          status: "success",
          data: {
            result: [
              {
                values: [
                  [1234567890, "100"],
                  [1234571490, "200"],
                ],
              },
            ],
          },
        });
      } else {
        return Promise.resolve({
          status: "success",
          data: {
            result: [
              {
                values: [
                  [1234567890, "50"],
                  [1234571490, "75"],
                ],
              },
            ],
          },
        });
      }
    });

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data.length).toBeGreaterThan(0);
  });

  it("should handle empty Prometheus results", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [],
      },
    });

    const { result } = renderHook(() => useAgentActivity(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.available).toBe(true);
    expect(result.current.data).toEqual([]);
  });
});
