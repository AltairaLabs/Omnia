import type { Message, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";

export type TimelineEventKind =
  | "user_message"
  | "assistant_message"
  | "system_message"
  | "tool_call"
  | "tool_result"
  | "pipeline_event"
  | "stage_event"
  | "provider_call"
  | "workflow_transition"
  | "workflow_completed"
  | "eval_event"
  | "error";

export interface TimelineEvent {
  id: string;
  timestamp: string;
  kind: TimelineEventKind;
  label: string;
  detail?: string;
  toolCallId?: string;
  duration?: number;
  status?: "success" | "error" | "pending";
  metadata?: Record<string, string>;
}

const MAX_DETAIL_LENGTH = 120;

function truncate(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength) + "...";
}

// --- Message events (conversation only) ---

function messageToTimelineEvent(message: Message): TimelineEvent {
  const kind: TimelineEventKind = message.role === "user" ? "user_message" : "assistant_message";
  return {
    id: message.id,
    timestamp: message.timestamp,
    kind,
    label: kind === "user_message" ? "User message" : "Assistant response",
    detail: message.content ? truncate(message.content, MAX_DETAIL_LENGTH) : undefined,
  };
}

// --- Tool call events ---

function resolveToolCallStatus(status: string): TimelineEvent["status"] {
  if (status === "error") return "error";
  if (status === "success") return "success";
  return undefined;
}

/** Convert first-class ToolCall records to timeline events. */
export function toolCallsToTimelineEvents(toolCalls: ToolCall[]): TimelineEvent[] {
  return toolCalls.map((tc) => ({
    id: `tc-${tc.id}`,
    timestamp: tc.createdAt,
    kind: "tool_call" as TimelineEventKind,
    label: `Tool: ${tc.name}`,
    detail: tc.arguments ? truncate(JSON.stringify(tc.arguments), MAX_DETAIL_LENGTH) : undefined,
    toolCallId: tc.callId || tc.id,
    duration: tc.durationMs,
    status: resolveToolCallStatus(tc.status),
  }));
}

// --- Provider call events ---

function resolveProviderCallStatus(status: string): TimelineEvent["status"] {
  if (status === "failed") return "error";
  if (status === "completed") return "success";
  return undefined;
}

/** Convert first-class ProviderCall records to timeline events. */
export function providerCallsToTimelineEvents(providerCalls: ProviderCall[]): TimelineEvent[] {
  return providerCalls.map((pc) => ({
    id: `pc-${pc.id}`,
    timestamp: pc.createdAt,
    kind: "provider_call" as TimelineEventKind,
    label: `Provider: ${pc.provider}/${pc.model}`,
    detail: pc.durationMs ? `${pc.durationMs}ms` : undefined,
    duration: pc.durationMs,
    status: resolveProviderCallStatus(pc.status),
  }));
}

// --- Runtime events (pipeline, stage, middleware, validation, workflow, eval) ---

/** Static labels for common runtime event types. */
const RUNTIME_EVENT_LABELS: Record<string, string> = {
  "pipeline.started": "Pipeline started",
  "pipeline.completed": "Pipeline completed",
  "pipeline.failed": "Pipeline failed",
  "workflow.completed": "Workflow completed",
  "context_built": "Context built",
  "token_budget_exceeded": "Token budget exceeded",
  "state_loaded": "State loaded",
  "state_saved": "State saved",
  "stream_interrupted": "Stream interrupted",
};

function runtimeEventKind(eventType: string): TimelineEventKind {
  if (eventType.startsWith("pipeline.")) return "pipeline_event";
  if (eventType.startsWith("stage.")) return "stage_event";
  if (eventType.startsWith("middleware.")) return "stage_event";
  if (eventType.startsWith("validation.")) return "stage_event";
  if (eventType === "workflow.completed") return "workflow_completed";
  if (eventType.startsWith("workflow.")) return "workflow_transition";
  if (eventType === "context_built" || eventType === "token_budget_exceeded") return "system_message";
  if (eventType === "state_loaded" || eventType === "state_saved") return "system_message";
  if (eventType === "stream_interrupted") return "error";
  return "system_message";
}

function runtimeEventLabel(eventType: string, data?: Record<string, unknown>): string {
  const simple = RUNTIME_EVENT_LABELS[eventType];
  if (simple) return simple;

  const name = data?.Name || data?.name;
  const nameStr = typeof name === "string" ? `: ${name}` : "";
  const parts = eventType.split(".");
  const prefix = parts[0];
  const action = parts[1] || "";

  if (prefix === "stage" || prefix === "middleware" || prefix === "validation") {
    const label = prefix.charAt(0).toUpperCase() + prefix.slice(1);
    return `${label}${nameStr} ${action}`;
  }
  if (eventType === "workflow.transitioned") return `Workflow transition${nameStr}`;
  return eventType;
}

/** Convert first-class RuntimeEvent records to timeline events. */
export function runtimeEventsToTimelineEvents(events: RuntimeEvent[]): TimelineEvent[] {
  return events.map((evt) => {
    const kind = runtimeEventKind(evt.eventType);
    const isFailed = evt.eventType.endsWith(".failed") || !!evt.errorMessage;
    const isCompleted = evt.eventType.endsWith(".completed") || evt.eventType.endsWith(".passed");
    let status: TimelineEvent["status"];
    if (isFailed) status = "error";
    else if (isCompleted) status = "success";

    return {
      id: `re-${evt.id}`,
      timestamp: evt.timestamp,
      kind,
      label: runtimeEventLabel(evt.eventType, evt.data),
      detail: evt.errorMessage || (evt.data ? truncate(JSON.stringify(evt.data), MAX_DETAIL_LENGTH) : undefined),
      duration: evt.durationMs,
      status,
      metadata: { type: evt.eventType },
    };
  });
}

// --- Main extraction ---

/**
 * Build a chronologically sorted list of timeline events from first-class records.
 * Messages contribute only user/assistant conversation events.
 * Tool calls, provider calls, and runtime events come from their dedicated tables.
 */
export function extractTimelineEvents(
  messages: Message[],
  toolCalls?: ToolCall[],
  providerCalls?: ProviderCall[],
  runtimeEvents?: RuntimeEvent[],
): TimelineEvent[] {
  const events: TimelineEvent[] = [];

  // Conversation messages (user + assistant only)
  for (const message of messages) {
    if (message.role === "user" || message.role === "assistant") {
      events.push(messageToTimelineEvent(message));
    }
  }

  // First-class records
  if (toolCalls) events.push(...toolCallsToTimelineEvents(toolCalls));
  if (providerCalls) events.push(...providerCallsToTimelineEvents(providerCalls));
  if (runtimeEvents) events.push(...runtimeEventsToTimelineEvents(runtimeEvents));

  // Sort by timestamp (stable sort preserves insertion order for equal timestamps)
  events.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

  return events;
}
