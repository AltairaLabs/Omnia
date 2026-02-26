/**
 * CRD route factory for workspace and shared API routes.
 *
 * Eliminates boilerplate across 12+ route files by providing factory functions
 * that generate Next.js route handlers for standard CRD CRUD operations.
 *
 * Factory functions:
 * - createCollectionRoutes: GET (list) + POST (create) for workspace-scoped CRDs
 * - createItemRoutes: GET + PUT + DELETE for workspace-scoped CRDs
 * - createSharedCollectionRoutes: GET (list) for shared/system CRDs
 * - createSharedItemRoutes: GET for shared/system CRDs
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  listCrd,
  createCrd,
  updateCrd,
  deleteCrd,
  listSharedCrd,
  getSharedCrd,
} from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  getWorkspaceResource,
  serverErrorResponse,
  notFoundResponse,
  handleK8sError,
  buildCrdResource,
  WORKSPACE_LABEL,
  SYSTEM_NAMESPACE,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AuditAction } from "@/lib/audit";

/**
 * Metadata shape expected on CRD resources for update merging.
 */
interface CrdMetadata {
  name?: string;
  namespace?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/**
 * Minimal CRD resource shape used by the factory for update operations.
 */
interface CrdResource {
  metadata?: CrdMetadata;
  spec?: unknown;
}

/**
 * Configuration for workspace-scoped collection routes (list + create).
 */
export interface CollectionRouteConfig {
  /** CRD kind name (e.g., "AgentRuntime", "PromptPack") */
  kind: string;
  /** CRD plural name (e.g., "agentruntimes", "promptpacks") */
  plural: string;
  /** Human-readable label for error messages (e.g., "agents", "prompt packs") */
  errorLabel: string;
}

/**
 * Configuration for workspace-scoped item routes (get/update/delete).
 */
export interface ItemRouteConfig {
  /** CRD kind name */
  kind: string;
  /** CRD plural name */
  plural: string;
  /** Human-readable singular label for error messages (e.g., "Agent", "Prompt pack") */
  resourceLabel: string;
  /** The route param key for the resource name (e.g., "agentName", "packName") */
  paramKey: string;
  /** Human-readable label for K8s error context (e.g., "this agent", "provider") */
  errorLabel: string;
}

/**
 * Configuration for shared (system-namespace) collection routes.
 */
export interface SharedCollectionRouteConfig {
  /** CRD plural name */
  plural: string;
  /** Human-readable label for error messages */
  errorLabel: string;
}

/**
 * Configuration for shared (system-namespace) item routes.
 */
export interface SharedItemRouteConfig {
  /** CRD plural name */
  plural: string;
  /** The route param key for the resource name */
  paramKey: string;
  /** Human-readable singular label for error messages (e.g., "Provider") */
  resourceLabel: string;
  /** Human-readable label for error context */
  errorLabel: string;
}

// ─── Collection routes (workspace-scoped) ────────────────────────────────

/**
 * Create GET and POST handlers for workspace-scoped CRD collection routes.
 *
 * GET: Lists all resources of the given CRD type in the workspace namespace.
 * POST: Creates a new resource of the given CRD type in the workspace namespace.
 */
export function createCollectionRoutes<T>(config: CollectionRouteConfig) {
  const { kind, plural, errorLabel } = config;

  const GET = withWorkspaceAccess(
    "viewer",
    async (
      _request: NextRequest,
      context: WorkspaceRouteContext,
      access: WorkspaceAccess,
      user: User
    ): Promise<NextResponse> => {
      const { name } = await context.params;
      let auditCtx;

      try {
        const result = await validateWorkspace(name, access.role!);
        if (!result.ok) return result.response;

        auditCtx = createAuditContext(
          name,
          result.workspace.spec.namespace.name,
          user,
          access.role!,
          kind
        );

        const items = await listCrd<T>(result.clientOptions, plural);

        auditSuccess(auditCtx, "list", undefined, { count: items.length });
        return NextResponse.json(items);
      } catch (error) {
        if (auditCtx) {
          auditError(auditCtx, "list", undefined, error, 500);
        }
        return serverErrorResponse(error, `Failed to list ${errorLabel}`);
      }
    }
  );

  const POST = withWorkspaceAccess(
    "editor",
    async (
      request: NextRequest,
      context: WorkspaceRouteContext,
      access: WorkspaceAccess,
      user: User
    ): Promise<NextResponse> => {
      const { name } = await context.params;
      let auditCtx;
      let resourceName = "";

      try {
        const result = await validateWorkspace(name, access.role!);
        if (!result.ok) return result.response;

        auditCtx = createAuditContext(
          name,
          result.workspace.spec.namespace.name,
          user,
          access.role!,
          kind
        );

        const body = await request.json();
        resourceName = body.metadata?.name || body.name || "";

        const resource = buildCrdResource(
          kind,
          name,
          result.workspace.spec.namespace.name,
          resourceName,
          body.spec,
          body.metadata?.labels,
          body.metadata?.annotations
        );

        const created = await createCrd<T>(
          result.clientOptions,
          plural,
          resource as unknown as T
        );

        auditSuccess(auditCtx, "create", resourceName);
        return NextResponse.json(created, { status: 201 });
      } catch (error) {
        if (auditCtx) {
          auditError(auditCtx, "create", resourceName, error, 500);
        }
        return serverErrorResponse(error, `Failed to create ${errorLabel}`);
      }
    }
  );

  return { GET, POST };
}

// ─── Item routes (workspace-scoped) ──────────────────────────────────────

type ItemRouteParams = { name: string } & Record<string, string>;

/**
 * Wrap a workspace-scoped item action with common param extraction,
 * resource lookup, audit context creation, and error handling.
 */
function withItemAction<T>(
  config: ItemRouteConfig,
  role: "viewer" | "editor",
  action: AuditAction,
  handler: (ctx: {
    request: NextRequest;
    workspaceName: string;
    itemName: string;
    result: Extract<Awaited<ReturnType<typeof getWorkspaceResource<T>>>, { ok: true }>;
    auditCtx: ReturnType<typeof createAuditContext>;
  }) => Promise<NextResponse>
) {
  const { kind, plural, resourceLabel, paramKey, errorLabel } = config;

  return withWorkspaceAccess<ItemRouteParams>(
    role,
    async (
      request: NextRequest,
      context: WorkspaceRouteContext<ItemRouteParams>,
      access: WorkspaceAccess,
      user: User
    ): Promise<NextResponse> => {
      const params = await context.params;
      const workspaceName = params.name;
      const itemName = params[paramKey];
      let auditCtx;

      try {
        const result = await getWorkspaceResource<T>(
          workspaceName, access.role!, plural, itemName, resourceLabel
        );
        if (!result.ok) return result.response;

        auditCtx = createAuditContext(
          workspaceName, result.workspace.spec.namespace.name,
          user, access.role!, kind
        );

        return await handler({
          request, workspaceName, itemName,
          result: result as Extract<typeof result, { ok: true }>,
          auditCtx,
        });
      } catch (error) {
        if (auditCtx) {
          auditError(auditCtx, action, itemName, error, 500);
        }
        return handleK8sError(error, `${action} ${errorLabel}`);
      }
    }
  );
}

/**
 * Wrap a workspace-scoped delete action with common setup.
 * Delete uses validateWorkspace instead of getWorkspaceResource.
 */
function withDeleteAction(
  config: ItemRouteConfig,
  handler: (ctx: {
    workspaceName: string;
    itemName: string;
    clientOptions: Awaited<ReturnType<typeof validateWorkspace>> extends { ok: true; clientOptions: infer C } ? C : never;
    auditCtx: ReturnType<typeof createAuditContext>;
  }) => Promise<NextResponse>
) {
  const { kind, paramKey, errorLabel } = config;

  return withWorkspaceAccess<ItemRouteParams>(
    "editor",
    async (
      _request: NextRequest,
      context: WorkspaceRouteContext<ItemRouteParams>,
      access: WorkspaceAccess,
      user: User
    ): Promise<NextResponse> => {
      const params = await context.params;
      const workspaceName = params.name;
      const itemName = params[paramKey];
      let auditCtx;

      try {
        const result = await validateWorkspace(workspaceName, access.role!);
        if (!result.ok) return result.response;

        auditCtx = createAuditContext(
          workspaceName, result.workspace.spec.namespace.name,
          user, access.role!, kind
        );

        return await handler({
          workspaceName, itemName,
          clientOptions: result.clientOptions as never,
          auditCtx,
        });
      } catch (error) {
        if (auditCtx) {
          auditError(auditCtx, "delete", itemName, error, 500);
        }
        return handleK8sError(error, `delete ${errorLabel}`);
      }
    }
  );
}

/**
 * Create GET, PUT, and DELETE handlers for workspace-scoped CRD item routes.
 *
 * GET: Fetches a single resource by name from the workspace namespace.
 * PUT: Updates the resource (merges metadata, replaces spec).
 * DELETE: Deletes the resource from the workspace namespace.
 */
export function createItemRoutes<T extends CrdResource>(config: ItemRouteConfig) {
  const GET = withItemAction<T>(config, "viewer", "get", async ({ itemName, auditCtx, result }) => {
    auditSuccess(auditCtx, "get", itemName);
    return NextResponse.json(result.resource);
  });

  const PUT = withItemAction<T>(config, "editor", "update", async ({ request, workspaceName, itemName, result, auditCtx }) => {
    const body = await request.json();
    const updated: T = {
      ...result.resource,
      metadata: {
        ...result.resource.metadata,
        labels: {
          ...result.resource.metadata?.labels,
          ...body.metadata?.labels,
          [WORKSPACE_LABEL]: workspaceName,
        },
        annotations: {
          ...result.resource.metadata?.annotations,
          ...body.metadata?.annotations,
        },
      },
      spec: body.spec || result.resource.spec,
    };

    const saved = await updateCrd<T>(result.clientOptions, config.plural, itemName, updated);
    auditSuccess(auditCtx, "update", itemName);
    return NextResponse.json(saved);
  });

  const DELETE = withDeleteAction(config, async ({ itemName, clientOptions, auditCtx }) => {
    await deleteCrd(clientOptions as never, config.plural, itemName);
    auditSuccess(auditCtx, "delete", itemName);
    return new NextResponse(null, { status: 204 });
  });

  return { GET, PUT, DELETE };
}

// ─── Shared collection routes (system namespace) ─────────────────────────

/**
 * Create a GET handler for shared (system-namespace) CRD collection routes.
 *
 * No authentication required - shared resources are read-only configuration data.
 */
export function createSharedCollectionRoutes<T>(config: SharedCollectionRouteConfig) {
  const { plural, errorLabel } = config;

  async function GET(): Promise<NextResponse> {
    try {
      const items = await listSharedCrd<T>(plural, SYSTEM_NAMESPACE);
      return NextResponse.json(items);
    } catch (error) {
      return serverErrorResponse(error, `Failed to list ${errorLabel}`);
    }
  }

  return { GET };
}

// ─── Shared item routes (system namespace) ───────────────────────────────

/**
 * Create a GET handler for shared (system-namespace) CRD item routes.
 *
 * No authentication required - shared resources are read-only configuration data.
 */
export function createSharedItemRoutes<T>(config: SharedItemRouteConfig) {
  const { plural, paramKey, resourceLabel, errorLabel } = config;

  interface RouteContext {
    params: Promise<Record<string, string>>;
  }

  async function GET(
    _request: NextRequest,
    context: RouteContext
  ): Promise<NextResponse> {
    try {
      const params = await context.params;
      const itemName = params[paramKey];

      const item = await getSharedCrd<T>(plural, SYSTEM_NAMESPACE, itemName);

      if (!item) {
        return notFoundResponse(`${resourceLabel} not found: ${itemName}`);
      }

      return NextResponse.json(item);
    } catch (error) {
      return serverErrorResponse(error, `Failed to get ${errorLabel}`);
    }
  }

  return { GET };
}
