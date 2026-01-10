import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentSelector } from "./agent-selector";

// Mock the hooks
vi.mock("@/hooks/use-namespaces", () => ({
  useNamespaces: () => ({
    data: ["production", "staging", "development"],
    isLoading: false,
  }),
}));

vi.mock("@/hooks/use-agents", () => ({
  useAgents: () => ({
    data: [
      {
        metadata: {
          name: "customer-support",
          namespace: "production",
          uid: "uid-1",
        },
        status: { phase: "Running" },
      },
      {
        metadata: {
          name: "code-assistant",
          namespace: "production",
          uid: "uid-2",
        },
        status: { phase: "Running" },
      },
      {
        metadata: {
          name: "data-analyst",
          namespace: "staging",
          uid: "uid-3",
        },
        status: { phase: "Running" },
      },
    ],
    isLoading: false,
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

describe("AgentSelector", () => {
  const mockOnSelect = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("rendering", () => {
    it("should render the title and description", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      expect(screen.getByText("Start a Conversation")).toBeInTheDocument();
      expect(
        screen.getByText("Select an agent to begin chatting")
      ).toBeInTheDocument();
    });

    it("should render namespace and agent labels", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      expect(screen.getByText("Namespace")).toBeInTheDocument();
      expect(screen.getByText("Agent")).toBeInTheDocument();
    });

    it("should render the start button", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      expect(startButton).toBeInTheDocument();
    });

    it("should render select components", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      // Should have two comboboxes (namespace and agent)
      const comboboxes = screen.getAllByRole("combobox");
      expect(comboboxes).toHaveLength(2);
    });
  });

  describe("accessibility", () => {
    it("should have accessible labels", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      expect(screen.getByText("Namespace")).toBeInTheDocument();
      expect(screen.getByText("Agent")).toBeInTheDocument();
    });
  });
});
