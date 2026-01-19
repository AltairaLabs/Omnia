/**
 * Workspace-scoped Kubernetes client factory.
 *
 * Creates K8s API clients that use workspace ServiceAccount tokens
 * for authentication. This enables workspace-scoped permissions for
 * all K8s API calls.
 *
 * Usage:
 * ```typescript
 * const client = await getWorkspaceCustomObjectsApi("my-workspace", "editor");
 * const agents = await client.listNamespacedCustomObject({...});
 * ```
 */

import * as k8s from "@kubernetes/client-node";
import type { WorkspaceRole } from "@/types/workspace";
import { getWorkspaceToken, refreshWorkspaceToken } from "./token-fetcher";

/**
 * Options for creating workspace K8s clients.
 */
export interface WorkspaceClientOptions {
  /** Workspace name */
  workspace: string;
  /** Namespace where workspace resources reside */
  namespace: string;
  /** User's role in the workspace */
  role: WorkspaceRole;
}

/**
 * Create a KubeConfig configured with a workspace ServiceAccount token.
 *
 * Falls back to the dashboard's own ServiceAccount if workspace ServiceAccounts
 * are not available (e.g., during development without the workspace controller).
 *
 * @param options - Workspace client options
 * @returns Configured KubeConfig
 */
export async function getWorkspaceKubeConfig(
  options: WorkspaceClientOptions
): Promise<k8s.KubeConfig> {
  const { workspace, namespace, role } = options;

  // Create a base KubeConfig to get cluster info
  const baseKc = new k8s.KubeConfig();
  try {
    baseKc.loadFromCluster();
  } catch {
    baseKc.loadFromDefault();
  }

  // Try to get a workspace-scoped token
  let token: string | null = null;
  try {
    token = await getWorkspaceToken(workspace, namespace, role);
  } catch (error) {
    // If we can't get a workspace token (e.g., SA doesn't exist),
    // fall back to using the dashboard's own ServiceAccount
    console.warn(
      `Could not get workspace token for ${workspace}/${role}, using fallback: ${error instanceof Error ? error.message : error}`
    );
  }

  // If we got a token, create a new config with it
  if (token) {
    const currentCluster = baseKc.getCurrentCluster();
    if (!currentCluster) {
      throw new Error("No cluster found in kubeconfig");
    }

    const clusterName = currentCluster.name;
    const userName = `workspace-${workspace}-${role}`;
    const contextName = `${userName}-context`;

    const configObj = {
      clusters: [
        {
          name: clusterName,
          cluster: {
            server: currentCluster.server,
            "certificate-authority-data": currentCluster.caData,
            "certificate-authority": currentCluster.caFile,
            "insecure-skip-tls-verify": currentCluster.skipTLSVerify,
          },
        },
      ],
      users: [
        {
          name: userName,
          user: {
            token,
          },
        },
      ],
      contexts: [
        {
          name: contextName,
          context: {
            cluster: clusterName,
            user: userName,
            namespace,
          },
        },
      ],
      "current-context": contextName,
    };

    const newKc = new k8s.KubeConfig();
    newKc.loadFromOptions(configObj);
    return newKc;
  }

  // Fallback: use the base kubeconfig (dashboard's own SA)
  // This provides cluster-wide access, suitable for development
  return baseKc;
}

/**
 * Create a CustomObjectsApi client for a workspace.
 *
 * Use this for working with Omnia CRDs (AgentRuntimes, PromptPacks, etc.)
 *
 * @param options - Workspace client options
 * @returns CustomObjectsApi client
 */
export async function getWorkspaceCustomObjectsApi(
  options: WorkspaceClientOptions
): Promise<k8s.CustomObjectsApi> {
  const kc = await getWorkspaceKubeConfig(options);
  return kc.makeApiClient(k8s.CustomObjectsApi);
}

/**
 * Create a CoreV1Api client for a workspace.
 *
 * Use this for working with core K8s resources (ConfigMaps, Secrets, Pods, etc.)
 *
 * @param options - Workspace client options
 * @returns CoreV1Api client
 */
export async function getWorkspaceCoreApi(
  options: WorkspaceClientOptions
): Promise<k8s.CoreV1Api> {
  const kc = await getWorkspaceKubeConfig(options);
  return kc.makeApiClient(k8s.CoreV1Api);
}

/**
 * Create an AppsV1Api client for a workspace.
 *
 * Use this for working with apps resources (Deployments, ReplicaSets, etc.)
 *
 * @param options - Workspace client options
 * @returns AppsV1Api client
 */
export async function getWorkspaceAppsApi(
  options: WorkspaceClientOptions
): Promise<k8s.AppsV1Api> {
  const kc = await getWorkspaceKubeConfig(options);
  return kc.makeApiClient(k8s.AppsV1Api);
}

/**
 * Wrapper that handles token refresh on auth errors.
 *
 * If a K8s API call fails with a 401 error, this will refresh the token
 * and retry the call once.
 *
 * @param options - Workspace client options
 * @param fn - Function that makes the K8s API call
 * @returns Result of the API call
 */
export async function withTokenRefresh<T>(
  options: WorkspaceClientOptions,
  fn: () => Promise<T>
): Promise<T> {
  try {
    return await fn();
  } catch (error) {
    // Check if it's an auth error
    if (isAuthError(error)) {
      // Refresh the token
      await refreshWorkspaceToken(
        options.workspace,
        options.namespace,
        options.role
      );
      // Retry the call
      return await fn();
    }
    throw error;
  }
}

/**
 * Check if an error is an authentication error (401).
 */
function isAuthError(error: unknown): boolean {
  if (typeof error === "object" && error !== null) {
    // Check for statusCode property
    if ("statusCode" in error && (error as { statusCode?: number }).statusCode === 401) {
      return true;
    }
    // Check for response.statusCode
    if (
      "response" in error &&
      typeof (error as { response: unknown }).response === "object" &&
      (error as { response: unknown }).response !== null
    ) {
      const response = (error as { response: { statusCode?: number } }).response;
      if (response?.statusCode === 401) {
        return true;
      }
    }
  }
  return false;
}
