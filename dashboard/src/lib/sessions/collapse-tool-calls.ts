import type { ToolCall } from "@/types/session";

/**
 * Collapse tool call lifecycle events into one entry per logical call.
 *
 * The backend records each lifecycle event (pending, success, error) as a
 * separate row sharing the same `callId`. This function merges them so the
 * UI shows one item per logical tool invocation with the most resolved state.
 *
 * Priority: success > error > pending.
 * Fields from the most resolved event win (result, errorMessage, durationMs, status).
 * Arguments and labels come from the earliest event (the started event).
 */
export function collapseToolCalls(events: ToolCall[]): ToolCall[] {
  const byCallId = new Map<string, ToolCall[]>();

  for (const tc of events) {
    const key = tc.callId || tc.id;
    const group = byCallId.get(key);
    if (group) {
      group.push(tc);
    } else {
      byCallId.set(key, [tc]);
    }
  }

  const statusPriority: Record<string, number> = {
    pending: 0,
    error: 1,
    success: 2,
  };

  const result: ToolCall[] = [];

  for (const group of byCallId.values()) {
    if (group.length === 1) {
      result.push(group[0]);
      continue;
    }

    // Sort by status priority (highest wins)
    group.sort(
      (a, b) => (statusPriority[b.status] ?? 0) - (statusPriority[a.status] ?? 0),
    );

    const resolved = group[0]; // highest priority status

    // Find the started event for arguments/labels (earliest by timestamp)
    const started = group.reduce((earliest, tc) =>
      new Date(tc.createdAt).getTime() < new Date(earliest.createdAt).getTime()
        ? tc
        : earliest,
      group[0],
    );

    result.push({
      ...resolved,
      // Preserve arguments and labels from the started event
      arguments: started.arguments ?? resolved.arguments,
      labels: started.labels ?? resolved.labels,
      // Use the started event's timestamp for timeline ordering
      createdAt: started.createdAt,
    });
  }

  // Sort by timestamp
  result.sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
  );

  return result;
}
