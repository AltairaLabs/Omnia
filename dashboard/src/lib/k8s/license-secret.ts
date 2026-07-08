/**
 * Runtime management of the Omnia license Secret.
 *
 * The operator validates the license offline from a Kubernetes Secret with a
 * fixed name/key/namespace (see ee/pkg/license/validator.go: LicenseSecretName
 * "arena-license", LicenseSecretKey "license", in the operator namespace). This
 * module is the runtime equivalent of the chart's license-secret.yaml template:
 * it writes an uploaded JWT into that exact Secret so the dashboard "Upload
 * License" flow actually takes effect.
 *
 * The operator re-reads the Secret every DefaultCacheTTL (5 minutes), so a
 * freshly-written license can take up to that long to reflect in the GET path.
 */

import * as k8s from "@kubernetes/client-node";
import { extractStatusCode } from "./k8s-errors";

/** Fixed Secret name the operator reads the license from. */
export const LICENSE_SECRET_NAME = "arena-license";
/** Key within the Secret holding the JWT. */
export const LICENSE_SECRET_KEY = "license";
/**
 * Namespace holding the license Secret. Matches the operator's
 * LicenseSecretNamespace ("omnia-system") and the shared SYSTEM_NAMESPACE in
 * workspace-route-helpers; inlined here to keep this helper free of the heavy
 * next/server + auth import chain that module carries.
 */
const SYSTEM_NAMESPACE = process.env.OMNIA_SYSTEM_NAMESPACE || "omnia-system";

/** Claims decoded from an uploaded license JWT (best-effort, unverified). */
export interface LicenseClaimsSummary {
  tier: string;
  customer: string;
  expiresAt: string | null;
}

/**
 * Structurally validate and decode a license JWT without verifying its
 * signature (the operator does the cryptographic validation on read). Throws a
 * descriptive Error when the input is not a well-formed license token, so the
 * upload flow surfaces a real message instead of silently writing garbage.
 */
export function parseLicenseJwt(raw: string): LicenseClaimsSummary {
  const token = raw.trim();
  if (!token) {
    throw new Error("License is empty");
  }

  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new Error("License is not a valid JWT (expected 3 dot-separated segments)");
  }

  let payload: Record<string, unknown>;
  try {
    const json = Buffer.from(
      parts[1].replaceAll("-", "+").replaceAll("_", "/"),
      "base64"
    ).toString("utf8");
    payload = JSON.parse(json) as Record<string, unknown>;
  } catch {
    throw new Error("License payload is not decodable JSON");
  }

  const tier = typeof payload.tier === "string" ? payload.tier : "";
  if (!tier) {
    throw new Error("License is missing a 'tier' claim");
  }

  const customer = typeof payload.customer === "string" ? payload.customer : "";
  const expiresAt =
    typeof payload.exp === "number"
      ? new Date(payload.exp * 1000).toISOString()
      : null;

  return { tier, customer, expiresAt };
}

/**
 * Build a CoreV1Api client. In-cluster config when running in K8s, kubeconfig
 * fallback for local dev — mirrors lib/k8s/secrets.ts.
 */
function buildClient(): k8s.CoreV1Api {
  const kc = new k8s.KubeConfig();
  try {
    kc.loadFromCluster();
  } catch {
    kc.loadFromDefault();
  }
  return kc.makeApiClient(k8s.CoreV1Api);
}

let cachedClient: k8s.CoreV1Api | null = null;

function getClient(): k8s.CoreV1Api {
  if (!cachedClient) {
    cachedClient = buildClient();
  }
  return cachedClient;
}

/**
 * Write the license JWT into the operator's arena-license Secret, creating it
 * if absent and replacing it otherwise. The client is injectable for tests.
 */
export async function writeLicenseSecret(
  jwt: string,
  api: k8s.CoreV1Api = getClient()
): Promise<void> {
  const name = LICENSE_SECRET_NAME;
  const namespace = SYSTEM_NAMESPACE;

  const body: k8s.V1Secret = {
    apiVersion: "v1",
    kind: "Secret",
    metadata: { name, namespace },
    type: "Opaque",
    data: { [LICENSE_SECRET_KEY]: Buffer.from(jwt).toString("base64") },
  };

  try {
    const existing = await api.readNamespacedSecret({ name, namespace });
    body.metadata!.resourceVersion = existing.metadata?.resourceVersion;
    await api.replaceNamespacedSecret({ name, namespace, body });
  } catch (error) {
    if (extractStatusCode(error) === 404) {
      await api.createNamespacedSecret({ namespace, body });
      return;
    }
    throw error;
  }
}
