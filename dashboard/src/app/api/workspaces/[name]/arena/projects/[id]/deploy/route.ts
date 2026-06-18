/**
 * API routes for deploying Arena projects as ArenaSource.
 *
 * POST /api/workspaces/:name/arena/projects/:id/deploy - Deploy project as ArenaSource
 *
 * The project files already live on the shared workspace-content volume, so
 * deploy simply creates/updates a `workspace`-type ArenaSource pointing at the
 * project dir. The arena-controller snapshots that dir into an immutable,
 * content-addressed version (see #1260) — no ConfigMap round-trip, no 1 MB cap.
 *
 * Protected by workspace access checks.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { createCrd, getCrd, updateCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  notFoundResponse,
  buildCrdResource,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaSourceSpec } from "@/types/arena";
import {
  ContentApiError,
  getContent,
  isContentListing,
} from "@/lib/data/content-api-service";
import { contentErrorResponse } from "@/lib/data/content-api-response";

const RESOURCE_TYPE = "ArenaProjectDeploy";

// Labels for tracking deployed resources.
const PROJECT_LABEL = "arena.omnia.altairalabs.ai/project-id";
const MANAGED_BY_LABEL = "app.kubernetes.io/managed-by";
const MANAGED_BY_VALUE = "omnia-arena";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

interface DeployRequest {
  name?: string;
  syncInterval?: string;
}

interface DeployResponse {
  source: ArenaSource;
  isNew: boolean;
}

/** Project dir relative to the workspace content root for this namespace. */
function projectRelPath(projectId: string): string {
  return `arena/projects/${projectId}`;
}

/**
 * Where versioned snapshots land — distinct from the source path so the
 * snapshot never nests inside the editable project dir.
 */
function deployTargetPath(projectId: string): string {
  return `arena/deployed/${projectId}`;
}

/** Result of the project content pre-check: a count, or an error response. */
type ContentCheck = { count: number } | { response: NextResponse };

/**
 * Verify the project dir exists and is non-empty via the content API. Returns
 * the entry count, or an error response (404 missing / 400 empty).
 */
async function checkProjectContent(
  workspace: string,
  user: User,
  projectId: string
): Promise<ContentCheck> {
  let count: number;
  try {
    const listing = await getContent(workspace, user, projectRelPath(projectId));
    count = isContentListing(listing) ? listing.entries.length : 0;
  } catch (error) {
    if (error instanceof ContentApiError && error.status === 404) {
      return { response: notFoundResponse(`Project not found: ${projectId}`) };
    }
    throw error;
  }
  if (count === 0) {
    return {
      response: NextResponse.json(
        { error: "Bad Request", message: "Project has no files to deploy" },
        { status: 400 }
      ),
    };
  }
  return { count };
}

type ClientOptions = Parameters<typeof getCrd>[0];

interface UpsertParams {
  workspace: string;
  namespace: string;
  sourceName: string;
  projectId: string;
  spec: ArenaSourceSpec;
  syncInterval?: string;
}

/**
 * Create or replace the project's ArenaSource. A full replace (PUT) drops any
 * stale source block from a prior deploy.
 */
async function upsertSource(
  clientOptions: ClientOptions,
  params: UpsertParams
): Promise<{ source: ArenaSource; isNew: boolean }> {
  const { workspace, namespace, sourceName, projectId, spec, syncInterval } = params;
  const labels = {
    [PROJECT_LABEL]: projectId,
    [MANAGED_BY_LABEL]: MANAGED_BY_VALUE,
  };

  const existingSource = await getCrd<ArenaSource>(clientOptions, CRD_ARENA_SOURCES, sourceName);

  if (existingSource) {
    const updatedSource = {
      ...existingSource,
      spec: { ...spec, interval: syncInterval || existingSource.spec.interval || "1h" },
      metadata: {
        ...existingSource.metadata,
        labels: { ...existingSource.metadata.labels, ...labels },
      },
    };
    const source = await updateCrd<ArenaSource>(
      clientOptions,
      CRD_ARENA_SOURCES,
      sourceName,
      updatedSource
    );
    return { source, isNew: false };
  }

  const newSource = buildCrdResource("ArenaSource", workspace, namespace, sourceName, spec, labels);
  const source = await createCrd<ArenaSource>(
    clientOptions,
    CRD_ARENA_SOURCES,
    newSource as unknown as ArenaSource
  );
  return { source, isNew: true };
}

/**
 * POST /api/workspaces/:name/arena/projects/:id/deploy
 *
 * Deploy a project as a `workspace`-type ArenaSource (in-volume snapshot).
 */
export const POST = withWorkspaceAccess<{ name: string; id: string }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, id: projectId } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      auditCtx = createAuditContext(name, namespace, user, access.role!, RESOURCE_TYPE);

      const body = (await request.json().catch(() => ({}))) as DeployRequest;
      const sourceName = body.name || `project-${projectId}`;

      const contentCheck = await checkProjectContent(name, user, projectId);
      if ("response" in contentCheck) return contentCheck.response;

      // Build a full workspace-type spec. A full replace (PUT) drops any stale
      // source block (e.g. a previous configmap deploy), satisfying the CRD's
      // one-of validation.
      const spec: ArenaSourceSpec = {
        type: "workspace",
        workspace: { path: projectRelPath(projectId) },
        targetPath: deployTargetPath(projectId),
        interval: body.syncInterval || "1h",
      };

      const { source, isNew } = await upsertSource(
        result.clientOptions,
        { workspace: name, namespace, sourceName, projectId, spec, syncInterval: body.syncInterval }
      );

      const response: DeployResponse = { source, isNew };
      auditSuccess(auditCtx, isNew ? "create" : "update", sourceName, {
        fileCount: contentCheck.count,
        targetPath: spec.targetPath,
      });

      return NextResponse.json(response, { status: isNew ? 201 : 200 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      if (error instanceof ContentApiError) {
        return contentErrorResponse(error, "Failed to deploy project");
      }
      return handleK8sError(error, "deploy project");
    }
  }
);
