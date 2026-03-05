import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { DebugPanel } from "./debug-panel";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { Session, Message } from "@/types/session";

const mockMessages: Message[] = [
  { id: "m1", role: "user", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
  { id: "m2", role: "assistant", content: '{"name":"search","arguments":{"q":"test"}}', timestamp: "2024-01-01T00:00:02Z", metadata: { type: "tool_call", duration_ms: "100", status: "success" }, toolCallId: "tc1" },
  { id: "m3", role: "assistant", content: "Hi!", timestamp: "2024-01-01T00:00:03Z" },
];

const mockSession: Session = {
  id: "s1",
  agentName: "agent-1",
  agentNamespace: "default",
  status: "completed",
  startedAt: "2024-01-01T00:00:00Z",
  messages: mockMessages,
  metrics: {
    messageCount: 2,
    toolCallCount: 1,
    totalTokens: 200,
    inputTokens: 100,
    outputTokens: 100,
  },
};

describe("DebugPanel", () => {
  beforeEach(() => {
    useDebugPanelStore.setState({
      isOpen: false,
      activeTab: "timeline",
      height: 30,
      selectedToolCallId: null,
    });
  });

  it("renders collapsed bar when closed", () => {
    render(<DebugPanel messages={mockMessages} session={mockSession} />);
    expect(screen.getByTestId("debug-panel-collapsed")).toBeInTheDocument();
    expect(screen.getByText("Debug: Timeline, Tool Calls, Raw")).toBeInTheDocument();
  });

  it("expands when expand button is clicked", () => {
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    fireEvent.click(screen.getByTestId("debug-panel-expand"));

    expect(useDebugPanelStore.getState().isOpen).toBe(true);
  });

  it("renders expanded panel with tabs when open", () => {
    useDebugPanelStore.setState({ isOpen: true });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    expect(screen.getByTestId("debug-panel")).toBeInTheDocument();
    expect(screen.getByTestId("debug-panel-tabs")).toBeInTheDocument();
  });

  it("shows timeline tab content by default", () => {
    useDebugPanelStore.setState({ isOpen: true, activeTab: "timeline" });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    expect(screen.getByText("User message")).toBeInTheDocument();
  });

  it("shows tool calls tab content when selected", () => {
    useDebugPanelStore.setState({ isOpen: true, activeTab: "toolcalls" });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    expect(screen.getByTestId("toolcalls-tab")).toBeInTheDocument();
  });

  it("shows raw tab content when selected", () => {
    useDebugPanelStore.setState({ isOpen: true, activeTab: "raw" });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    expect(screen.getByTestId("raw-tab")).toBeInTheDocument();
  });

  it("minimizes panel when minimize button is clicked", () => {
    useDebugPanelStore.setState({ isOpen: true });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    fireEvent.click(screen.getByTestId("debug-panel-minimize"));
    expect(useDebugPanelStore.getState().isOpen).toBe(false);
  });

  it("closes panel when close button is clicked", () => {
    useDebugPanelStore.setState({ isOpen: true });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    fireEvent.click(screen.getByTestId("debug-panel-close"));
    expect(useDebugPanelStore.getState().isOpen).toBe(false);
  });

  it("displays tool call count badge", () => {
    useDebugPanelStore.setState({ isOpen: true });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("switches tabs when clicking different tab", () => {
    useDebugPanelStore.setState({ isOpen: true, activeTab: "timeline" });
    render(<DebugPanel messages={mockMessages} session={mockSession} />);

    fireEvent.click(screen.getByTestId("debug-tab-raw"));
    expect(useDebugPanelStore.getState().activeTab).toBe("raw");
  });
});
