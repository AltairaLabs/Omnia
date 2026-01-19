/**
 * Kubernetes Workspace CRD client.
 *
 * Provides operations for fetching Workspace resources from the K8s API.
 * Workspaces are cluster-scoped resources that define team/project boundaries.
 */

import * as k8s from "@kubernetes/client-node";
import type { Workspace } from "@/types/workspace";

const GROUP = "omnia.altairalabs.ai";
const VERSION = "v1alpha1";
const PLURAL = "workspaces";

/**
 * Create the K8s custom objects API client.
 * Uses in-cluster config when running in K8s, falls back to kubeconfig for local dev.
 */
function createCustomObjectsClient(): k8s.CustomObjectsApi {
  const kc = new k8s.KubeConfig();

  try {
    // Try in-cluster config first (when running in K8s)
    kc.loadFromCluster();
  } catch {
    // Fall back to default kubeconfig for local development
    kc.loadFromDefault();
  }

  return kc.makeApiClient(k8s.CustomObjectsApi);
}

// Singleton client
let client: k8s.CustomObjectsApi | null = null;

function getClient(): k8s.CustomObjectsApi {
  if (!client) {
    client = createCustomObjectsClient();
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

/**
 * Reset the client (for testing purposes).
 */
export function resetWorkspaceClient(): void {
  client = null;
}
