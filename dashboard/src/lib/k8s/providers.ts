/**
 * Kubernetes Provider CRD management.
 *
 * Provides operations for updating Provider resources directly via K8s API.
 * Used for operations that don't require going through the operator.
 */

import * as k8s from "@kubernetes/client-node";

// Re-exported from a Node-free module so client components can read a
// Provider's effective secret name without dragging the k8s SDK into
// the browser bundle. See provider-secret-ref.ts for the rationale.
export { effectiveSecretRefName } from "./provider-secret-ref";

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
 *
 * The Provider CRD has TWO ways to express its credential reference:
 *
 *   spec.secretRef            — legacy (Phase 1, marked Deprecated in v1alpha1)
 *   spec.credential.secretRef — current shape (#1036)
 *
 * Operator's k8s/EffectiveSecretRef accepts either; readers should
 * prefer `effectiveSecretRefName(provider)` over poking at either
 * field directly so a Provider written in either shape works.
 *
 * Writers in this package always set the new shape and remove the
 * old one in the same patch so a single Provider never carries both
 * — the CRD validation rejects "both set" at admission.
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
    credential?: {
      secretRef?: {
        name: string;
        key?: string;
      };
      envVar?: string;
      filePath?: string;
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

  // Always write the new shape (spec.credential.secretRef). The CRD
  // validation rejects "both set", so we also clear the legacy field
  // in the same patch — without this, a Provider that started life
  // with spec.secretRef would fail to update.
  delete existing.spec.secretRef;
  if (secretName === null) {
    if (existing.spec.credential) {
      delete existing.spec.credential.secretRef;
      // Drop the credential block entirely if it's now empty so the
      // Provider doesn't carry an inert {} that confuses readers.
      if (
        Object.keys(existing.spec.credential).length === 0
      ) {
        delete existing.spec.credential;
      }
    }
  } else {
    if (!existing.spec.credential) {
      existing.spec.credential = {};
    }
    existing.spec.credential.secretRef = { name: secretName };
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
