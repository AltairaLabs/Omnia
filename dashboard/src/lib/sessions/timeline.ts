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

/** Event type constants to avoid string duplication (sonarjs/no-duplicate-string). */
const EVENT_PIPELINE_STARTED = "pipeline.started";
const EVENT_PIPELINE_COMPLETED = "pipeline.completed";

function truncate(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text;
  return text.slice(0, maxLength) + "...";
}

function resolveMessageKind(message: Message): TimelineEventKind {
  const metadataType = message.metadata?.type;

  if (metadataType === "tool_call") return "tool_call";
  if (metadataType === "tool_result" || metadataType === "tool_call_completed" || message.role === "tool") return "tool_result";
  if (metadataType === "eval_completed" || metadataType === "eval_failed") return "eval_event";
  if (metadataType === "workflow_transition") return "workflow_transition";
  if (metadataType === "workflow_completed") return "workflow_completed";
  if (metadataType === "error") return "error";
  if (metadataType === EVENT_PIPELINE_STARTED || metadataType === EVENT_PIPELINE_COMPLETED) return "pipeline_event";
  if (metadataType === "stage.started" || metadataType === "stage.completed") return "stage_event";
  if (metadataType === "provider_call") return "provider_call";

  switch (message.role) {
    case "user":
      return "user_message";
    case "assistant":
      return "assistant_message";
    case "system":
      return "system_message";
    default:
      return "system_message";
  }
}

/** Try to extract a tool name from JSON content. */
function parseToolName(content: string): string | undefined {
  try {
    const parsed = JSON.parse(content);
    return parsed.name;
  } catch {
    return undefined;
  }
}

/** Try to extract eval info from JSON content. */
function parseEvalInfo(content: string): { evalID?: string; passed?: boolean } {
  try {
    const parsed = JSON.parse(content);
    return { evalID: parsed.evalID, passed: parsed.passed };
  } catch {
    return {};
  }
}

/** Try to extract a stage/pipeline name from JSON content. */
function parseStageName(content: string): string | undefined {
  try {
    const parsed = JSON.parse(content);
    return parsed.Name || parsed.name;
  } catch {
    return undefined;
  }
}

function buildStageLabel(message: Message): string {
  const name = parseStageName(message.content);
  const action = message.metadata?.type === "stage.started" ? "started" : "completed";
  return name ? `Stage: ${name} ${action}` : `Stage ${action}`;
}

function buildEvalLabel(message: Message): string {
  const evalInfo = parseEvalInfo(message.content);
  const evalId = evalInfo.evalID || message.metadata?.eval_id || "eval";
  const status = evalInfo.passed ? "passed" : "failed";
  return `Eval: ${evalId} (${status})`;
}

const SIMPLE_LABELS: Partial<Record<TimelineEventKind, string>> = {
  user_message: "User message",
  assistant_message: "Assistant response",
  system_message: "System message",
  provider_call: "Provider call",
  workflow_completed: "Workflow completed",
  error: "Error",
};

function buildLabel(kind: TimelineEventKind, message: Message): string {
  const simple = SIMPLE_LABELS[kind];
  if (simple) return simple;

  switch (kind) {
    case "pipeline_event":
      return `Pipeline ${message.metadata?.type === EVENT_PIPELINE_STARTED ? "started" : "completed"}`;
    case "stage_event":
      return buildStageLabel(message);
    case "tool_call": {
      const tcName = parseToolName(message.content);
      return tcName ? `Tool: ${tcName}` : "Tool call";
    }
    case "tool_result": {
      const trName = message.metadata?.handler_name || parseToolName(message.content);
      return trName ? `Result: ${trName}` : "Tool result";
    }
    case "workflow_transition": {
      const from = message.metadata?.from;
      const to = message.metadata?.to;
      return from && to ? `Workflow: ${from} → ${to}` : "Workflow transition";
    }
    case "eval_event":
      return buildEvalLabel(message);
    default:
      return "Event";
  }
}

function resolveEventStatus(kind: TimelineEventKind, message: Message): TimelineEvent["status"] {
  if (kind === "error") return "error";
  if (kind === "eval_event") {
    return message.metadata?.passed === "true" ? "success" : "error";
  }
  const status = message.metadata?.status;
  if (status === "success" || status === "error") return status;
  return undefined;
}

function resolveToolCallStatus(status: string): TimelineEvent["status"] {
  if (status === "error") return "error";
  if (status === "success") return "success";
  return undefined;
}

function resolveProviderCallStatus(status: string): TimelineEvent["status"] {
  if (status === "failed") return "error";
  if (status === "completed") return "success";
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

/** Map a runtime event type to a TimelineEventKind. */
function runtimeEventKind(eventType: string): TimelineEventKind {
  if (eventType.startsWith("pipeline.")) return "pipeline_event";
  if (eventType.startsWith("stage.")) return "stage_event";
  if (eventType.startsWith("middleware.")) return "stage_event";
  if (eventType.startsWith("validation.")) return "stage_event";
  if (eventType === "workflow.completed") return "workflow_completed";
  if (eventType.startsWith("workflow.")) return "workflow_transition";
  if (eventType === "eval.completed" || eventType === "eval.failed") return "eval_event";
  if (eventType === "context_built" || eventType === "token_budget_exceeded") return "system_message";
  if (eventType === "state_loaded" || eventType === "state_saved") return "system_message";
  if (eventType === "stream_interrupted") return "error";
  return "system_message";
}

/** Build a human-readable label for a runtime event. */
/** Simple static labels for runtime events. */
const RUNTIME_EVENT_LABELS: Record<string, string> = {
  [EVENT_PIPELINE_STARTED]: "Pipeline started",
  [EVENT_PIPELINE_COMPLETED]: "Pipeline completed",
  "pipeline.failed": "Pipeline failed",
  "workflow.completed": "Workflow completed",
  "context_built": "Context built",
  "token_budget_exceeded": "Token budget exceeded",
  "state_loaded": "State loaded",
  "state_saved": "State saved",
  "stream_interrupted": "Stream interrupted",
};

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
  if (prefix === "eval") {
    const evalId = data?.eval_id || "eval";
    const status = eventType === "eval.completed" && data?.passed ? "passed" : "failed";
    return `Eval: ${evalId} (${status})`;
  }
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

/** Lifecycle event kinds that are replaced by first-class RuntimeEvents. */
const RUNTIME_EVENT_KINDS = new Set<TimelineEventKind>([
  "pipeline_event",
  "stage_event",
  "workflow_transition",
  "workflow_completed",
  "eval_event",
]);

/** Check whether a message-based event should be skipped because first-class records replace it. */
function shouldSkipMessageEvent(
  kind: TimelineEventKind,
  hasToolCalls: boolean,
  hasProviderCalls: boolean,
  hasRuntimeEvents: boolean,
): boolean {
  if (hasToolCalls && (kind === "tool_call" || kind === "tool_result")) return true;
  if (hasProviderCalls && kind === "provider_call") return true;
  if (hasRuntimeEvents && RUNTIME_EVENT_KINDS.has(kind)) return true;
  // Skip generic system messages from runtime events when first-class events exist
  if (hasRuntimeEvents && kind === "system_message") return true;
  return false;
}

/** Convert a single message to a TimelineEvent. */
function messageToTimelineEvent(message: Message, kind: TimelineEventKind): TimelineEvent {
  const durationStr = message.metadata?.duration_ms;
  const duration = durationStr ? Number.parseInt(durationStr, 10) : undefined;

  return {
    id: message.id,
    timestamp: message.timestamp,
    kind,
    label: buildLabel(kind, message),
    detail: message.content ? truncate(message.content, MAX_DETAIL_LENGTH) : undefined,
    toolCallId: (kind === "tool_call" || kind === "tool_result") ? message.toolCallId : undefined,
    duration: duration && !Number.isNaN(duration) ? duration : undefined,
    metadata: message.metadata,
    status: resolveEventStatus(kind, message),
  };
}

/**
 * Extract a flat, chronologically sorted list of timeline events from session messages,
 * optionally merging first-class tool call, provider call, and runtime event records.
 */
export function extractTimelineEvents(
  messages: Message[],
  toolCalls?: ToolCall[],
  providerCalls?: ProviderCall[],
  runtimeEvents?: RuntimeEvent[],
): TimelineEvent[] {
  const hasToolCalls = (toolCalls?.length ?? 0) > 0;
  const hasProviderCalls = (providerCalls?.length ?? 0) > 0;
  const hasRuntimeEvents = (runtimeEvents?.length ?? 0) > 0;
  const events: TimelineEvent[] = [];

  for (const message of messages) {
    const kind = resolveMessageKind(message);
    if (shouldSkipMessageEvent(kind, hasToolCalls, hasProviderCalls, hasRuntimeEvents)) continue;
    events.push(messageToTimelineEvent(message, kind));
  }

  // Merge first-class records
  if (toolCalls) events.push(...toolCallsToTimelineEvents(toolCalls));
  if (providerCalls) events.push(...providerCallsToTimelineEvents(providerCalls));
  if (runtimeEvents) events.push(...runtimeEventsToTimelineEvents(runtimeEvents));

  // Sort by timestamp (stable sort preserves insertion order for equal timestamps)
  events.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

  return events;
}
