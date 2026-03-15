import type { Message, ToolCall, ProviderCall } from "@/types/session";

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

function resolveMessageKind(message: Message): TimelineEventKind {
  const metadataType = message.metadata?.type;

  if (metadataType === "tool_call") return "tool_call";
  if (metadataType === "tool_result" || metadataType === "tool_call_completed" || message.role === "tool") return "tool_result";
  if (metadataType === "eval_completed" || metadataType === "eval_failed") return "eval_event";
  if (metadataType === "workflow_transition") return "workflow_transition";
  if (metadataType === "workflow_completed") return "workflow_completed";
  if (metadataType === "error") return "error";
  if (metadataType === "pipeline.started" || metadataType === "pipeline.completed") return "pipeline_event";
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
      return `Pipeline ${message.metadata?.type === "pipeline.started" ? "started" : "completed"}`;
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

/** Check whether a message-based event should be skipped because first-class records replace it. */
function shouldSkipMessageEvent(
  kind: TimelineEventKind,
  hasToolCalls: boolean,
  hasProviderCalls: boolean,
): boolean {
  if (hasToolCalls && (kind === "tool_call" || kind === "tool_result")) return true;
  if (hasProviderCalls && kind === "provider_call") return true;
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
 * optionally merging first-class tool call and provider call records.
 */
export function extractTimelineEvents(
  messages: Message[],
  toolCalls?: ToolCall[],
  providerCalls?: ProviderCall[],
): TimelineEvent[] {
  const hasToolCalls = (toolCalls?.length ?? 0) > 0;
  const hasProviderCalls = (providerCalls?.length ?? 0) > 0;
  const events: TimelineEvent[] = [];

  for (const message of messages) {
    const kind = resolveMessageKind(message);
    if (shouldSkipMessageEvent(kind, hasToolCalls, hasProviderCalls)) continue;
    events.push(messageToTimelineEvent(message, kind));
  }

  // Merge first-class records
  if (toolCalls) events.push(...toolCallsToTimelineEvents(toolCalls));
  if (providerCalls) events.push(...providerCallsToTimelineEvents(providerCalls));

  // Sort by timestamp (stable sort preserves insertion order for equal timestamps)
  events.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

  return events;
}
