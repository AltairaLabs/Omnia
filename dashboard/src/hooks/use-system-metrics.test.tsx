import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSystemMetrics } from "./use-system-metrics";

// Mock prometheus module
const mockIsPrometheusAvailable = vi.fn().mockResolvedValue(true);
const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusRange = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
  queryPrometheus: (query: string) => mockQueryPrometheus(query),
  queryPrometheusRange: (query: string, from: Date, to: Date, step: string) =>
    mockQueryPrometheusRange(query, from, to, step),
}));

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

describe("useSystemMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Default mock responses for instant queries
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [{ value: [1234567890, "10.5"] }],
      },
    });

    // Default mock responses for range queries
    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [
          {
            values: [
              [1234567890, "8.0"],
              [1234567950, "9.5"],
              [1234568010, "11.0"],
            ],
          },
        ],
      },
    });
  });

  it("should return empty metrics when Prometheus is not available", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(false);

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(false);
    expect(result.current.data?.requestsPerSec.display).toBe("--");
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should fetch metrics when Prometheus is available", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(true);
    expect(mockQueryPrometheus).toHaveBeenCalled();
    expect(mockQueryPrometheusRange).toHaveBeenCalled();
  });

  it("should format request rate correctly", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockResolvedValueOnce({
      status: "success",
      data: {
        result: [{ value: [1234567890, "5.5"] }],
      },
    });

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.requestsPerSec.unit).toBe("req/s");
  });

  it("should handle empty prometheus results", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [],
      },
    });
    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [],
      },
    });

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.requestsPerSec.current).toBe(0);
  });

  it("should handle prometheus error status", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockResolvedValue({
      status: "error",
      error: "Query failed",
    });
    mockQueryPrometheusRange.mockResolvedValue({
      status: "error",
      error: "Query failed",
    });

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.requestsPerSec.current).toBe(0);
  });

  it("should handle prometheus fetch error", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(false);
  });

  it("should aggregate multiple series results", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { value: [1234567890, "5.0"] },
          { value: [1234567890, "3.0"] },
        ],
      },
    });

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // Should sum multiple results
    expect(result.current.data?.requestsPerSec.current).toBe(8.0);
  });

  it("should handle NaN values in results", async () => {
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [{ value: [1234567890, "NaN"] }],
      },
    });

    const { result } = renderHook(() => useSystemMetrics(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.requestsPerSec.current).toBe(0);
  });
});
