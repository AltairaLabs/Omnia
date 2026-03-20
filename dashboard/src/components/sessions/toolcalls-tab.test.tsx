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

  it("shows result panel when result is present", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "search",
        arguments: { q: "test" },
        result: { items: [1, 2, 3] },
        status: "success",
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByText("Result")).toBeInTheDocument();
    const jsonBlocks = screen.getAllByTestId("json-block");
    expect(jsonBlocks.length).toBe(2); // arguments + result
  });

  it("shows Error heading when result has errorMessage", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "failing-tool",
        arguments: {},
        result: "something went wrong",
        errorMessage: "timeout exceeded",
        status: "error",
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    // Should show "Error" heading (not "Result") when errorMessage is set
    // "Error" also appears in the badge, so check the heading specifically
    const errorHeadings = screen.getAllByText("Error");
    const h4 = errorHeadings.find((el) => el.tagName === "H4");
    expect(h4).toBeDefined();
    expect(screen.queryByText("Result")).not.toBeInTheDocument();
  });

  it("shows error-only panel when result is absent but errorMessage exists", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "broken-tool",
        arguments: { cmd: "run" },
        errorMessage: "connection refused",
        status: "error",
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    // "Error" appears in both badge and heading — verify heading exists
    const errorHeadings = screen.getAllByText("Error");
    const h4 = errorHeadings.find((el) => el.tagName === "H4");
    expect(h4).toBeDefined();
  });

  it("parses string result as JSON when valid", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "json-tool",
        arguments: {},
        result: '{"key":"value"}',
        status: "success",
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    const jsonBlocks = screen.getAllByTestId("json-block");
    // The result JSON block should contain the parsed key
    const resultBlock = jsonBlocks[1];
    expect(resultBlock.textContent).toContain("key");
    expect(resultBlock.textContent).toContain("value");
  });

  it("renders string result as-is when not valid JSON", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "text-tool",
        arguments: {},
        result: "plain text output",
        status: "success",
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByText("Result")).toBeInTheDocument();
  });

  it("falls back to tc.id when callId is empty", () => {
    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "", name: "fallback-tool", status: "success" }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    // Should use tc.id for data-testid when callId is empty
    expect(screen.getByTestId("toolcall-item-tc1")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("toolcall-item-tc1"));
    expect(useDebugPanelStore.getState().selectedToolCallId).toBe("tc1");
  });

  it("selects tool call by id when callId does not match", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "tc1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "id-match", arguments: { x: 1 } }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    // Should find the tool call via tc.id match and show detail
    expect(screen.getByTestId("toolcall-detail")).toBeInTheDocument();
    expect(screen.getByText("Arguments")).toBeInTheDocument();
  });

  it("shows only handler_type label without registry_name", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "handler-only",
        arguments: {},
        labels: {
          handler_name: "my-handler",
          handler_type: "http",
        },
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByTestId("toolcall-registry-info")).toBeInTheDocument();
    expect(screen.getByText("my-handler (http)")).toBeInTheDocument();
    expect(screen.queryByText("Registry")).not.toBeInTheDocument();
  });

  it("shows only registry_name label without handler_type", () => {
    useDebugPanelStore.setState({ selectedToolCallId: "call-1" });

    const toolCalls: ToolCall[] = [
      makeToolCall({
        id: "tc1",
        callId: "call-1",
        name: "registry-only",
        arguments: {},
        labels: {
          registry_name: "tools-v2",
        },
      }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    expect(screen.getByTestId("toolcall-registry-info")).toBeInTheDocument();
    expect(screen.getByText("tools-v2")).toBeInTheDocument();
    expect(screen.queryByText("Handler")).not.toBeInTheDocument();
  });

  it("does not show status badge when status is undefined", () => {
    const toolCalls: ToolCall[] = [
      makeToolCall({ id: "tc1", callId: "call-1", name: "no-status", status: undefined as unknown as ToolCall["status"] }),
    ];

    render(<ToolCallsTab toolCalls={toolCalls} />);

    // The badge should not render when status is falsy
    expect(screen.queryByTestId("tool-call-badge")).not.toBeInTheDocument();
  });
});
