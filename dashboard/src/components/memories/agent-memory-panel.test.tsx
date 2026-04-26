import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

const { mockUseAgentMemories } = vi.hoisted(() => ({
  mockUseAgentMemories: vi.fn(),
}));

vi.mock("@/hooks/use-agent-memories", () => ({
  useAgentMemories: mockUseAgentMemories,
}));

import { AgentMemoryPanel } from "./agent-memory-panel";

describe("AgentMemoryPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows skeleton while loading", () => {
    mockUseAgentMemories.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    render(<AgentMemoryPanel agentId="support" />);
    expect(screen.queryByTestId("agent-memory-empty")).not.toBeInTheDocument();
    expect(screen.queryByTestId("agent-memory-list")).not.toBeInTheDocument();
  });

  it("shows empty state when no memories", () => {
    mockUseAgentMemories.mockReturnValue({
      data: { memories: [], total: 0 },
      isLoading: false,
      error: null,
    });
    render(<AgentMemoryPanel agentId="support" />);
    expect(screen.getByTestId("agent-memory-empty")).toBeInTheDocument();
  });

  it("shows error alert on failure", () => {
    mockUseAgentMemories.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("backend offline"),
    });
    render(<AgentMemoryPanel agentId="support" />);
    expect(screen.getByTestId("agent-memory-error")).toBeInTheDocument();
    expect(screen.getByText("backend offline")).toBeInTheDocument();
  });

  it("renders rows with tier + category badges", () => {
    mockUseAgentMemories.mockReturnValue({
      data: {
        memories: [
          {
            id: "a-1",
            type: "pattern",
            content: "Customers reporting slow API → check p99 latency.",
            confidence: 0.88,
            scope: { workspace_id: "w", agent_id: "support" },
            createdAt: "2026-04-01T00:00:00Z",
            tier: "agent",
            metadata: { consent_category: "memory:context" },
          },
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    });
    render(<AgentMemoryPanel agentId="support" />);
    expect(
      screen.getByText("Customers reporting slow API → check p99 latency."),
    ).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("88% confidence")).toBeInTheDocument();
  });
});
