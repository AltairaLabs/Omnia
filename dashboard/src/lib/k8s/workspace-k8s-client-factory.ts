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
import * as fs from "node:fs";
import type { WorkspaceRole } from "@/types/workspace";
import { getWorkspaceToken, refreshWorkspaceToken } from "./token-fetcher";

// In-cluster paths
const SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token";
const SA_CA_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt";
const SA_NAMESPACE_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/namespace";

/**
 * Check if we're running in a Kubernetes cluster and can use in-cluster config.
 */
function isInCluster(): boolean {
  const host = process.env.KUBERNETES_SERVICE_HOST;
  const port = process.env.KUBERNETES_SERVICE_PORT;
  return !!(host && port);
}

/**
 * Manually construct in-cluster KubeConfig.
 * This is a workaround for when loadFromCluster() doesn't work properly
 * (e.g., in Next.js/Turbopack environments).
 */
function loadInClusterConfig(): k8s.KubeConfig {
  const host = process.env.KUBERNETES_SERVICE_HOST;
  const port = process.env.KUBERNETES_SERVICE_PORT;

  if (!host || !port) {
    throw new Error("Not running in cluster: KUBERNETES_SERVICE_HOST/PORT not set");
  }

  // Read ServiceAccount token and CA
  let token: string;
  let caData: string;
  let _namespace: string;

  try {
    token = fs.readFileSync(SA_TOKEN_PATH, "utf8").trim();
  } catch (err) {
    throw new Error(`Failed to read SA token from ${SA_TOKEN_PATH}: ${err}`);
  }

  try {
    caData = fs.readFileSync(SA_CA_PATH, "utf8");
  } catch (err) {
    throw new Error(`Failed to read CA cert from ${SA_CA_PATH}: ${err}`);
  }

  try {
    _namespace = fs.readFileSync(SA_NAMESPACE_PATH, "utf8").trim();
  } catch {
    _namespace = "default";
  }

  const clusterName = "in-cluster";
  const userName = "in-cluster-user";

  // Use loadFromClusterAndUser for proper in-cluster setup
  const kc = new k8s.KubeConfig();
  kc.loadFromClusterAndUser(
    {
      name: clusterName,
      server: `https://${host}:${port}`,
      caData: Buffer.from(caData).toString("base64"),
      skipTLSVerify: false,
    },
    {
      name: userName,
      token,
    }
  );

  return kc;
}

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
 * Load base KubeConfig from cluster or local environment.
 * Returns the config and whether it was loaded from in-cluster.
 */
function loadBaseKubeConfig(): { config: k8s.KubeConfig; fromCluster: boolean } {
  // Try manual in-cluster loader first (more reliable in Next.js)
  if (isInCluster()) {
    try {
      return { config: loadInClusterConfig(), fromCluster: true };
    } catch (err) {
      console.warn(`Manual in-cluster config failed: ${err}`);
    }
  }

  // Try library methods
  const kc = new k8s.KubeConfig();
  try {
    kc.loadFromCluster();
    return { config: kc, fromCluster: true };
  } catch {
    // Not in cluster, try default kubeconfig
    try {
      kc.loadFromDefault();
      return { config: kc, fromCluster: false };
    } catch {
      throw new Error(
        "No Kubernetes configuration found. Ensure the dashboard is running in a cluster with a ServiceAccount or has access to a kubeconfig file."
      );
    }
  }
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

  const { config: baseKc, fromCluster } = loadBaseKubeConfig();

  // Verify we have a valid cluster configuration
  const currentCluster = baseKc.getCurrentCluster();
  if (!currentCluster) {
    const context = fromCluster ? " (in-cluster config may be incomplete)" : " in kubeconfig";
    throw new Error(`No active Kubernetes cluster found${context}`);
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
    const newKc = new k8s.KubeConfig();
    newKc.loadFromClusterAndUser(
      {
        name: currentCluster.name,
        server: currentCluster.server,
        caData: currentCluster.caData,
        caFile: currentCluster.caFile,
        skipTLSVerify: currentCluster.skipTLSVerify ?? false,
      },
      {
        name: `workspace-${workspace}-${role}`,
        token,
      }
    );
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
