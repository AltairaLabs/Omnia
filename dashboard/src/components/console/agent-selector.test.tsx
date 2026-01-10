import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentSelector } from "./agent-selector";

// Mock the hooks
const mockNamespaces = ["production", "staging", "development"];
const mockAgents = [
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
];

vi.mock("@/hooks/use-namespaces", () => ({
  useNamespaces: () => ({
    data: mockNamespaces,
    isLoading: false,
  }),
}));

vi.mock("@/hooks/use-agents", () => ({
  useAgents: () => ({
    data: mockAgents,
    isLoading: false,
  }),
}));

// Mock the Select component to allow testing interactions
vi.mock("@/components/ui/select", () => ({
  Select: ({ children, value, onValueChange, disabled }: {
    children: React.ReactNode;
    value?: string;
    onValueChange?: (value: string) => void;
    disabled?: boolean;
  }) => (
    <div data-testid="mock-select" data-disabled={disabled}>
      <select
        value={value}
        onChange={(e) => onValueChange?.(e.target.value)}
        disabled={disabled}
        data-testid="select-native"
      >
        {children}
      </select>
    </div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>,
  SelectContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SelectItem: ({ children, value }: { children: React.ReactNode; value: string }) => (
    <option value={value}>{children}</option>
  ),
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

    it("should render the start button disabled initially", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      expect(startButton).toBeDisabled();
    });

    it("should apply custom className", () => {
      const { container } = render(
        <AgentSelector onSelect={mockOnSelect} className="custom-class" />,
        { wrapper: TestWrapper }
      );

      expect(container.firstChild).toHaveClass("custom-class");
    });

    it("should render the bot icon", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const icon = document.querySelector(".lucide-bot");
      expect(icon).toBeInTheDocument();
    });
  });

  describe("namespace selection", () => {
    it("should show 'All namespaces' as default value", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      // The namespace select should have __all__ as default
      const selects = screen.getAllByTestId("select-native");
      expect(selects[0]).toHaveValue("__all__");
    });

    it("should update namespace when changed", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");
      fireEvent.change(selects[0], { target: { value: "production" } });

      expect(selects[0]).toHaveValue("production");
    });

    it("should clear agent selection when namespace changes to all namespaces", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");

      // Select production namespace first
      fireEvent.change(selects[0], { target: { value: "production" } });

      // Select an agent
      fireEvent.change(selects[1], { target: { value: "customer-support" } });

      // Start button should be enabled
      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      expect(startButton).not.toBeDisabled();

      // Change namespace to all - start button should become disabled
      fireEvent.change(selects[0], { target: { value: "__all__" } });

      // The button becomes disabled because agent selection is reset
      expect(startButton).toBeDisabled();
    });
  });

  describe("agent selection", () => {
    it("should update agent when changed", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");
      fireEvent.change(selects[1], { target: { value: "customer-support" } });

      expect(selects[1]).toHaveValue("customer-support");
    });

    it("should enable start button when agent is selected", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");
      fireEvent.change(selects[1], { target: { value: "customer-support" } });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      expect(startButton).not.toBeDisabled();
    });
  });

  describe("start conversation", () => {
    it("should call onSelect with namespace and agent when clicking start", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");
      fireEvent.change(selects[1], { target: { value: "customer-support" } });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      fireEvent.click(startButton);

      expect(mockOnSelect).toHaveBeenCalledWith("production", "customer-support");
    });

    it("should use filtered namespace when agent is selected from specific namespace", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");

      // Select staging namespace
      fireEvent.change(selects[0], { target: { value: "staging" } });

      // Select data-analyst (which is in staging)
      fireEvent.change(selects[1], { target: { value: "data-analyst" } });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      fireEvent.click(startButton);

      expect(mockOnSelect).toHaveBeenCalledWith("staging", "data-analyst");
    });

    it("should not call onSelect when no agent is selected", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const startButton = screen.getByRole("button", {
        name: /start conversation/i,
      });
      fireEvent.click(startButton);

      expect(mockOnSelect).not.toHaveBeenCalled();
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

    it("should have descriptive heading", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const heading = screen.getByRole("heading", { level: 2 });
      expect(heading).toHaveTextContent("Start a Conversation");
    });
  });

  describe("empty state", () => {
    it("should not show empty message when agents are available", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      expect(screen.queryByText(/no running agents/i)).not.toBeInTheDocument();
    });
  });

  describe("loading state", () => {
    it("should be interactive when not loading", () => {
      render(<AgentSelector onSelect={mockOnSelect} />, {
        wrapper: TestWrapper,
      });

      const selects = screen.getAllByTestId("select-native");
      selects.forEach((select) => {
        expect(select).not.toBeDisabled();
      });
    });
  });
});
