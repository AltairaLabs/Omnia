/**
 * Client for the operator's deploy-intent API.
 *
 * The dashboard forwards a versioned, CRD-agnostic DeployIntent (the
 * operator's `internal/api/deploy` package owns the schema) rather than
 * constructing AgentRuntime/PromptPack/etc CRDs itself. This service mints a
 * short-lived RS256 identity JWT (carrying the authenticated user's identity
 * + groups) signed with the dashboard's mgmt-plane key and POSTs the intent
 * opaquely — the dashboard intentionally does NOT maintain a parallel
 * DeployIntent TS type, since that coupling is what this decoupling removes.
 *
 * Server-only: reads the signing key off disk and never runs in the browser.
 */

import type { User } from "@/lib/auth/types";
import { OperatorApiError, mintOperatorIdentityToken, operatorBaseURL } from "./operator-identity";

/** Per-object apply outcome, mirroring Go's deploy.ResourceResult json shape. */
export interface DeployResourceResult {
  kind: string;
  name: string;
  action: string;
  error?: string;
}

/** Deploy endpoint response: best-effort per-resource status, mirroring Go's deploy.DeployResult. */
export interface DeployResult {
  succeeded: boolean;
  results: DeployResourceResult[];
}

/**
 * Error carrying the operator's HTTP status so route handlers can pass through
 * 400 / 401 / 403 instead of collapsing everything to 500.
 */
export class DeployApiError extends OperatorApiError {
  constructor(message: string, status: number) {
    super(message, status);
    this.name = "DeployApiError";
  }
}

/** Re-throw a shared OperatorApiError (config errors) as a DeployApiError so callers only see this file's error type. */
function asDeployError<T>(fn: () => T): T {
  try {
    return fn();
  } catch (err) {
    if (err instanceof OperatorApiError && !(err instanceof DeployApiError)) {
      throw new DeployApiError(err.message, err.status);
    }
    throw err;
  }
}

function identityToken(workspace: string, user: User): string {
  return asDeployError(() => mintOperatorIdentityToken(workspace, user));
}

function baseURL(): string {
  return asDeployError(() => operatorBaseURL("OPERATOR_DEPLOY_API_URL"));
}

async function deployRequest(
  workspace: string,
  user: User,
  intent: unknown,
): Promise<{ status: number; result: DeployResult }> {
  const token = identityToken(workspace, user);
  const url = `${baseURL()}/api/v1/workspaces/${encodeURIComponent(workspace)}/deployments`;
  const res = await fetch(url, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify(intent),
  });
  // 207 (multi-status) is a success outcome — the operator applied objects
  // best-effort and some resources failed; res.ok is true for the whole
  // 200-299 range so this only throws on genuine 4xx/5xx.
  if (!res.ok) {
    throw new DeployApiError(`deploy API POST ${url} -> ${res.status}`, res.status);
  }
  return { status: res.status, result: (await res.json()) as DeployResult };
}

/** POST a DeployIntent (forwarded opaquely) to the operator for the given workspace. */
export function postDeployment(
  workspace: string,
  user: User,
  intent: unknown,
): Promise<{ status: number; result: DeployResult }> {
  return deployRequest(workspace, user, intent);
}
