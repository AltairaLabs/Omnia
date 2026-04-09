/**
 * Kubernetes Workspace CRD client.
 *
 * Provides operations for fetching Workspace resources from the K8s API.
 * Workspaces are cluster-scoped resources that define team/project boundaries.
 */

import * as k8s from "@kubernetes/client-node";
import type { Workspace, WorkspaceSpec } from "@/types/workspace";

const GROUP = "omnia.altairalabs.ai";
const VERSION = "v1alpha1";
const PLURAL = "workspaces";

/**
 * Create the K8s custom objects API client.
 * Uses in-cluster config when running in K8s, falls back to kubeconfig for local dev.
 * Returns null if no valid K8s API server is available.
 */
function createCustomObjectsClient(): k8s.CustomObjectsApi | null {
  const kc = new k8s.KubeConfig();

  try {
    // Try in-cluster config first (when running in K8s)
    kc.loadFromCluster();
  } catch {
    // Fall back to default kubeconfig for local development
    kc.loadFromDefault();
  }

  // Verify the cluster has a valid server URL — if not, K8s is unavailable
  // (e.g. dashboard E2E, standalone demo mode). Return null to avoid
  // making failing HTTP requests on every API call.
  const cluster = kc.getCurrentCluster();
  if (!cluster?.server) {
    return null;
  }

  return kc.makeApiClient(k8s.CustomObjectsApi);
}

// Singleton client (null means K8s unavailable)
let client: k8s.CustomObjectsApi | null = null;
let clientInitialized = false;

function getClient(): k8s.CustomObjectsApi | null {
  if (!clientInitialized) {
    client = createCustomObjectsClient();
    clientInitialized = true;
  }
  return client;
}

/**
 * Get a single Workspace by name.
 * Workspaces are cluster-scoped resources.
 *
 * @param name - The workspace name
 * @returns The workspace resource, or null if not found
 */
export async function getWorkspace(name: string): Promise<Workspace | null> {
  const k8sClient = getClient();
  if (!k8sClient) {
    return null;
  }

  try {
    const response = await k8sClient.getClusterCustomObject({
      group: GROUP,
      version: VERSION,
      plural: PLURAL,
      name,
    });
    return response as Workspace;
  } catch (error) {
    if (isNotFoundError(error)) {
      return null;
    }
    // K8s API unreachable (e.g. dashboard E2E without cluster) — treat as not found
    if (isConnectionError(error)) {
      return null;
    }
    throw error;
  }
}

/**
 * List all Workspaces in the cluster.
 *
 * @param labelSelector - Optional label selector to filter workspaces
 * @returns Array of workspace resources (empty if CRD not installed)
 */
export async function listWorkspaces(
  labelSelector?: string
): Promise<Workspace[]> {
  const k8sClient = getClient();
  if (!k8sClient) {
    return [];
  }

  try {
    const response = await k8sClient.listClusterCustomObject({
      group: GROUP,
      version: VERSION,
      plural: PLURAL,
      labelSelector,
    });

    const list = response as { items: Workspace[] };
    return list.items || [];
  } catch (error) {
    // Return empty array if CRD doesn't exist (404) or not accessible
    if (isNotFoundError(error)) {
      console.warn("Workspace CRD not found - workspaces feature unavailable");
      return [];
    }
    throw error;
  }
}

/**
 * Merge-patch a Workspace by name.
 * Only the fields provided in updates are changed; all others are preserved.
 *
 * @param name - The workspace name
 * @param updates - Partial spec fields to apply via merge-patch
 * @returns The patched workspace, or null if client is unavailable or an error occurs
 */
export async function patchWorkspace(
  name: string,
  updates: Partial<WorkspaceSpec>
): Promise<Workspace | null> {
  const k8sClient = getClient();
  if (!k8sClient) {
    return null;
  }

  try {
    const response = await k8sClient.patchClusterCustomObject({
      group: GROUP,
      version: VERSION,
      plural: PLURAL,
      name,
      body: { spec: updates },
    });
    return response as Workspace;
  } catch (error) {
    console.error("patchWorkspace failed", error);
    return null;
  }
}

/**
 * Watch for Workspace changes (for cache invalidation).
 * Returns an async iterator of watch events.
 *
 * @param resourceVersion - Start watching from this resource version
 */
export async function watchWorkspaces(
  _resourceVersion?: string
): Promise<k8s.Watch> {
  const kc = new k8s.KubeConfig();

  try {
    kc.loadFromCluster();
  } catch {
    kc.loadFromDefault();
  }

  return new k8s.Watch(kc);
}

/**
 * Get the API path for watching workspaces.
 * Used with the Watch API.
 */
export function getWorkspaceWatchPath(): string {
  return `/apis/${GROUP}/${VERSION}/${PLURAL}`;
}

// Helper functions

function isNotFoundError(error: unknown): boolean {
  if (
    typeof error === "object" &&
    error !== null &&
    "statusCode" in error
  ) {
    return (error as { statusCode?: number }).statusCode === 404;
  }
  if (
    typeof error === "object" &&
    error !== null &&
    "response" in error &&
    typeof (error as { response: unknown }).response === "object" &&
    (error as { response: unknown }).response !== null
  ) {
    const response = (error as { response: { statusCode?: number } }).response;
    return response?.statusCode === 404;
  }
  return false;
}

function isConnectionError(error: unknown): boolean {
  if (error instanceof TypeError && error.message.includes("Invalid URL")) {
    return true;
  }
  if (
    typeof error === "object" &&
    error !== null &&
    "code" in error
  ) {
    const code = (error as { code?: string }).code;
    return code === "ECONNREFUSED" || code === "ENOTFOUND" || code === "ERR_INVALID_URL";
  }
  return false;
}

/**
 * Reset the client (for testing purposes).
 */
export function resetWorkspaceClient(): void {
  client = null;
  clientInitialized = false;
}
