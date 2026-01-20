/**
 * Shared helpers for workspace API routes.
 *
 * Reduces duplication across workspace-scoped API routes by providing
 * common patterns for workspace validation, error responses, CRD operations,
 * and audit logging.
 */

import { NextResponse } from "next/server";
import { getWorkspace } from "./workspace-client";
import { extractK8sErrorMessage, isForbiddenError } from "./crd-operations";
import { getUser } from "@/lib/auth";
import {
  logCrdSuccess,
  logCrdDenied,
  logCrdError,
  logError,
  methodToAction,
  type AuditAction,
} from "@/lib/audit";
import type { Workspace, WorkspaceRole } from "@/types/workspace";
import type { WorkspaceClientOptions } from "./workspace-k8s-client-factory";
import type { User } from "@/lib/auth/types";

/**
 * System namespace for shared resources.
 */
export const SYSTEM_NAMESPACE = process.env.OMNIA_SYSTEM_NAMESPACE || "omnia-system";

/**
 * Result of workspace validation.
 */
export type WorkspaceResult =
  | { ok: true; workspace: Workspace; clientOptions: WorkspaceClientOptions }
  | { ok: false; response: NextResponse };

/**
 * Validate workspace exists and build client options.
 * Returns either the workspace with client options, or a 404 response.
 */
export async function validateWorkspace(
  workspaceName: string,
  role: WorkspaceRole
): Promise<WorkspaceResult> {
  const workspace = await getWorkspace(workspaceName);
  if (!workspace) {
    return {
      ok: false,
      response: notFoundResponse(`Workspace not found: ${workspaceName}`),
    };
  }

  return {
    ok: true,
    workspace,
    clientOptions: {
      workspace: workspaceName,
      namespace: workspace.spec.namespace.name,
      role,
    },
  };
}

/**
 * Standard 404 Not Found response.
 */
export function notFoundResponse(message: string): NextResponse {
  return NextResponse.json(
    { error: "Not Found", message },
    { status: 404 }
  );
}

/**
 * Standard 500 Internal Server Error response.
 */
export function serverErrorResponse(error: unknown, context: string): NextResponse {
  logError(context, error, "workspace-route");
  return NextResponse.json(
    {
      error: "Internal Server Error",
      message: extractK8sErrorMessage(error),
    },
    { status: 500 }
  );
}

/**
 * Standard 403 Forbidden response.
 */
export function forbiddenResponse(message: string): NextResponse {
  return NextResponse.json(
    { error: "Forbidden", message },
    { status: 403 }
  );
}

/**
 * Standard 401 Unauthorized response.
 */
export function unauthorizedResponse(): NextResponse {
  return NextResponse.json(
    { error: "Unauthorized", message: "Authentication required" },
    { status: 401 }
  );
}

/**
 * Result of authentication check.
 */
export type AuthResult =
  | { ok: true; user: User }
  | { ok: false; response: NextResponse };

/**
 * Require authentication for a request.
 * Returns the user if authenticated, or a 401 response if anonymous.
 */
export async function requireAuth(): Promise<AuthResult> {
  const user = await getUser();
  if (user.provider === "anonymous") {
    return { ok: false, response: unauthorizedResponse() };
  }
  return { ok: true, user };
}

/**
 * Handle K8s API errors with appropriate HTTP responses.
 * Returns a 403 for permission errors, 500 for others.
 */
export function handleK8sError(error: unknown, context: string): NextResponse {
  if (isForbiddenError(error)) {
    return forbiddenResponse(`Insufficient permissions to ${context}`);
  }
  return serverErrorResponse(error, `Failed to ${context}`);
}

/**
 * Result of workspace resource fetch.
 */
export type WorkspaceResourceResult<T> =
  | { ok: true; resource: T; workspace: Workspace; clientOptions: WorkspaceClientOptions }
  | { ok: false; response: NextResponse };

/**
 * Get a workspace resource by name with validation.
 * Combines workspace validation, resource fetch, and 404 handling.
 */
export async function getWorkspaceResource<T>(
  workspaceName: string,
  role: WorkspaceRole,
  crdPlural: string,
  resourceName: string,
  resourceLabel: string
): Promise<WorkspaceResourceResult<T>> {
  const { getCrd } = await import("./crd-operations");

  const validation = await validateWorkspace(workspaceName, role);
  if (!validation.ok) return validation;

  const resource = await getCrd<T>(validation.clientOptions, crdPlural, resourceName);
  if (!resource) {
    return {
      ok: false,
      response: notFoundResponse(`${resourceLabel} not found: ${resourceName}`),
    };
  }

  return {
    ok: true,
    resource,
    workspace: validation.workspace,
    clientOptions: validation.clientOptions,
  };
}

/**
 * CRD plural names for Omnia resources.
 */
export const CRD_AGENTS = "agentruntimes";
export const CRD_PROMPTPACKS = "promptpacks";

/**
 * CRD API version for Omnia resources.
 */
export const CRD_API_VERSION = "omnia.altairalabs.ai/v1alpha1";

/**
 * Workspace label key.
 */
export const WORKSPACE_LABEL = "omnia.altairalabs.ai/workspace";

/**
 * Build a CRD resource with standard metadata.
 */
export function buildCrdResource(
  kind: string,
  workspaceName: string,
  namespace: string,
  name: string,
  spec: unknown,
  labels?: Record<string, string>,
  annotations?: Record<string, string>
): { apiVersion: string; kind: string; metadata: Record<string, unknown>; spec: unknown } {
  return {
    apiVersion: CRD_API_VERSION,
    kind,
    metadata: {
      name,
      namespace,
      labels: {
        ...labels,
        [WORKSPACE_LABEL]: workspaceName,
      },
      annotations,
    },
    spec,
  };
}

/**
 * Audit logging context for route handlers.
 */
export interface AuditContext {
  workspace: string;
  namespace: string;
  user: User;
  role: WorkspaceRole;
  resourceType: string;
}

/**
 * Get user identifier for audit logging.
 * Prefers email, falls back to username, then "unknown".
 */
function getUserIdentifier(user: User): string {
  return user.email || user.username || "unknown";
}

/**
 * Log a successful CRD operation in a route handler.
 */
export function auditSuccess(
  ctx: AuditContext,
  action: AuditAction,
  resourceName?: string,
  metadata?: Record<string, unknown>
): void {
  logCrdSuccess({
    action,
    resourceType: ctx.resourceType,
    resourceName,
    workspace: ctx.workspace,
    namespace: ctx.namespace,
    user: getUserIdentifier(ctx.user),
    role: ctx.role,
    metadata,
  });
}

/**
 * Log a denied CRD operation in a route handler.
 */
export function auditDenied(
  ctx: AuditContext,
  action: AuditAction,
  resourceName?: string,
  errorMessage?: string
): void {
  logCrdDenied({
    action,
    resourceType: ctx.resourceType,
    resourceName,
    workspace: ctx.workspace,
    namespace: ctx.namespace,
    user: getUserIdentifier(ctx.user),
    role: ctx.role,
    errorMessage,
  });
}

/**
 * Log a failed CRD operation in a route handler.
 */
export function auditError(
  ctx: AuditContext,
  action: AuditAction,
  resourceName?: string,
  error?: unknown,
  statusCode?: number
): void {
  const errorMessage = error instanceof Error ? error.message : String(error);
  logCrdError({
    action,
    resourceType: ctx.resourceType,
    resourceName,
    workspace: ctx.workspace,
    namespace: ctx.namespace,
    user: getUserIdentifier(ctx.user),
    role: ctx.role,
    errorMessage,
    statusCode,
  });
}

/**
 * Create an audit context from route handler parameters.
 */
export function createAuditContext(
  workspace: string,
  namespace: string,
  user: User,
  role: WorkspaceRole,
  resourceType: string
): AuditContext {
  return { workspace, namespace, user, role, resourceType };
}

// Re-export methodToAction for convenience
export { methodToAction };
