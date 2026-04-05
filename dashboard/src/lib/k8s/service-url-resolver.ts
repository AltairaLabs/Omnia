/**
 * Resolves per-workspace service URLs from the Workspace CRD status.
 * Falls back to env vars for local development.
 */

import { getWorkspace } from "./workspace-route-helpers";

const ENV_SESSION_API_URL = process.env.SESSION_API_URL;
const ENV_MEMORY_API_URL = process.env.MEMORY_API_URL;

export interface ServiceURLs {
  sessionURL: string;
  memoryURL: string;
}

/**
 * Resolve service URLs for a workspace.
 * Priority: Workspace CRD status -> env var fallback.
 */
export async function resolveServiceURLs(
  workspaceName: string,
  serviceGroup = "default"
): Promise<ServiceURLs | null> {
  // Try Workspace CRD status first (may fail if K8s API unavailable)
  try {
    const workspace = await getWorkspace(workspaceName);
    if (workspace?.status?.services) {
      const sg = workspace.status.services.find(
        (s) => s.name === serviceGroup && s.ready
      );
      if (sg) {
        return { sessionURL: sg.sessionURL, memoryURL: sg.memoryURL };
      }
    }
  } catch {
    // K8s API unavailable — fall through to env var fallback
  }

  // Fall back to env vars (local dev, dashboard E2E)
  if (ENV_SESSION_API_URL && ENV_MEMORY_API_URL) {
    return { sessionURL: ENV_SESSION_API_URL, memoryURL: ENV_MEMORY_API_URL };
  }

  return null;
}
