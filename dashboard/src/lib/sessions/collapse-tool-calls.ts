import type { ToolCall } from "@/types/session";

/**
 * Collapse tool call lifecycle events into one entry per logical call.
 *
 * The backend records each lifecycle event (pending, then success/error) as a
 * separate row sharing the same `callId`. This function pairs them so the UI
 * shows one item per logical tool invocation with the most resolved state.
 *
 * `callId` is NOT globally unique within a session — providers reset their
 * call indexer on each tool-calling round (e.g. Gemini emits `call_0` again
 * for the first tool call of round 2). Pairing must therefore walk events
 * in chronological order and match each `pending` with the next
 * chronologically-following `success`/`error` carrying the same callId
 * (FIFO). Anything left unmatched (an orphan pending or an orphan resolution)
 * is returned as a standalone entry.
 *
 * The merged entry keeps `arguments` / `labels` from the `pending` row and
 * `result` / `errorMessage` / `durationMs` / `status` from the resolution.
 * Output is sorted by the started timestamp.
 */
export function collapseToolCalls(events: ToolCall[]): ToolCall[] {
  // Sort chronologically so FIFO pairing matches user-visible order.
  const sorted = [...events].sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
  );

  // Pending queue per callId, in arrival order.
  const pendingByCallId = new Map<string, ToolCall[]>();
  const merged: ToolCall[] = [];

  for (const tc of sorted) {
    const key = tc.callId || tc.id;

    if (tc.status === "pending") {
      const queue = pendingByCallId.get(key) ?? [];
      queue.push(tc);
      pendingByCallId.set(key, queue);
      continue;
    }

    // Resolution event (success / error). Pair with the oldest pending of
    // the same callId, if one exists.
    const queue = pendingByCallId.get(key);
    const started = queue?.shift();
    if (queue && queue.length === 0) {
      pendingByCallId.delete(key);
    }

    if (!started) {
      // Orphan resolution — no matching pending was seen. Surface as-is so
      // the user still sees the row instead of dropping it on the floor.
      merged.push(tc);
      continue;
    }

    merged.push({
      ...tc,
      // Preserve arguments / labels from the started event.
      arguments: started.arguments ?? tc.arguments,
      labels: started.labels ?? tc.labels,
      // Use the started event's timestamp so the row sorts by call start.
      createdAt: started.createdAt,
    });
  }

  // Any pendings still in the queue never resolved — emit them so the user
  // sees the in-flight call instead of losing it.
  for (const queue of pendingByCallId.values()) {
    for (const orphan of queue) {
      merged.push(orphan);
    }
  }

  merged.sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
  );

  return merged;
}
