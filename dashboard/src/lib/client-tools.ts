/**
 * Client-side tool handler registry.
 *
 * Tools listed in a ToolRegistry with type "client" are executed by the
 * browser, not the server. This module provides a registry of handler
 * functions keyed by tool name. When the user approves a client tool call,
 * the matching handler is invoked and its return value is sent back as
 * the tool result.
 *
 * To add a new client tool, register a handler here:
 *
 *   registerClientToolHandler("camera_capture", async (args) => {
 *     const stream = await navigator.mediaDevices.getUserMedia({ video: true });
 *     // ...capture frame...
 *     return { image: base64Data };
 *   });
 */

/** A client tool handler receives the tool arguments and returns the result. */
export type ClientToolHandler = (
  args?: Record<string, unknown>,
) => Promise<unknown>;

const handlers = new Map<string, ClientToolHandler>();

/** Register a handler for a client-side tool. */
export function registerClientToolHandler(
  toolName: string,
  handler: ClientToolHandler,
): void {
  handlers.set(toolName, handler);
}

/** Look up a registered handler by tool name. Returns undefined if not registered. */
export function getClientToolHandler(
  toolName: string,
): ClientToolHandler | undefined {
  return handlers.get(toolName);
}

/** Check whether a handler is registered for the given tool name. */
export function hasClientToolHandler(toolName: string): boolean {
  return handlers.has(toolName);
}

// ---------------------------------------------------------------------------
// Auto-approve persistence (localStorage)
// ---------------------------------------------------------------------------

const STORAGE_KEY = "omnia-client-tool-auto-approve";

function loadAutoApproved(): Set<string> {
  if (typeof localStorage === "undefined") return new Set();
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? new Set(JSON.parse(raw) as string[]) : new Set();
  } catch {
    return new Set();
  }
}

function saveAutoApproved(tools: Set<string>): void {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify([...tools]));
}

/** Check if a tool is auto-approved. */
export function isAutoApproved(toolName: string): boolean {
  return loadAutoApproved().has(toolName);
}

/** Mark a tool as auto-approved (persisted across sessions). */
export function setAutoApproved(toolName: string): void {
  const tools = loadAutoApproved();
  tools.add(toolName);
  saveAutoApproved(tools);
}

/** Remove auto-approval for a tool. */
export function clearAutoApproved(toolName: string): void {
  const tools = loadAutoApproved();
  tools.delete(toolName);
  saveAutoApproved(tools);
}

// ---------------------------------------------------------------------------
// Built-in handlers
// ---------------------------------------------------------------------------

registerClientToolHandler("get_user_location", () => {
  return new Promise((resolve, reject) => {
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      reject(new Error("Geolocation is not available in this browser"));
      return;
    }

    // eslint-disable-next-line sonarjs/no-intrusive-permissions
    navigator.geolocation.getCurrentPosition(
      (pos) =>
        resolve({
          latitude: pos.coords.latitude,
          longitude: pos.coords.longitude,
          accuracy: pos.coords.accuracy,
        }),
      (err) => reject(new Error(`Geolocation error: ${err.message}`)),
      { timeout: 10000 },
    );
  });
});
