import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { useWorkspaceCosts, type WorkspaceCostData } from "./use-workspace-costs";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Create wrapper with QueryClientProvider
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe("useWorkspaceCosts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch workspace costs successfully", async () => {
    const mockCostData: WorkspaceCostData = {
      available: true,
      summary: {
        totalCost: 100.5,
        totalInputCost: 40.2,
        totalOutputCost: 60.3,
        totalCacheSavings: 5.5,
        totalRequests: 1000,
        totalTokens: 50000,
        inputTokens: 20000,
        outputTokens: 30000,
        projectedMonthlyCost: 3015,
        inputPercent: 40,
        outputPercent: 60,
      },
      byAgent: [],
      byProvider: [],
      byModel: [],
      timeSeries: [],
      budget: {
        dailyBudget: "200.00",
        monthlyBudget: "5000.00",
        dailyUsedPercent: 50.25,
        monthlyUsedPercent: 60.3,
      },
    };

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => mockCostData,
    });

    const { result } = renderHook(() => useWorkspaceCosts("test-workspace"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockCostData);
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/costs");
  });

  it("should handle API errors", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: async () => ({ message: "Workspace not found" }),
    });

    const { result } = renderHook(() => useWorkspaceCosts("nonexistent"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Workspace not found");
  });

  it("should not fetch when workspace name is null", () => {
    const { result } = renderHook(() => useWorkspaceCosts(null), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("should not fetch when workspace name is undefined", () => {
    const { result } = renderHook(() => useWorkspaceCosts(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("should not fetch when enabled is false", () => {
    const { result } = renderHook(
      () => useWorkspaceCosts("test-workspace", { enabled: false }),
      { wrapper: createWrapper() }
    );

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("should encode workspace name in URL", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        available: true,
        summary: {
          totalCost: 0,
          totalInputCost: 0,
          totalOutputCost: 0,
          totalCacheSavings: 0,
          totalRequests: 0,
          totalTokens: 0,
          inputTokens: 0,
          outputTokens: 0,
          projectedMonthlyCost: 0,
          inputPercent: 0,
          outputPercent: 0,
        },
        byAgent: [],
        byProvider: [],
        byModel: [],
        timeSeries: [],
      }),
    });

    renderHook(() => useWorkspaceCosts("workspace with spaces"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(mockFetch).toHaveBeenCalled());

    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/workspace%20with%20spaces/costs");
  });

  it("should handle unavailable cost data", async () => {
    const mockUnavailableData: WorkspaceCostData = {
      available: false,
      reason: "Prometheus not configured",
      summary: {
        totalCost: 0,
        totalInputCost: 0,
        totalOutputCost: 0,
        totalCacheSavings: 0,
        totalRequests: 0,
        totalTokens: 0,
        inputTokens: 0,
        outputTokens: 0,
        projectedMonthlyCost: 0,
        inputPercent: 0,
        outputPercent: 0,
      },
      byAgent: [],
      byProvider: [],
      byModel: [],
      timeSeries: [],
    };

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => mockUnavailableData,
    });

    const { result } = renderHook(() => useWorkspaceCosts("test-workspace"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.available).toBe(false);
    expect(result.current.data?.reason).toBe("Prometheus not configured");
  });
});
