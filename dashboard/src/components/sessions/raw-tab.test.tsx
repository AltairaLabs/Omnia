import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RawTab } from "./raw-tab";
import type { Session } from "@/types/session";

const mockSession: Session = {
  id: "s1",
  agentName: "agent-1",
  agentNamespace: "default",
  status: "completed",
  startedAt: "2024-01-01T00:00:00Z",
  messages: [
    { id: "m1", role: "user", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
  ],
  metrics: {
    messageCount: 1,
    toolCallCount: 0,
    totalTokens: 100,
    inputTokens: 50,
    outputTokens: 50,
  },
};

describe("RawTab", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("renders session JSON", () => {
    render(<RawTab session={mockSession} />);

    const pre = screen.getByTestId("raw-json");
    expect(pre.textContent).toContain('"id": "s1"');
    expect(pre.textContent).toContain('"agentName": "agent-1"');
  });

  it("renders copy button", () => {
    render(<RawTab session={mockSession} />);
    expect(screen.getByTestId("raw-copy-button")).toBeInTheDocument();
    expect(screen.getByText("Copy")).toBeInTheDocument();
  });

  it("copies JSON to clipboard on button click", () => {
    render(<RawTab session={mockSession} />);

    fireEvent.click(screen.getByTestId("raw-copy-button"));

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      JSON.stringify(mockSession, null, 2)
    );
  });

  it("shows 'Copied' after clicking copy", () => {
    render(<RawTab session={mockSession} />);

    fireEvent.click(screen.getByTestId("raw-copy-button"));

    expect(screen.getByText("Copied")).toBeInTheDocument();
  });
});
