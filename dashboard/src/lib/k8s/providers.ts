/**
 * Kubernetes Provider CRD management.
 *
 * Provides operations for updating Provider resources directly via K8s API.
 * Used for operations that don't require going through the operator.
 */

import * as k8s from "@kubernetes/client-node";

const GROUP = "omnia.altairalabs.ai";
const VERSION = "v1alpha1";
const PLURAL = "providers";

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
 * Provider resource from K8s API.
 */
interface ProviderResource {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace: string;
    resourceVersion?: string;
    [key: string]: unknown;
  };
  spec: {
    type: string;
    model?: string;
    secretRef?: {
      name: string;
      key?: string;
    };
    [key: string]: unknown;
  };
  status?: {
    phase?: string;
    [key: string]: unknown;
  };
}

/**
 * Get a Provider resource.
 */
export async function getProvider(
  namespace: string,
  name: string
): Promise<ProviderResource | null> {
  const k8sClient = getClient();

  try {
    const response = await k8sClient.getNamespacedCustomObject({
      group: GROUP,
      version: VERSION,
      namespace,
      plural: PLURAL,
      name,
    });
    return response as ProviderResource;
  } catch (error) {
    if (isNotFoundError(error)) {
      return null;
    }
    throw error;
  }
}

/**
 * Update the secretRef on a Provider.
 * Pass null to remove the secretRef (set to None).
 */
export async function updateProviderSecretRef(
  namespace: string,
  name: string,
  secretName: string | null
): Promise<ProviderResource> {
  const k8sClient = getClient();

  // Get the existing provider first
  const existing = await getProvider(namespace, name);
  if (!existing) {
    throw new Error(`Provider ${namespace}/${name} not found`);
  }

  // Update the secretRef
  if (secretName === null) {
    // Remove secretRef
    delete existing.spec.secretRef;
  } else {
    // Set secretRef
    existing.spec.secretRef = { name: secretName };
  }

  // Use replaceNamespacedCustomObject to update
  const response = await k8sClient.replaceNamespacedCustomObject({
    group: GROUP,
    version: VERSION,
    namespace,
    plural: PLURAL,
    name,
    body: existing,
  });

  return response as ProviderResource;
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
