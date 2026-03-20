/**
 * Session namespace guard.
 *
 * Verifies that a session belongs to the workspace's namespace before
 * returning data. Prevents IDOR attacks where a user with access to
 * workspace A could read sessions from workspace B by guessing session IDs.
 */

import { NextResponse } from "next/server";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";

const SESSION_API_URL = process.env.SESSION_API_URL;

/**
 * Resolve the workspace namespace and verify the session belongs to it.
 *
 * Fetches the session metadata from session-api, then compares its namespace
 * against the workspace's namespace. Returns an error response if:
 *  - SESSION_API_URL is not configured (503)
 *  - The workspace does not exist (404)
 *  - The session does not exist (404 forwarded from backend)
 *  - The session's namespace does not match the workspace's namespace (404)
 *
 * @returns `{ ok: true, namespace, baseUrl }` on success, or
 *          `{ ok: false, response }` with an appropriate HTTP error.
 */
export async function verifySessionNamespace(
  workspaceName: string,
  sessionId: string
): Promise<
  | { ok: true; namespace: string; baseUrl: string }
  | { ok: false; response: NextResponse }
> {
  if (!SESSION_API_URL) {
    return {
      ok: false,
      response: NextResponse.json(
        { error: "Session API not configured" },
        { status: 503 }
      ),
    };
  }

  const workspace = await getWorkspace(workspaceName);
  if (!workspace) {
    return {
      ok: false,
      response: NextResponse.json(
        { error: "Workspace not found" },
        { status: 404 }
      ),
    };
  }

  const namespace = workspace.spec.namespace.name;
  const baseUrl = SESSION_API_URL.endsWith("/")
    ? SESSION_API_URL.slice(0, -1)
    : SESSION_API_URL;

  // Fetch session metadata to verify namespace ownership.
  const sessionUrl = `${baseUrl}/api/v1/sessions/${encodeURIComponent(sessionId)}`;
  let sessionResponse: Response;
  try {
    sessionResponse = await fetch(sessionUrl, {
      headers: { Accept: "application/json" },
    });
  } catch {
    return {
      ok: false,
      response: NextResponse.json(
        { error: "Failed to connect to Session API" },
        { status: 502 }
      ),
    };
  }

  if (!sessionResponse.ok) {
    const data = await sessionResponse.json();
    return {
      ok: false,
      response: NextResponse.json(data, { status: sessionResponse.status }),
    };
  }

  const sessionData = await sessionResponse.json();
  const sessionNamespace: string | undefined = sessionData?.session?.namespace;

  if (sessionNamespace !== namespace) {
    return {
      ok: false,
      response: NextResponse.json(
        { error: "Session not found" },
        { status: 404 }
      ),
    };
  }

  return { ok: true, namespace, baseUrl };
}
