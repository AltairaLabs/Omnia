import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ToolCallsTab } from "./toolcalls-tab";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { Message } from "@/types/session";

function makeToolCallMessage(id: string, toolCallId: string, name: string, args: Record<string, unknown>, meta?: Record<string, string>): Message {
  return {
    id,
    role: "assistant",
    content: JSON.stringify({ name, arguments: args }),
    timestamp: "2024-01-01T00:00:01Z",
    metadata: { type: "tool_call", ...meta },
    toolCallId,
  };
}

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

  it("renders list of tool calls from tool_call messages", () => {
    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "search", { q: "cats" }, { duration_ms: "150", status: "success" }),
      makeToolCallMessage("m2", "tc2", "fetch", { url: "example.com" }, { status: "error" }),
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByText("search")).toBeInTheDocument();
    expect(screen.getByText("fetch")).toBeInTheDocument();
    expect(screen.getByText("150ms")).toBeInTheDocument();
  });

  it("selects tool call on click and shows details", () => {
    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "search", { q: "cats", limit: 10 }),
    ];

    render(<ToolCallsTab messages={messages} />);

    fireEvent.click(screen.getByTestId("toolcall-item-tc1"));

    expect(useDebugPanelStore.getState().selectedToolCallId).toBe("tc1");

    // Detail pane should show arguments as key-value table (flat object)
    expect(screen.getByText("q")).toBeInTheDocument();
    expect(screen.getByText("cats")).toBeInTheDocument();
    expect(screen.getByText("Arguments")).toBeInTheDocument();
  });

  it("renders flat arguments as key-value table", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "cmd", { path: "/tmp", recursive: true }),
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
      makeToolCallMessage("m1", "tc1", "cmd", { nested: { a: 1 } }),
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByTestId("json-block")).toBeInTheDocument();
  });

  it("shows no selection message when nothing is selected", () => {
    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "cmd", {}),
    ];

    render(<ToolCallsTab messages={messages} />);
    expect(screen.getByText("Select a tool call to view details")).toBeInTheDocument();
  });

  it("displays registry metadata when present", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "search", { q: "test" }, {
        handler_name: "mcp-handler",
        handler_type: "mcp",
        registry_name: "my-tools",
      }),
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.getByTestId("toolcall-registry-info")).toBeInTheDocument();
    expect(screen.getByText("mcp-handler (mcp)")).toBeInTheDocument();
    expect(screen.getByText("my-tools")).toBeInTheDocument();
  });

  it("hides registry info when metadata is absent", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const messages: Message[] = [
      makeToolCallMessage("m1", "tc1", "search", { q: "test" }),
    ];

    render(<ToolCallsTab messages={messages} />);

    expect(screen.queryByTestId("toolcall-registry-info")).not.toBeInTheDocument();
  });
});
