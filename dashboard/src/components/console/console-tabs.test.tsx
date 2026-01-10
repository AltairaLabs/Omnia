import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Mock crypto.randomUUID to return predictable values
let uuidCounter = 0;
vi.stubGlobal("crypto", {
  randomUUID: () => `test-uuid-${++uuidCounter}`,
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
      {
        metadata: {
          name: "staging-agent",
          namespace: "staging",
          uid: "uid-2",
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
    uuidCounter = 0;
    // Reset modules to clear zustand store state
    vi.resetModules();
  });

  describe("rendering", () => {
    it("should render the new tab button", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      expect(screen.getByRole("button", { name: /new tab/i })).toBeInTheDocument();
    });

    it("should render with tab bar", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      expect(screen.getByRole("button", { name: /new tab/i })).toBeInTheDocument();
    });

    it("should render within a container with proper styling", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      const { container } = render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      const mainDiv = container.firstChild as HTMLElement;
      expect(mainDiv).toHaveClass("flex", "flex-col", "h-full");
    });
  });

  describe("initial state", () => {
    it("should create an initial tab on mount", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("Start a Conversation")).toBeInTheDocument();
      });
    });

    it("should display 'New Session' for selecting state tabs", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });
    });

    it("should show the agent selector when in selecting state", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("Select an agent to begin chatting")).toBeInTheDocument();
      });
    });
  });

  describe("tab creation", () => {
    it("should create a new tab when clicking the new tab button", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      const newTabButton = screen.getByRole("button", { name: /new tab/i });
      fireEvent.click(newTabButton);

      await waitFor(() => {
        const sessions = screen.getAllByText("New Session");
        expect(sessions.length).toBeGreaterThanOrEqual(2);
      });
    });

    it("should persist tabs to localStorage", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      expect(localStorageMock.setItem).toHaveBeenCalled();
    });
  });

  describe("tab switching", () => {
    it("should allow clicking on tabs to switch", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      // Create second tab
      const newTabButton = screen.getByRole("button", { name: /new tab/i });
      fireEvent.click(newTabButton);

      await waitFor(() => {
        const sessions = screen.getAllByText("New Session");
        expect(sessions.length).toBeGreaterThanOrEqual(2);
      });

      // Click first tab
      const tabs = screen.getAllByText("New Session");
      fireEvent.click(tabs[0]);

      // Both tabs show same content
      expect(screen.getByText("Start a Conversation")).toBeInTheDocument();
    });
  });

  describe("tab closing", () => {
    it("should have close buttons with correct accessibility labels", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      const closeButton = screen.getByRole("button", { name: /close new session/i });
      expect(closeButton).toBeInTheDocument();
    });

    it("should close tab when clicking close button", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      // Create second tab
      const newTabButton = screen.getByRole("button", { name: /new tab/i });
      fireEvent.click(newTabButton);

      await waitFor(() => {
        const sessions = screen.getAllByText("New Session");
        expect(sessions.length).toBeGreaterThanOrEqual(2);
      });

      // Close the first tab
      const closeButtons = screen.getAllByRole("button", { name: /close/i });
      fireEvent.click(closeButtons[0]);

      await waitFor(() => {
        const sessions = screen.getAllByText("New Session");
        expect(sessions.length).toBe(1);
      });
    });
  });

  describe("agent selection", () => {
    it("should show namespace and agent labels", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("Namespace")).toBeInTheDocument();
        expect(screen.getByText("Agent")).toBeInTheDocument();
      });
    });

    it("should show start conversation button", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /start conversation/i })).toBeInTheDocument();
      });
    });

    it("should have start button disabled initially", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        const startButton = screen.getByRole("button", { name: /start conversation/i });
        expect(startButton).toBeDisabled();
      });
    });
  });

  describe("accessibility", () => {
    it("should have accessible new tab button", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      const newTabButton = screen.getByRole("button", { name: /new tab/i });
      expect(newTabButton).toBeInTheDocument();
    });

    it("should have accessible close buttons", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        expect(screen.getByText("New Session")).toBeInTheDocument();
      });

      const closeButton = screen.getByRole("button", { name: /close new session/i });
      expect(closeButton).toBeInTheDocument();
    });

    it("should have comboboxes for selection", async () => {
      const { ConsoleTabs: FreshConsoleTabs } = await import("./console-tabs");
      render(<FreshConsoleTabs />, { wrapper: TestWrapper });

      await waitFor(() => {
        const comboboxes = screen.getAllByRole("combobox");
        expect(comboboxes).toHaveLength(2);
      });
    });
  });
});
