import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ToolCallsTab } from "./toolcalls-tab";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { Message } from "@/types/session";

describe("ToolCallsTab", () => {
  beforeEach(() => {
    useDebugPanelStore.setState({
      isOpen: true,
      activeTab: "toolcalls",
      height: 30,
      selectedToolCallId: null,
    });
  });

  it("renders empty state when no tool calls", () => {
    render(<ToolCallsTab messages={[]} />);
    expect(screen.getByText("No tool calls in this session")).toBeInTheDocument();
  });

  it("renders empty state when messages have no tool calls", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
    ];
    render(<ToolCallsTab messages={messages} />);
    expect(screen.getByTestId("toolcalls-empty")).toBeInTheDocument();
  });

  it("renders list of tool calls", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Working",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "search", arguments: { q: "cats" }, status: "success", duration: 150 },
          { id: "tc2", name: "fetch", arguments: { url: "example.com" }, status: "error" },
        ],
      },
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByText("search")).toBeInTheDocument();
    expect(screen.getByText("fetch")).toBeInTheDocument();
    expect(screen.getByText("150ms")).toBeInTheDocument();
  });

  it("selects tool call on click and shows details", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Done",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "search", arguments: { q: "cats", limit: 10 }, result: { count: 2 }, status: "success" },
        ],
      },
    ];

    render(<ToolCallsTab messages={messages} />);

    fireEvent.click(screen.getByTestId("toolcall-item-tc1"));

    expect(useDebugPanelStore.getState().selectedToolCallId).toBe("tc1");

    // Detail pane should show arguments as key-value table (flat object)
    expect(screen.getByText("q")).toBeInTheDocument();
    expect(screen.getByText("cats")).toBeInTheDocument();
    expect(screen.getByText("Arguments")).toBeInTheDocument();
    expect(screen.getByText("Result")).toBeInTheDocument();
  });

  it("renders flat arguments as key-value table", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Done",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "cmd", arguments: { path: "/tmp", recursive: true }, status: "success" },
        ],
      },
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByText("path")).toBeInTheDocument();
    expect(screen.getByText("/tmp")).toBeInTheDocument();
    expect(screen.getByText("recursive")).toBeInTheDocument();
    expect(screen.getByText("true")).toBeInTheDocument();
  });

  it("renders nested arguments as JSON", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Done",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "cmd", arguments: { nested: { a: 1 } }, status: "success" },
        ],
      },
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByTestId("json-block")).toBeInTheDocument();
  });

  it("shows no selection message when nothing is selected", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Done",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "cmd", arguments: {}, status: "success" },
        ],
      },
    ];

    render(<ToolCallsTab messages={messages} />);
    expect(screen.getByText("Select a tool call to view details")).toBeInTheDocument();
  });
});
