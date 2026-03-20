import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TimelineTab } from "./timeline-tab";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import type { Message, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";
import type { TimelineEvent } from "@/lib/sessions/timeline";
import * as timelineModule from "@/lib/sessions/timeline";

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
    const toolCalls: ToolCall[] = [
      {
        id: "tc1",
        callId: "tc1",
        sessionId: "s1",
        name: "search",
        arguments: { q: "test" },
        status: "success",
        durationMs: 200,
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} toolCalls={toolCalls} />);

    expect(screen.getByText("Tool: search")).toBeInTheDocument();
    expect(screen.getByText("200ms")).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("opens tool call in debug panel when clicked", () => {
    const toolCalls: ToolCall[] = [
      {
        id: "tc1",
        callId: "call-1",
        sessionId: "s1",
        name: "search",
        arguments: {},
        status: "success",
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} toolCalls={toolCalls} />);

    // Timeline event ID is tc-{id}, toolCallId is callId
    fireEvent.click(screen.getByTestId("timeline-event-tc-tc1"));

    const state = useDebugPanelStore.getState();
    expect(state.activeTab).toBe("toolcalls");
    expect(state.selectedToolCallId).toBe("call-1");
  });

  it("renders error status badges", () => {
    const toolCalls: ToolCall[] = [
      {
        id: "tc1",
        callId: "tc1",
        sessionId: "s1",
        name: "cmd",
        arguments: {},
        status: "error",
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} toolCalls={toolCalls} />);
    expect(screen.getByText("Err")).toBeInTheDocument();
  });

  it("renders workflow transition events", () => {
    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "re1",
        sessionId: "s1",
        eventType: "workflow.transitioned",
        data: { from: "idle", to: "running" },
        timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);
    expect(screen.getByText("Workflow transition")).toBeInTheDocument();
  });

  it("renders timestamps in HH:mm:ss.SSS format", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T14:30:45.123Z" },
    ];

    render(<TimelineTab messages={messages} />);
    expect(screen.getByText("14:30:45.123")).toBeInTheDocument();
  });

  it("does not show handler type badge when absent on tool calls", () => {
    const toolCalls: ToolCall[] = [
      {
        id: "tc1",
        callId: "tc1",
        sessionId: "s1",
        name: "search",
        arguments: {},
        status: "success",
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} toolCalls={toolCalls} />);
    expect(screen.queryByText("mcp")).not.toBeInTheDocument();
  });

  it("renders tool call with success status", () => {
    const toolCalls: ToolCall[] = [
      {
        id: "tc1",
        callId: "tc1",
        sessionId: "s1",
        name: "calc",
        arguments: {},
        result: { output: "42" },
        status: "success",
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} toolCalls={toolCalls} />);
    expect(screen.getByText("Tool: calc")).toBeInTheDocument();
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("renders pipeline events as collapsible group", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m6", role: "assistant", content: "Hello!", timestamp: "2024-01-01T00:00:05Z" },
    ];

    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      { id: "re2", sessionId: "s1", eventType: "stage.started", data: { Name: "provider" }, timestamp: "2024-01-01T00:00:02Z" },
      { id: "re3", sessionId: "s1", eventType: "stage.completed", data: { Name: "provider" }, timestamp: "2024-01-01T00:00:03Z" },
      { id: "re4", sessionId: "s1", eventType: "pipeline.completed", timestamp: "2024-01-01T00:00:04Z" },
    ];

    render(<TimelineTab messages={messages} runtimeEvents={runtimeEvents} />);

    // Pipeline group is collapsed by default — child events not visible
    expect(screen.getByText("Agent Pipeline")).toBeInTheDocument();
    expect(screen.getByText("2 events")).toBeInTheDocument();
    expect(screen.queryByText("Stage: provider started")).not.toBeInTheDocument();

    // Top-level events still visible
    expect(screen.getByText("User message")).toBeInTheDocument();
    expect(screen.getByText("Assistant response")).toBeInTheDocument();
  });

  it("expands pipeline group on click", () => {
    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      { id: "re2", sessionId: "s1", eventType: "stage.started", data: { Name: "provider" }, timestamp: "2024-01-01T00:00:02Z" },
      { id: "re3", sessionId: "s1", eventType: "pipeline.completed", timestamp: "2024-01-01T00:00:03Z" },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);

    // Click to expand
    fireEvent.click(screen.getByText("Agent Pipeline"));

    // Child events now visible
    expect(screen.getByText("Stage: provider started")).toBeInTheDocument();
  });

  it("renders pending status badge for unclosed pipeline", () => {
    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      { id: "re2", sessionId: "s1", eventType: "stage.started", data: { Name: "provider" }, timestamp: "2024-01-01T00:00:02Z" },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);
    expect(screen.getByText("...")).toBeInTheDocument();
  });

  it("renders eval runtime events as regular events", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m2", role: "assistant", content: "Hello", timestamp: "2024-01-01T00:00:01Z" },
    ];

    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "e1", sessionId: "s1",
        eventType: "eval.completed",
        data: { evalID: "check1", passed: true },
        timestamp: "2024-01-01T00:00:02Z",
      },
      {
        id: "e2", sessionId: "s1",
        eventType: "eval.failed",
        data: { evalID: "check2", passed: false },
        errorMessage: "check failed",
        timestamp: "2024-01-01T00:00:03Z",
      },
    ];

    render(<TimelineTab messages={messages} runtimeEvents={runtimeEvents} />);
    // Eval runtime events render as system_message kind with eventType as label
    expect(screen.getByText("eval.completed")).toBeInTheDocument();
    expect(screen.getByText("eval.failed")).toBeInTheDocument();
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
    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      {
        id: "re2", sessionId: "s1",
        eventType: "pipeline.completed",
        errorMessage: "timeout",
        durationMs: 500,
        timestamp: "2024-01-01T00:00:02Z",
      },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);
    expect(screen.getByText("Err")).toBeInTheDocument();
    expect(screen.getByText("500ms")).toBeInTheDocument();
  });

  it("renders pipeline with success status and duration", () => {
    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      {
        id: "re2", sessionId: "s1",
        eventType: "pipeline.completed",
        durationMs: 250,
        timestamp: "2024-01-01T00:00:02Z",
      },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);
    expect(screen.getByText("OK")).toBeInTheDocument();
    expect(screen.getByText("250ms")).toBeInTheDocument();
  });

  it("renders provider call events", () => {
    const providerCalls: ProviderCall[] = [
      {
        id: "pc1",
        sessionId: "s1",
        provider: "openai",
        model: "gpt-4",
        status: "completed",
        durationMs: 1200,
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} providerCalls={providerCalls} />);
    expect(screen.getByText("Provider: openai/gpt-4")).toBeInTheDocument();
    // Duration appears both as detail text and badge — use getAllByText
    const durationElements = screen.getAllByText("1200ms");
    expect(durationElements.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("renders provider call with failed status", () => {
    const providerCalls: ProviderCall[] = [
      {
        id: "pc1",
        sessionId: "s1",
        provider: "anthropic",
        model: "claude-3",
        status: "failed",
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    render(<TimelineTab messages={[]} providerCalls={providerCalls} />);
    expect(screen.getByText("Provider: anthropic/claude-3")).toBeInTheDocument();
    expect(screen.getByText("Err")).toBeInTheDocument();
  });

  it("closes unclosed pipeline group when new pipeline starts", () => {
    const runtimeEvents: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:01Z" },
      { id: "re2", sessionId: "s1", eventType: "stage.started", data: { Name: "provider" }, timestamp: "2024-01-01T00:00:02Z" },
      // Second pipeline starts without first completing
      { id: "re3", sessionId: "s1", eventType: "pipeline.started", timestamp: "2024-01-01T00:00:03Z" },
      { id: "re4", sessionId: "s1", eventType: "pipeline.completed", durationMs: 100, timestamp: "2024-01-01T00:00:04Z" },
    ];

    render(<TimelineTab messages={[]} runtimeEvents={runtimeEvents} />);

    // Both pipeline groups should render
    const pipelineGroups = screen.getAllByText("Agent Pipeline");
    expect(pipelineGroups).toHaveLength(2);
  });

  describe("EvalGroupSection", () => {
    afterEach(() => {
      vi.restoreAllMocks();
    });

    function renderWithEvalEvents(events: TimelineEvent[]) {
      vi.spyOn(timelineModule, "extractTimelineEvents").mockReturnValue(events);
      return render(<TimelineTab messages={[]} />);
    }

    it("renders eval group with every_turn trigger label", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check_tone", status: "success", metadata: { trigger: "every_turn" } },
        { id: "ev2", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check_length", status: "error", metadata: { trigger: "every_turn" } },
      ]);

      expect(screen.getByText("Evals: Turn")).toBeInTheDocument();
      expect(screen.getByText("2 evals")).toBeInTheDocument();
      expect(screen.getByText("1 OK")).toBeInTheDocument();
      expect(screen.getByText("1 Fail")).toBeInTheDocument();
    });

    it("renders eval group with on_session_complete trigger label", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "final_check", status: "success", metadata: { trigger: "on_session_complete" } },
      ]);

      expect(screen.getByText("Evals: Session")).toBeInTheDocument();
      expect(screen.getByText("1 evals")).toBeInTheDocument();
      expect(screen.getByText("1 OK")).toBeInTheDocument();
    });

    it("renders eval group with custom trigger label", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "custom_eval", status: "success", metadata: { trigger: "custom_trigger" } },
      ]);

      expect(screen.getByText("Evals: custom_trigger")).toBeInTheDocument();
    });

    it("renders eval group with unknown trigger when metadata is missing", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "some_eval", status: "success" },
      ]);

      expect(screen.getByText("Evals: unknown")).toBeInTheDocument();
    });

    it("expands eval group on click to show child events", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check_tone", status: "success", metadata: { trigger: "every_turn" } },
        { id: "ev2", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check_length", status: "error", metadata: { trigger: "every_turn" } },
      ]);

      // Child events not visible when collapsed
      expect(screen.queryByText("check_tone")).not.toBeInTheDocument();

      // Click to expand
      fireEvent.click(screen.getByText("Evals: Turn"));

      // Child events now visible
      expect(screen.getByText("check_tone")).toBeInTheDocument();
      expect(screen.getByText("check_length")).toBeInTheDocument();
    });

    it("renders eval group with all failures and no OK badge", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check1", status: "error", metadata: { trigger: "every_turn" } },
        { id: "ev2", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check2", status: "error", metadata: { trigger: "every_turn" } },
      ]);

      expect(screen.getByText("2 Fail")).toBeInTheDocument();
      expect(screen.queryByText("OK", { exact: false })).not.toBeInTheDocument();
    });

    it("groups eval events between non-eval events separately", () => {
      renderWithEvalEvents([
        { id: "ev1", timestamp: "2024-01-01T00:00:01Z", kind: "eval_event", label: "check1", status: "success", metadata: { trigger: "every_turn" } },
        { id: "m1", timestamp: "2024-01-01T00:00:02Z", kind: "user_message", label: "User message" },
        { id: "ev2", timestamp: "2024-01-01T00:00:03Z", kind: "eval_event", label: "check2", status: "success", metadata: { trigger: "every_turn" } },
      ]);

      // Two separate eval groups with a user message between them
      const evalGroups = screen.getAllByText("Evals: Turn");
      expect(evalGroups).toHaveLength(2);
      expect(screen.getByText("User message")).toBeInTheDocument();
    });
  });
});
