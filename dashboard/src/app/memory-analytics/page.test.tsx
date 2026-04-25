import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import MemoryAnalyticsPage from "./page";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "default" },
    workspaces: [],
    setCurrentWorkspace: vi.fn(),
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url.includes("groupBy=tier&metric=distinct_users")) {
      return {
        ok: true,
        json: async () => [{ key: "user", value: 17, count: 17 }],
      };
    }
    if (url.includes("groupBy=tier")) {
      return {
        ok: true,
        json: async () => [
          { key: "institutional", value: 10, count: 10 },
          { key: "agent", value: 20, count: 20 },
          { key: "user", value: 70, count: 70 },
        ],
      };
    }
    if (url.includes("groupBy=category")) {
      return {
        ok: true,
        json: async () => [{ key: "memory:context", value: 80, count: 80 }],
      };
    }
    if (url.includes("groupBy=day")) {
      return {
        ok: true,
        json: async () => [{ key: "2026-04-25", value: 5, count: 5 }],
      };
    }
    if (url.includes("groupBy=agent")) {
      return {
        ok: true,
        json: async () => [{ key: "support-agent", value: 50, count: 50 }],
      };
    }
    if (url.includes("/privacy/consent/stats")) {
      return {
        ok: true,
        json: async () => ({
          totalUsers: 100,
          optedOutAll: 5,
          grantsByCategory: { "memory:context": 90 },
        }),
      };
    }
    throw new Error(`unexpected url: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
});

describe("MemoryAnalyticsPage", () => {
  it("renders the page title", async () => {
    render(<MemoryAnalyticsPage />, { wrapper });
    expect(screen.getByText(/Memory analytics$/)).toBeInTheDocument();
  });

  it("renders all panels with data", async () => {
    render(<MemoryAnalyticsPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getAllByText("Institutional").length).toBeGreaterThan(0);
    });
    expect(screen.getByText(/Memory by category/i)).toBeInTheDocument();
    expect(screen.getByText(/Growth over time/i)).toBeInTheDocument();
    expect(screen.getByText(/Memory by agent/i)).toBeInTheDocument();
    expect(screen.getByText(/Privacy posture/i)).toBeInTheDocument();
    expect(screen.getByText(/How memory is organized/i)).toBeInTheDocument();
  });
});
