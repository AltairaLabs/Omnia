/**
 * TokenRequest API client for fetching workspace ServiceAccount tokens.
 *
 * Uses the Kubernetes TokenRequest API to get short-lived tokens for
 * workspace ServiceAccounts. Tokens are valid for 1 hour by default.
 *
 * Environment variables for dev mode:
 * - OMNIA_K8S_DEV_MODE=true - Use static token instead of TokenRequest
 * - OMNIA_K8S_DEV_TOKEN - Token to use in dev mode
 */

import * as k8s from "@kubernetes/client-node";
import * as fs from "fs";
import type { WorkspaceRole } from "@/types/workspace";
import {
  getCachedToken,
  setCachedToken,
  invalidateToken,
} from "./token-cache";

/** Token expiration time in seconds (1 hour) */
const TOKEN_EXPIRATION_SECONDS = 3600;

// In-cluster paths
const SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token";
const SA_CA_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt";
const SA_NAMESPACE_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/namespace";

/** Singleton KubeConfig */
let kubeConfig: k8s.KubeConfig | null = null;

/**
 * Check if we're running in a Kubernetes cluster.
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
 * Get or create the KubeConfig.
 */
function getKubeConfig(): k8s.KubeConfig {
  if (!kubeConfig) {
    // First, check if we're in a cluster and try our manual loader
    if (isInCluster()) {
      try {
        kubeConfig = loadInClusterConfig();
        return kubeConfig;
      } catch (err) {
        console.warn(`Manual in-cluster config failed: ${err}`);
      }
    }

    // Fall back to library methods
    kubeConfig = new k8s.KubeConfig();
    try {
      // Try in-cluster config first (when running in K8s)
      kubeConfig.loadFromCluster();
    } catch {
      // Fall back to default kubeconfig for local development
      kubeConfig.loadFromDefault();
    }
  }
  return kubeConfig;
}

/**
 * Get the ServiceAccount name for a workspace and role.
 *
 * @param workspaceName - Name of the workspace
 * @param role - Workspace role
 * @returns ServiceAccount name
 */
export function getServiceAccountName(
  workspaceName: string,
  role: WorkspaceRole
): string {
  return `workspace-${workspaceName}-${role}-sa`;
}

/**
 * Fetch a token for a workspace ServiceAccount using the TokenRequest API.
 *
 * This makes a request to the K8s API to get a short-lived token for the
 * specified ServiceAccount. The token can then be used to make K8s API
 * calls with the permissions of that ServiceAccount.
 *
 * @param workspaceName - Name of the workspace
 * @param namespace - Namespace where the SA resides (usually same as workspace)
 * @param role - Workspace role (owner, editor, viewer)
 * @returns Token string and expiration timestamp
 */
export async function fetchServiceAccountToken(
  workspaceName: string,
  namespace: string,
  role: WorkspaceRole
): Promise<{ token: string; expiresAt: number }> {
  // Check for dev mode
  if (process.env.OMNIA_K8S_DEV_MODE === "true") {
    const devToken = process.env.OMNIA_K8S_DEV_TOKEN;
    if (!devToken) {
      throw new Error(
        "OMNIA_K8S_DEV_MODE is enabled but OMNIA_K8S_DEV_TOKEN is not set"
      );
    }
    // Return dev token with 1 hour expiry
    return {
      token: devToken,
      expiresAt: Date.now() + TOKEN_EXPIRATION_SECONDS * 1000,
    };
  }

  const kc = getKubeConfig();
  const coreApi = kc.makeApiClient(k8s.CoreV1Api);
  const saName = getServiceAccountName(workspaceName, role);

  // Create TokenRequest body
  const tokenRequest: k8s.AuthenticationV1TokenRequest = {
    apiVersion: "authentication.k8s.io/v1",
    kind: "TokenRequest",
    spec: {
      audiences: [], // Empty means token is valid for default API server audience
      expirationSeconds: TOKEN_EXPIRATION_SECONDS,
    },
  };

  try {
    const response = await coreApi.createNamespacedServiceAccountToken({
      name: saName,
      namespace,
      body: tokenRequest,
    });

    const token = response.status?.token;
    if (!token) {
      throw new Error("TokenRequest response did not contain a token");
    }

    // Calculate expiration time from response or use default
    const expirationTimestamp = response.status?.expirationTimestamp;
    const expiresAt = expirationTimestamp
      ? new Date(expirationTimestamp).getTime()
      : Date.now() + TOKEN_EXPIRATION_SECONDS * 1000;

    return { token, expiresAt };
  } catch (error) {
    // Wrap K8s API errors with more context
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(
      `Failed to fetch token for SA ${saName} in namespace ${namespace}: ${message}`
    );
  }
}

/**
 * Get a token for a workspace ServiceAccount, using cache when available.
 *
 * This is the main entry point for getting workspace tokens. It checks the
 * cache first and only fetches a new token if needed.
 *
 * @param workspaceName - Name of the workspace
 * @param namespace - Namespace where the SA resides
 * @param role - Workspace role (owner, editor, viewer)
 * @returns Token string
 */
export async function getWorkspaceToken(
  workspaceName: string,
  namespace: string,
  role: WorkspaceRole
): Promise<string> {
  // Check cache first
  const cachedToken = getCachedToken(workspaceName, role);
  if (cachedToken) {
    return cachedToken;
  }

  // Fetch new token
  const { token, expiresAt } = await fetchServiceAccountToken(
    workspaceName,
    namespace,
    role
  );

  // Cache the token
  setCachedToken(workspaceName, role, token, expiresAt);

  return token;
}

/**
 * Refresh a token for a workspace ServiceAccount.
 *
 * Forces a new token fetch even if a cached token exists.
 * Use this when a cached token has been rejected (e.g., 401 error).
 *
 * @param workspaceName - Name of the workspace
 * @param namespace - Namespace where the SA resides
 * @param role - Workspace role (owner, editor, viewer)
 * @returns New token string
 */
export async function refreshWorkspaceToken(
  workspaceName: string,
  namespace: string,
  role: WorkspaceRole
): Promise<string> {
  // Invalidate cached token
  invalidateToken(workspaceName, role);

  // Fetch new token
  const { token, expiresAt } = await fetchServiceAccountToken(
    workspaceName,
    namespace,
    role
  );

  // Cache the new token
  setCachedToken(workspaceName, role, token, expiresAt);

  return token;
}

/**
 * Reset the KubeConfig (for testing purposes).
 */
export function resetKubeConfig(): void {
  kubeConfig = null;
}
