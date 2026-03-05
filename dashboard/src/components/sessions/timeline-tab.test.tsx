import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TimelineTab } from "./timeline-tab";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { Message } from "@/types/session";

describe("TimelineTab", () => {
  beforeEach(() => {
    useDebugPanelStore.setState({
      isOpen: true,
      activeTab: "timeline",
      height: 30,
      selectedToolCallId: null,
    });
  });

  it("renders empty state when no messages", () => {
    render(<TimelineTab messages={[]} />);
    expect(screen.getByText("No events recorded")).toBeInTheDocument();
  });

  it("renders timeline events from messages", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
      { id: "m2", role: "assistant", content: "Hi there!", timestamp: "2024-01-01T00:00:02Z" },
    ];

    render(<TimelineTab messages={messages} />);

    expect(screen.getByText("User message")).toBeInTheDocument();
    expect(screen.getByText("Assistant response")).toBeInTheDocument();
  });

  it("renders tool call events", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"search","arguments":{"q":"test"}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", duration_ms: "200", status: "success" },
        toolCallId: "tc1",
      },
    ];

    render(<TimelineTab messages={messages} />);

    expect(screen.getByText("Tool: search")).toBeInTheDocument();
    expect(screen.getByText("200ms")).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("opens tool call in debug panel when clicked", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"search","arguments":{}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", status: "success" },
        toolCallId: "tc1",
      },
    ];

    render(<TimelineTab messages={messages} />);

    fireEvent.click(screen.getByTestId("timeline-event-m1"));

    const state = useDebugPanelStore.getState();
    expect(state.activeTab).toBe("toolcalls");
    expect(state.selectedToolCallId).toBe("tc1");
  });

  it("renders error status badges", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"cmd","arguments":{}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", status: "error" },
        toolCallId: "tc1",
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Err")).toBeInTheDocument();
  });

  it("renders workflow transition events", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "system",
        content: "Transitioning",
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "workflow_transition", from: "idle", to: "running" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Workflow: idle → running")).toBeInTheDocument();
  });

  it("renders timestamps in HH:mm:ss.SSS format", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T14:30:45.123Z" },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("14:30:45.123")).toBeInTheDocument();
  });

  it("shows handler type badge for tool calls with metadata", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"search","arguments":{}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", handler_type: "mcp", status: "success" },
        toolCallId: "tc1",
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("mcp")).toBeInTheDocument();
  });

  it("does not show handler type badge when absent", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"search","arguments":{}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", status: "success" },
        toolCallId: "tc1",
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.queryByText("mcp")).not.toBeInTheDocument();
  });
});
