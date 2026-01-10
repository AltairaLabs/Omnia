import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConsoleTabs } from "./console-tabs";

// Mock ResizeObserver
class MockResizeObserver {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
global.ResizeObserver = MockResizeObserver;

// Mock crypto.randomUUID
vi.stubGlobal("crypto", {
  randomUUID: () => "12345678-1234-1234-1234-123456789012",
});

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();
Object.defineProperty(window, "localStorage", { value: localStorageMock });

// Mock the hooks
vi.mock("@/hooks/use-namespaces", () => ({
  useNamespaces: () => ({
    data: ["production", "staging"],
    isLoading: false,
  }),
}));

vi.mock("@/hooks/use-agents", () => ({
  useAgents: () => ({
    data: [
      {
        metadata: {
          name: "test-agent",
          namespace: "production",
          uid: "uid-1",
        },
        status: { phase: "Running" },
      },
    ],
    isLoading: false,
  }),
}));

// Mock useDataService
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    createAgentConnection: () => ({
      connect: vi.fn(),
      disconnect: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
      onStatusChange: vi.fn(),
    }),
  }),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("ConsoleTabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.clear();
  });

  describe("rendering", () => {
    it("should render the new tab button", () => {
      render(<ConsoleTabs />, { wrapper: TestWrapper });

      expect(screen.getByRole("button", { name: /new tab/i })).toBeInTheDocument();
    });

    it("should render with tab bar", () => {
      render(<ConsoleTabs />, { wrapper: TestWrapper });

      // The component should render without errors
      expect(screen.getByRole("button", { name: /new tab/i })).toBeInTheDocument();
    });
  });

  describe("initial state", () => {
    it("should create an initial tab on mount", () => {
      render(<ConsoleTabs />, { wrapper: TestWrapper });

      // The AgentSelector should be visible for new tabs
      expect(screen.getByText("Start a Conversation")).toBeInTheDocument();
    });
  });
});
