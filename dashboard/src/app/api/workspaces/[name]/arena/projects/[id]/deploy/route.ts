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
import * as fs from "node:fs/promises";
import * as path from "node:path";

const RESOURCE_TYPE = "ArenaProjectDeploy";

// Base path for workspace content (mounted PVC).
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

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

/** Absolute path to the editable project dir on the mounted volume. */
function getProjectFsPath(workspaceName: string, namespace: string, projectId: string): string {
  return path.join(WORKSPACE_CONTENT_BASE, workspaceName, namespace, "arena", "projects", projectId);
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

      // Verify the project dir exists and is non-empty on the shared volume.
      const projectPath = getProjectFsPath(name, namespace, projectId);
      let entries: string[];
      try {
        entries = await fs.readdir(projectPath);
      } catch {
        return notFoundResponse(`Project not found: ${projectId}`);
      }
      if (entries.length === 0) {
        return NextResponse.json(
          { error: "Bad Request", message: "Project has no files to deploy" },
          { status: 400 }
        );
      }

      // Build a full workspace-type spec. A full replace (PUT) drops any stale
      // source block (e.g. a previous configmap deploy), satisfying the CRD's
      // one-of validation.
      const spec: ArenaSourceSpec = {
        type: "workspace",
        workspace: { path: projectRelPath(projectId) },
        targetPath: deployTargetPath(projectId),
        interval: body.syncInterval || "1h",
      };
      const labels = {
        [PROJECT_LABEL]: projectId,
        [MANAGED_BY_LABEL]: MANAGED_BY_VALUE,
      };

      const existingSource = await getCrd<ArenaSource>(
        result.clientOptions,
        CRD_ARENA_SOURCES,
        sourceName
      );

      let source: ArenaSource;
      let isNew = false;

      if (existingSource) {
        const updatedSource = {
          ...existingSource,
          spec: { ...spec, interval: body.syncInterval || existingSource.spec.interval || "1h" },
          metadata: {
            ...existingSource.metadata,
            labels: { ...existingSource.metadata.labels, ...labels },
          },
        };
        source = await updateCrd<ArenaSource>(
          result.clientOptions,
          CRD_ARENA_SOURCES,
          sourceName,
          updatedSource
        );
      } else {
        isNew = true;
        const newSource = buildCrdResource(
          "ArenaSource",
          name,
          namespace,
          sourceName,
          spec,
          labels
        );
        source = await createCrd<ArenaSource>(
          result.clientOptions,
          CRD_ARENA_SOURCES,
          newSource as unknown as ArenaSource
        );
      }

      const response: DeployResponse = { source, isNew };
      auditSuccess(auditCtx, isNew ? "create" : "update", sourceName, {
        fileCount: entries.length,
        targetPath: spec.targetPath,
      });

      return NextResponse.json(response, { status: isNew ? 201 : 200 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      return handleK8sError(error, "deploy project");
    }
  }
);
