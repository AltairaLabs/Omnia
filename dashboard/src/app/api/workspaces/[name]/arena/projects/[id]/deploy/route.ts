/**
 * API routes for deploying Arena projects as ArenaSource.
 *
 * POST /api/workspaces/:name/arena/projects/:id/deploy - Deploy project as ArenaSource
 *
 * This endpoint:
 * 1. Reads all project files from the workspace filesystem
 * 2. Creates/updates a ConfigMap containing the project files
 * 3. Creates/updates an ArenaSource pointing to the ConfigMap
 *
 * Protected by workspace access checks.
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
import { getWorkspaceCoreApi, withTokenRefresh } from "@/lib/k8s/workspace-k8s-client-factory";
import type { WorkspaceAccess, WorkspaceRole } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource } from "@/types/arena";
import * as fs from "node:fs/promises";
import * as path from "node:path";

const RESOURCE_TYPE = "ArenaProjectDeploy";

// Base path for workspace content
const WORKSPACE_CONTENT_BASE =
  process.env.WORKSPACE_CONTENT_PATH || "/workspace-content";

// Labels for tracking deployed resources
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
  configMap: { name: string; namespace: string };
  isNew: boolean;
}

/**
 * Get the project directory path
 */
function getProjectPath(workspaceName: string, namespace: string, projectId: string): string {
  return path.join(
    WORKSPACE_CONTENT_BASE,
    workspaceName,
    namespace,
    "arena",
    "projects",
    projectId
  );
}

/**
 * Recursively read all files from a directory
 */
async function readAllFiles(
  dirPath: string,
  basePath: string = ""
): Promise<Array<{ path: string; content: string }>> {
  const files: Array<{ path: string; content: string }> = [];

  const entries = await fs.readdir(dirPath, { withFileTypes: true });

  for (const entry of entries) {
    // Skip hidden files and directories
    if (entry.name.startsWith(".")) continue;

    const fullPath = path.join(dirPath, entry.name);
    const relativePath = basePath ? `${basePath}/${entry.name}` : entry.name;

    if (entry.isDirectory()) {
      const subFiles = await readAllFiles(fullPath, relativePath);
      files.push(...subFiles);
    } else {
      const content = await fs.readFile(fullPath, "utf-8");
      files.push({ path: relativePath, content });
    }
  }

  return files;
}

/**
 * Encode file path for ConfigMap key (replace / with __)
 */
function encodeFilePathForConfigMap(filePath: string): string {
  return filePath.replace(/\//g, "__");
}

/**
 * Create or update a ConfigMap with project files
 */
async function createOrUpdateConfigMap(
  options: { workspace: string; namespace: string; role: WorkspaceRole },
  configMapName: string,
  projectId: string,
  files: Array<{ path: string; content: string }>
): Promise<void> {
  return withTokenRefresh(options, async () => {
    const coreApi = await getWorkspaceCoreApi(options);

    // Convert files to ConfigMap data
    const data: Record<string, string> = {};
    for (const file of files) {
      data[encodeFilePathForConfigMap(file.path)] = file.content;
    }

    const configMapBody = {
      apiVersion: "v1",
      kind: "ConfigMap",
      metadata: {
        name: configMapName,
        namespace: options.namespace,
        labels: {
          [MANAGED_BY_LABEL]: MANAGED_BY_VALUE,
          [PROJECT_LABEL]: projectId,
        },
      },
      data,
    };

    try {
      // Try to get existing ConfigMap
      await coreApi.readNamespacedConfigMap({
        namespace: options.namespace,
        name: configMapName,
      });

      // Update existing ConfigMap
      await coreApi.replaceNamespacedConfigMap({
        namespace: options.namespace,
        name: configMapName,
        body: configMapBody,
      });
    } catch (error) {
      // If not found, create new ConfigMap
      if (isNotFoundError(error)) {
        await coreApi.createNamespacedConfigMap({
          namespace: options.namespace,
          body: configMapBody,
        });
      } else {
        throw error;
      }
    }
  });
}

/**
 * Extract status code from various error formats
 */
function extractStatusCode(error: unknown): number | null {
  if (typeof error !== "object" || error === null) {
    return null;
  }

  const err = error as Record<string, unknown>;

  // Direct statusCode property
  if (typeof err.statusCode === "number") {
    return err.statusCode;
  }

  // Response statusCode
  if (err.response && typeof (err.response as Record<string, unknown>).statusCode === "number") {
    return (err.response as Record<string, unknown>).statusCode as number;
  }

  // Kubernetes client error format: "HTTP-Code: 404" in message
  if (typeof err.message === "string" && err.message.includes("HTTP-Code: 404")) {
    return 404;
  }

  // Kubernetes API response body
  if (typeof err.body === "string") {
    try {
      const parsed = JSON.parse(err.body) as Record<string, unknown>;
      if (typeof parsed.code === "number") {
        return parsed.code;
      }
    } catch {
      // Not JSON, ignore
    }
  } else if (err.body && typeof (err.body as Record<string, unknown>).code === "number") {
    return (err.body as Record<string, unknown>).code as number;
  }

  return null;
}

/**
 * Check if error is a 404 Not Found
 */
function isNotFoundError(error: unknown): boolean {
  return extractStatusCode(error) === 404;
}

/**
 * POST /api/workspaces/:name/arena/projects/:id/deploy
 *
 * Deploy a project as an ArenaSource backed by a ConfigMap.
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

      auditCtx = createAuditContext(
        name,
        namespace,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      // Parse request body
      const body = (await request.json().catch(() => ({}))) as DeployRequest;

      // Generate resource names
      const configMapName = `arena-project-${projectId}`;
      const sourceName = body.name || `project-${projectId}`;

      // Get project path and verify it exists
      const projectPath = getProjectPath(name, namespace, projectId);
      try {
        await fs.access(projectPath);
      } catch {
        return notFoundResponse(`Project not found: ${projectId}`);
      }

      // Read all project files
      const files = await readAllFiles(projectPath);
      if (files.length === 0) {
        return NextResponse.json(
          { error: "Bad Request", message: "Project has no files to deploy" },
          { status: 400 }
        );
      }

      // Create or update ConfigMap
      await createOrUpdateConfigMap(
        result.clientOptions,
        configMapName,
        projectId,
        files
      );

      // Check if ArenaSource already exists
      const existingSource = await getCrd<ArenaSource>(
        result.clientOptions,
        CRD_ARENA_SOURCES,
        sourceName
      );

      let source: ArenaSource;
      let isNew = false;

      if (existingSource) {
        // Update existing source
        const updatedSource = {
          ...existingSource,
          spec: {
            ...existingSource.spec,
            type: "configmap" as const,
            configMap: {
              name: configMapName,
            },
            interval: body.syncInterval || existingSource.spec.interval || "5m",
          },
          metadata: {
            ...existingSource.metadata,
            labels: {
              ...existingSource.metadata.labels,
              [PROJECT_LABEL]: projectId,
              [MANAGED_BY_LABEL]: MANAGED_BY_VALUE,
            },
          },
        };

        source = await updateCrd<ArenaSource>(
          result.clientOptions,
          CRD_ARENA_SOURCES,
          sourceName,
          updatedSource
        );
      } else {
        // Create new source
        isNew = true;
        const newSource = buildCrdResource(
          "ArenaSource",
          name,
          namespace,
          sourceName,
          {
            type: "configmap",
            configMap: {
              name: configMapName,
            },
            interval: body.syncInterval || "5m",
          },
          {
            [PROJECT_LABEL]: projectId,
            [MANAGED_BY_LABEL]: MANAGED_BY_VALUE,
          }
        );

        source = await createCrd<ArenaSource>(
          result.clientOptions,
          CRD_ARENA_SOURCES,
          newSource as unknown as ArenaSource
        );
      }

      const response: DeployResponse = {
        source,
        configMap: {
          name: configMapName,
          namespace,
        },
        isNew,
      };

      auditSuccess(auditCtx, isNew ? "create" : "update", sourceName, {
        configMap: configMapName,
        fileCount: files.length,
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
