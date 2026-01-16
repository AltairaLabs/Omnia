/**
 * Kubernetes secrets management for provider credentials.
 *
 * Security: This module NEVER returns secret values. Only metadata (names, keys, timestamps).
 *
 * Label filtering: Only manages secrets with label `omnia.altairalabs.ai/type=credentials`.
 * This provides defense-in-depth beyond K8s RBAC.
 */

import * as k8s from "@kubernetes/client-node";

// Label used to identify secrets managed by this system
export const CREDENTIALS_LABEL = "omnia.altairalabs.ai/type";
export const CREDENTIALS_LABEL_VALUE = "credentials";
export const PROVIDER_ANNOTATION = "omnia.altairalabs.ai/provider";

/**
 * Reference to a provider that uses a secret.
 */
export interface ProviderRef {
  namespace: string;
  name: string;
  type: string;
}

/**
 * Secret summary - metadata only, no values.
 */
export interface SecretSummary {
  namespace: string;
  name: string;
  keys: string[];
  annotations?: Record<string, string>;
  referencedBy: ProviderRef[];
  createdAt: string;
  modifiedAt: string;
}

/**
 * Request to create or update a secret.
 */
export interface SecretWriteRequest {
  namespace: string;
  name: string;
  data: Record<string, string>;
  providerType?: string;
}

/**
 * Create the K8s API client.
 * Uses in-cluster config when running in K8s, falls back to kubeconfig for local dev.
 */
function createK8sClient(): k8s.CoreV1Api {
  const kc = new k8s.KubeConfig();

  try {
    // Try in-cluster config first (when running in K8s)
    kc.loadFromCluster();
  } catch {
    // Fall back to default kubeconfig for local development
    kc.loadFromDefault();
  }

  return kc.makeApiClient(k8s.CoreV1Api);
}

/**
 * Create a custom objects API client for fetching Provider CRDs.
 */
function createCustomObjectsClient(): k8s.CustomObjectsApi {
  const kc = new k8s.KubeConfig();

  try {
    kc.loadFromCluster();
  } catch {
    kc.loadFromDefault();
  }

  return kc.makeApiClient(k8s.CustomObjectsApi);
}

// Singleton clients
let coreClient: k8s.CoreV1Api | null = null;
let customClient: k8s.CustomObjectsApi | null = null;

function getCoreClient(): k8s.CoreV1Api {
  if (!coreClient) {
    coreClient = createK8sClient();
  }
  return coreClient;
}

function getCustomClient(): k8s.CustomObjectsApi {
  if (!customClient) {
    customClient = createCustomObjectsClient();
  }
  return customClient;
}

/**
 * Fetch all Provider CRDs to determine which secrets are referenced.
 */
async function getProviderReferences(): Promise<Map<string, ProviderRef[]>> {
  const client = getCustomClient();
  const references = new Map<string, ProviderRef[]>();

  try {
    const response = await client.listClusterCustomObject({
      group: "omnia.altairalabs.ai",
      version: "v1alpha1",
      plural: "providers",
    });

    const providers = (response as { items?: Array<{
      metadata?: { namespace?: string; name?: string };
      spec?: { type?: string; secretRef?: { name?: string } };
    }> }).items || [];

    for (const provider of providers) {
      const secretName = provider.spec?.secretRef?.name;
      const namespace = provider.metadata?.namespace || "default";
      const providerName = provider.metadata?.name || "";
      const providerType = provider.spec?.type || "";

      if (secretName) {
        const key = `${namespace}/${secretName}`;
        const refs = references.get(key) || [];
        refs.push({
          namespace,
          name: providerName,
          type: providerType,
        });
        references.set(key, refs);
      }
    }
  } catch (error) {
    // Log but don't fail - providers list may be unavailable
    console.warn("Failed to fetch providers for references:", error);
  }

  return references;
}

/**
 * List secrets with the credentials label.
 * Returns metadata only - never secret values.
 */
export async function listSecrets(namespace?: string): Promise<SecretSummary[]> {
  const client = getCoreClient();
  const labelSelector = `${CREDENTIALS_LABEL}=${CREDENTIALS_LABEL_VALUE}`;

  // Fetch provider references for the "referencedBy" field
  const providerRefs = await getProviderReferences();

  let secrets: k8s.V1Secret[] = [];

  if (namespace) {
    // List secrets in specific namespace
    const response = await client.listNamespacedSecret({
      namespace,
      labelSelector,
    });
    secrets = response.items;
  } else {
    // List secrets across all namespaces
    const response = await client.listSecretForAllNamespaces({
      labelSelector,
    });
    secrets = response.items;
  }

  return secrets.map((secret) => {
    const ns = secret.metadata?.namespace || "default";
    const name = secret.metadata?.name || "";
    const key = `${ns}/${name}`;

    return {
      namespace: ns,
      name,
      keys: Object.keys(secret.data || {}),
      annotations: filterOmniaAnnotations(secret.metadata?.annotations),
      referencedBy: providerRefs.get(key) || [],
      createdAt: secret.metadata?.creationTimestamp?.toISOString() || "",
      modifiedAt: getModifiedAt(secret),
    };
  });
}

/**
 * Get a single secret's metadata.
 * Returns metadata only - never secret values.
 */
export async function getSecret(
  namespace: string,
  name: string
): Promise<SecretSummary | null> {
  const client = getCoreClient();

  try {
    const secret = await client.readNamespacedSecret({
      namespace,
      name,
    });

    // Verify it has our label
    const labels = secret.metadata?.labels || {};
    if (labels[CREDENTIALS_LABEL] !== CREDENTIALS_LABEL_VALUE) {
      return null; // Not a managed credential secret
    }

    const providerRefs = await getProviderReferences();
    const key = `${namespace}/${name}`;

    return {
      namespace,
      name,
      keys: Object.keys(secret.data || {}),
      annotations: filterOmniaAnnotations(secret.metadata?.annotations),
      referencedBy: providerRefs.get(key) || [],
      createdAt: secret.metadata?.creationTimestamp?.toISOString() || "",
      modifiedAt: getModifiedAt(secret),
    };
  } catch (error) {
    if (isNotFoundError(error)) {
      return null;
    }
    throw error;
  }
}

/**
 * Create or update a secret.
 * Creates if it doesn't exist, updates if it does.
 */
export async function createOrUpdateSecret(
  request: SecretWriteRequest
): Promise<SecretSummary> {
  const client = getCoreClient();
  const { namespace, name, data, providerType } = request;

  // Convert string values to base64
  const encodedData: Record<string, string> = {};
  for (const [key, value] of Object.entries(data)) {
    encodedData[key] = Buffer.from(value).toString("base64");
  }

  const labels: Record<string, string> = {
    [CREDENTIALS_LABEL]: CREDENTIALS_LABEL_VALUE,
  };

  const annotations: Record<string, string> = {};
  if (providerType) {
    annotations[PROVIDER_ANNOTATION] = providerType;
  }

  const secretBody: k8s.V1Secret = {
    apiVersion: "v1",
    kind: "Secret",
    metadata: {
      name,
      namespace,
      labels,
      annotations: Object.keys(annotations).length > 0 ? annotations : undefined,
    },
    type: "Opaque",
    data: encodedData,
  };

  try {
    // Try to get existing secret first
    const existing = await client.readNamespacedSecret({
      namespace,
      name,
    });

    // Verify it has our label before updating
    const existingLabels = existing.metadata?.labels || {};
    if (existingLabels[CREDENTIALS_LABEL] !== CREDENTIALS_LABEL_VALUE) {
      throw new Error(
        `Secret ${namespace}/${name} exists but is not a managed credential secret`
      );
    }

    // Update existing secret - merge with existing data if partial update
    const mergedData = { ...existing.data, ...encodedData };
    secretBody.data = mergedData;
    secretBody.metadata!.resourceVersion = existing.metadata?.resourceVersion;

    // Preserve existing annotations and merge
    const existingAnnotations = existing.metadata?.annotations || {};
    secretBody.metadata!.annotations = { ...existingAnnotations, ...annotations };

    await client.replaceNamespacedSecret({
      namespace,
      name,
      body: secretBody,
    });
  } catch (error) {
    if (isNotFoundError(error)) {
      // Create new secret
      await client.createNamespacedSecret({
        namespace,
        body: secretBody,
      });
    } else {
      throw error;
    }
  }

  // Return the created/updated secret metadata
  const result = await getSecret(namespace, name);
  if (!result) {
    throw new Error("Failed to read secret after create/update");
  }
  return result;
}

/**
 * Delete a secret.
 * Only deletes if it has the credentials label.
 */
export async function deleteSecret(
  namespace: string,
  name: string
): Promise<boolean> {
  const client = getCoreClient();

  try {
    // Verify it has our label before deleting
    const existing = await client.readNamespacedSecret({
      namespace,
      name,
    });

    const labels = existing.metadata?.labels || {};
    if (labels[CREDENTIALS_LABEL] !== CREDENTIALS_LABEL_VALUE) {
      throw new Error(
        `Secret ${namespace}/${name} is not a managed credential secret`
      );
    }

    await client.deleteNamespacedSecret({
      namespace,
      name,
    });

    return true;
  } catch (error) {
    if (isNotFoundError(error)) {
      return false;
    }
    throw error;
  }
}

/**
 * List all namespaces (for the namespace dropdown).
 */
export async function listNamespaces(): Promise<string[]> {
  const client = getCoreClient();
  const response = await client.listNamespace();
  return response.items
    .map((ns) => ns.metadata?.name)
    .filter((name): name is string => !!name)
    .sort();
}

// Helper functions

function filterOmniaAnnotations(
  annotations?: Record<string, string>
): Record<string, string> | undefined {
  if (!annotations) return undefined;

  const filtered: Record<string, string> = {};
  for (const [key, value] of Object.entries(annotations)) {
    if (key.startsWith("omnia.altairalabs.ai/")) {
      filtered[key] = value;
    }
  }
  return Object.keys(filtered).length > 0 ? filtered : undefined;
}

function getModifiedAt(secret: k8s.V1Secret): string {
  // Use last-applied-configuration timestamp if available, otherwise creation
  const managedFields = secret.metadata?.managedFields || [];
  if (managedFields.length > 0) {
    const lastField = managedFields[managedFields.length - 1];
    if (lastField.time) {
      return lastField.time.toISOString();
    }
  }
  return secret.metadata?.creationTimestamp?.toISOString() || "";
}

function isNotFoundError(error: unknown): boolean {
  // Check for response body with statusCode 404
  if (
    typeof error === "object" &&
    error !== null &&
    "statusCode" in error
  ) {
    return (error as { statusCode?: number }).statusCode === 404;
  }
  // Check for nested response object with statusCode
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
