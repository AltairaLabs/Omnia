import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: vi.fn(),
}));

import { queryPrometheus } from "@/lib/prometheus";
import { useConsolidationStats } from "./use-consolidation-stats";

const mocked = vi.mocked(queryPrometheus);

function wrap({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => mocked.mockReset());

describe("useConsolidationStats", () => {
  it("returns passes + actions counts from Prometheus", async () => {
    mocked.mockImplementation(async (q?: string) => {
      const query = q ?? "";
      if (query.includes("passes_total")) {
        return {
          status: "success" as const,
          data: { resultType: "vector" as const, result: [{ metric: {}, value: [0, "12"] }] },
        };
      }
      if (query.includes("by (action)")) {
        return {
          status: "success" as const,
          data: {
            resultType: "vector" as const,
            result: [
              { metric: { action: "create_summary" }, value: [0, "30"] },
              { metric: { action: "supersede" }, value: [0, "12"] },
            ],
          },
        };
      }
      return {
        status: "success" as const,
        data: { resultType: "vector" as const, result: [{ metric: {}, value: [0, "47"] }] },
      };
    });

    const { result } = renderHook(() => useConsolidationStats({ rangeDays: 7 }), {
      wrapper: wrap,
    });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.passesTotal).toBe(12);
    expect(result.current.data?.actionsTotal).toBe(47);
    expect(result.current.data?.actionsByType.create_summary).toBe(30);
    expect(result.current.data?.actionsByType.supersede).toBe(12);
  });

  it("treats unparseable values as zero", async () => {
    mocked.mockResolvedValue({
      status: "success" as const,
      data: { resultType: "vector" as const, result: [{ metric: {}, value: [0, "NaN"] }] },
    });
    const { result } = renderHook(() => useConsolidationStats({ rangeDays: 7 }), {
      wrapper: wrap,
    });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.passesTotal).toBe(0);
  });
});
