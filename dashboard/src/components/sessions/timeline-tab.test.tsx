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

  it("renders tool result events", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "tool",
        content: '{"output":"42"}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_result", handler_name: "calc", status: "success" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Result: calc")).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("renders pipeline events as collapsible group", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m2", role: "system", content: "{}", timestamp: "2024-01-01T00:00:01Z", metadata: { source: "runtime", type: "pipeline.started" } },
      { id: "m3", role: "system", content: '{"Name":"provider"}', timestamp: "2024-01-01T00:00:02Z", metadata: { source: "runtime", type: "stage.started" } },
      { id: "m4", role: "system", content: '{"Name":"provider"}', timestamp: "2024-01-01T00:00:03Z", metadata: { source: "runtime", type: "stage.completed" } },
      { id: "m5", role: "system", content: "{}", timestamp: "2024-01-01T00:00:04Z", metadata: { source: "runtime", type: "pipeline.completed" } },
      { id: "m6", role: "assistant", content: "Hello!", timestamp: "2024-01-01T00:00:05Z" },
    ];

    render(<TimelineTab messages={messages} />);

    // Pipeline group is collapsed by default — child events not visible
    expect(screen.getByText("Agent Pipeline")).toBeInTheDocument();
    expect(screen.getByText("2 events")).toBeInTheDocument();
    expect(screen.queryByText("Stage: provider started")).not.toBeInTheDocument();

    // Top-level events still visible
    expect(screen.getByText("User message")).toBeInTheDocument();
    expect(screen.getByText("Assistant response")).toBeInTheDocument();
  });

  it("expands pipeline group on click", () => {
    const messages: Message[] = [
      { id: "m1", role: "system", content: "{}", timestamp: "2024-01-01T00:00:01Z", metadata: { source: "runtime", type: "pipeline.started" } },
      { id: "m2", role: "system", content: '{"Name":"provider"}', timestamp: "2024-01-01T00:00:02Z", metadata: { source: "runtime", type: "stage.started" } },
      { id: "m3", role: "system", content: "{}", timestamp: "2024-01-01T00:00:03Z", metadata: { source: "runtime", type: "pipeline.completed" } },
    ];

    render(<TimelineTab messages={messages} />);

    // Click to expand
    fireEvent.click(screen.getByText("Agent Pipeline"));

    // Child events now visible
    expect(screen.getByText("Stage: provider started")).toBeInTheDocument();
  });

  it("renders pending status badge for unclosed pipeline", () => {
    const messages: Message[] = [
      { id: "m1", role: "system", content: "{}", timestamp: "2024-01-01T00:00:01Z", metadata: { source: "runtime", type: "pipeline.started" } },
      { id: "m2", role: "system", content: '{"Name":"provider"}', timestamp: "2024-01-01T00:00:02Z", metadata: { source: "runtime", type: "stage.started" } },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("...")).toBeInTheDocument();
  });

  it("groups consecutive eval events by trigger", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m2", role: "assistant", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
      {
        id: "e1", role: "system",
        content: '{"evalID":"check1","passed":true}',
        timestamp: "2024-01-01T00:00:02Z",
        metadata: { source: "runtime", type: "eval_completed", trigger: "every_turn", passed: "true" },
      },
      {
        id: "e2", role: "system",
        content: '{"evalID":"check2","passed":false}',
        timestamp: "2024-01-01T00:00:03Z",
        metadata: { source: "runtime", type: "eval_completed", trigger: "every_turn", passed: "false" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Evals: Turn")).toBeInTheDocument();
    expect(screen.getByText("2 evals")).toBeInTheDocument();
    expect(screen.getByText("1 OK")).toBeInTheDocument();
    expect(screen.getByText("1 Fail")).toBeInTheDocument();
  });

  it("expands eval group on click", () => {
    const messages: Message[] = [
      {
        id: "e1", role: "system",
        content: '{"evalID":"tone-check","passed":true}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { source: "runtime", type: "eval_completed", trigger: "on_session_complete", status: "success" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Evals: Session")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Evals: Session"));
    // Child eval event visible after expand
    expect(screen.getByText("Eval: tone-check (passed)")).toBeInTheDocument();
  });

  it("shows unknown trigger label for unexpected trigger values", () => {
    const messages: Message[] = [
      {
        id: "e1", role: "system",
        content: '{"evalID":"x","passed":true}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { source: "runtime", type: "eval_completed", trigger: "custom_trigger", status: "success" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Evals: custom_trigger")).toBeInTheDocument();
  });

  it("handles eval events without trigger metadata", () => {
    const messages: Message[] = [
      {
        id: "e1", role: "system",
        content: '{"evalID":"check","passed":true}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { source: "runtime", type: "eval_completed", status: "success" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Evals: unknown")).toBeInTheDocument();
  });

  it("does not open debug panel for non-tool events", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
    ];

    render(<TimelineTab messages={messages} />);
    fireEvent.click(screen.getByTestId("timeline-event-m1"));

    const state = useDebugPanelStore.getState();
    // Should remain unchanged — user messages are not clickable
    expect(state.selectedToolCallId).toBeNull();
  });

  it("handles invalid timestamp gracefully", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "not-a-date" },
    ];

    render(<TimelineTab messages={messages} />);
    // Should still render without crashing
    expect(screen.getByText("User message")).toBeInTheDocument();
  });

  it("renders pipeline with error status", () => {
    const messages: Message[] = [
      { id: "m1", role: "system", content: "{}", timestamp: "2024-01-01T00:00:01Z", metadata: { source: "runtime", type: "pipeline.started" } },
      {
        id: "m2", role: "system", content: '{"error":"timeout"}',
        timestamp: "2024-01-01T00:00:02Z",
        metadata: { source: "runtime", type: "pipeline.completed", status: "error", duration_ms: "500" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("Err")).toBeInTheDocument();
    expect(screen.getByText("500ms")).toBeInTheDocument();
  });

  it("renders pipeline with success status and duration", () => {
    const messages: Message[] = [
      { id: "m1", role: "system", content: "{}", timestamp: "2024-01-01T00:00:01Z", metadata: { source: "runtime", type: "pipeline.started" } },
      {
        id: "m2", role: "system", content: "{}",
        timestamp: "2024-01-01T00:00:02Z",
        metadata: { source: "runtime", type: "pipeline.completed", status: "success", duration_ms: "250" },
      },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("OK")).toBeInTheDocument();
    expect(screen.getByText("250ms")).toBeInTheDocument();
  });
});
