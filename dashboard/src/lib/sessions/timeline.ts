import type { Message } from "@/types/session";

export type TimelineEventKind =
  | "user_message"
  | "assistant_message"
  | "system_message"
  | "tool_call"
  | "pipeline_event"
  | "stage_event"
  | "provider_call"
  | "workflow_transition"
  | "workflow_completed"
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

/** Try to extract a stage/pipeline name from JSON content. */
function parseStageName(content: string): string | undefined {
  try {
    const parsed = JSON.parse(content);
    return parsed.Name || parsed.name;
  } catch {
    return undefined;
  }
}

function buildLabel(kind: TimelineEventKind, message: Message): string {
  switch (kind) {
    case "user_message":
      return "User message";
    case "assistant_message":
      return "Assistant response";
    case "system_message":
      return "System message";
    case "pipeline_event": {
      const action = message.metadata?.type === "pipeline.started" ? "started" : "completed";
      return `Pipeline ${action}`;
    }
    case "stage_event": {
      const name = parseStageName(message.content);
      const action = message.metadata?.type === "stage.started" ? "started" : "completed";
      return name ? `Stage: ${name} ${action}` : `Stage ${action}`;
    }
    case "provider_call":
      return "Provider call";
    case "workflow_transition": {
      const from = message.metadata?.from;
      const to = message.metadata?.to;
      if (from && to) return `Workflow: ${from} → ${to}`;
      return "Workflow transition";
    }
    case "workflow_completed":
      return "Workflow completed";
    case "error":
      return "Error";
    default:
      return "Event";
  }
}

/**
 * Extract a flat, chronologically sorted list of timeline events from session messages.
 *
 * Skips `role === "tool"` messages since tool results are represented
 * through the tool_call events attached to their parent assistant message.
 */
export function extractTimelineEvents(messages: Message[]): TimelineEvent[] {
  const events: TimelineEvent[] = [];

  for (const message of messages) {
    // Skip raw tool result messages
    if (message.role === "tool") continue;

    const kind = resolveMessageKind(message);

    // Emit the message-level event
    events.push({
      id: message.id,
      timestamp: message.timestamp,
      kind,
      label: buildLabel(kind, message),
      detail: message.content ? truncate(message.content, MAX_DETAIL_LENGTH) : undefined,
      metadata: message.metadata,
      status: kind === "error" ? "error" : undefined,
    });

    // Emit individual tool call events
    if (message.toolCalls) {
      for (const tc of message.toolCalls) {
        events.push({
          id: `${message.id}-tc-${tc.id}`,
          timestamp: message.timestamp,
          kind: "tool_call",
          label: tc.name,
          toolCallId: tc.id,
          duration: tc.duration,
          status: tc.status,
        });
      }
    }
  }

  // Sort by timestamp (stable sort preserves insertion order for equal timestamps)
  events.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

  return events;
}
