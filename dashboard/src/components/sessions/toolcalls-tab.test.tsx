import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ToolCallsTab } from "./toolcalls-tab";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { ToolCall } from "@/types/session";

function makeToolCall(overrides: Partial<ToolCall> & { id: string; callId: string; name: string }): ToolCall {
  return {
    sessionId: "sess-1",
    arguments: {},
    status: "pending",
    createdAt: "2024-01-01T00:00:01Z",
    ...overrides,
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
    render(<ToolCallsTab toolCalls={[]} />);
    expect(screen.getByText("No tool calls in this session")).toBeInTheDocument();
  });

  it("renders list of tool calls", () => {
    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "search", durationMs: 150, status: "success" }),
      makeToolCall({ id: "tc2", callId: "call-2", name: "fetch", status: "error" }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByText("search")).toBeInTheDocument();
    expect(screen.getByText("fetch")).toBeInTheDocument();
    expect(screen.getByText("150ms")).toBeInTheDocument();
  });

  it("selects tool call on click and shows details", () => {
    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "search", arguments: { q: "cats", limit: 10 } }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    fireEvent.click(screen.getByTestId("toolcall-item-call-1"));

    expect(useDebugPanelStore.getState().selectedToolCallId).toBe("call-1");

    expect(screen.getByText("Arguments")).toBeInTheDocument();
    expect(screen.getByTestId("json-block")).toBeInTheDocument();
  });

  it("renders arguments in collapsible JSON viewer", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "cmd", arguments: { path: "/tmp", recursive: true } }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    const jsonBlock = screen.getByTestId("json-block");
    expect(jsonBlock).toBeInTheDocument();
    expect(jsonBlock.textContent).toContain("path");
    expect(jsonBlock.textContent).toContain("/tmp");
    expect(jsonBlock.textContent).toContain("recursive");
    expect(jsonBlock.textContent).toContain("true");
  });

  it("renders nested arguments as JSON", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "cmd", arguments: { nested: { a: 1 } } }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByTestId("json-block")).toBeInTheDocument();
  });

  it("shows no selection message when nothing is selected", () => {
    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "cmd" }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);
    expect(screen.getByText("Select a tool call to view details")).toBeInTheDocument();
  });

  it("displays registry metadata from labels when present", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "search",
        arguments: { q: "test" },
        labels: {
          handler_name: "mcp-handler",
          handler_type: "mcp",
          registry_name: "my-tools",
        },
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByTestId("toolcall-registry-info")).toBeInTheDocument();
    expect(screen.getByText("mcp-handler (mcp)")).toBeInTheDocument();
    expect(screen.getByText("my-tools")).toBeInTheDocument();
  });

  it("hides registry info when labels are absent", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "search", arguments: { q: "test" } }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.queryByTestId("toolcall-registry-info")).not.toBeInTheDocument();
  });
});
