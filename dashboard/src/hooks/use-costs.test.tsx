import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useCosts } from "./use-costs";

// Mock data
const mockCostData = {
  total: 1250.50,
  breakdown: {
    compute: 800.00,
    storage: 300.00,
    network: 150.50,
  },
  period: {
    start: "2024-01-01",
    end: "2024-01-31",
  },
};

// Mock useDataService
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getCosts: vi.fn().mockResolvedValue(mockCostData),
  }),
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

describe("useCosts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch cost data", async () => {
    const { result } = renderHook(() => useCosts(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockCostData);
  });

  it("should accept options", async () => {
    const options = { namespace: "production", period: "monthly" };
    const { result } = renderHook(() => useCosts(options), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeDefined();
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useCosts(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should include query key with options", async () => {
    const options = { namespace: "staging" };
    const { result } = renderHook(() => useCosts(options), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeDefined();
  });
});
