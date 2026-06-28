/**
 * Resolves per-workspace service URLs from the Workspace CRD status.
 * Falls back to env vars for local development.
 */

import { getWorkspace } from "./workspace-route-helpers";

const ENV_SESSION_API_URL = process.env.SESSION_API_URL;
const ENV_MEMORY_API_URL = process.env.MEMORY_API_URL;
const ENV_SESSION_API_NAMESPACE = process.env.SESSION_API_NAMESPACE;
const ENV_PRIVACY_API_URL = process.env.PRIVACY_API_URL;

export interface ServiceURLs {
  sessionURL: string;
  memoryURL: string;
  /**
   * The Kubernetes namespace backing this workspace
   * (`Workspace.spec.namespace.name`, falling back to status then the name) —
   * NOT the workspace name. A workspace named `default` is provisioned in
   * namespace `omnia-default`, so backends that filter by namespace
   * (eval-results, provider-calls) MUST use this, never the workspace name.
   * See #1257.
   */
  namespace: string;
  /**
   * Workspace-level URL for the privacy-api. One per workspace, resolved from
   * `Workspace.status.privacyURL` (CRD) or `PRIVACY_API_URL` (env fallback).
   */
  privacyURL: string;
}

/**
 * Resolve service URLs + backing namespace for a workspace.
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
        return {
          sessionURL: sg.sessionURL,
          memoryURL: sg.memoryURL,
          // The backing namespace (e.g. "omnia-default"). spec.namespace.name
          // is the configured value and is always populated — the same source
          // the sessions route uses; fall back to status, then the name.
          namespace:
            workspace.spec?.namespace?.name ??
            workspace.status?.namespace?.name ??
            workspaceName,
          privacyURL: workspace.status?.privacyURL ?? "",
        };
      }
    }
  } catch {
    // K8s API unavailable — fall through to env var fallback
  }

  // Fall back to env vars (local dev, dashboard E2E). The namespace defaults
  // to the workspace name (legacy behaviour) unless explicitly overridden.
  if (ENV_SESSION_API_URL && ENV_MEMORY_API_URL) {
    return {
      sessionURL: ENV_SESSION_API_URL,
      memoryURL: ENV_MEMORY_API_URL,
      namespace: ENV_SESSION_API_NAMESPACE ?? workspaceName,
      privacyURL: ENV_PRIVACY_API_URL ?? "",
    };
  }

  return null;
}
