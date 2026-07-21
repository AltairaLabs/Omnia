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
import { OperatorApiError, asOperatorError, mintOperatorIdentityToken, operatorBaseURL } from "./operator-identity";

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

/** Wrap a shared OperatorApiError (config errors) as a DeployApiError so callers only see this file's error type. */
const asDeployError = <T>(fn: () => T): T =>
  asOperatorError(fn, (message, status) => new DeployApiError(message, status));

function identityToken(workspace: string, user: User): string {
  return asDeployError(() => mintOperatorIdentityToken(workspace, user));
}

function baseURL(): string {
  return asDeployError(() => operatorBaseURL("OPERATOR_DEPLOY_API_URL"));
}

/**
 * Best-effort read of an error response body, returned as a `: <trimmed>`
 * suffix (or "" when empty/unreadable) for embedding in an error message.
 */
async function readErrorBody(res: Response): Promise<string> {
  try {
    const body = (await res.text()).trim();
    return body ? `: ${body}` : "";
  } catch {
    return "";
  }
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
    // Surface the operator's error body (e.g. a version-negotiation rejection)
    // so callers get a diagnosable message, not just a bare status code.
    const detail = await readErrorBody(res);
    throw new DeployApiError(`deploy API POST ${url} -> ${res.status}${detail}`, res.status);
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
