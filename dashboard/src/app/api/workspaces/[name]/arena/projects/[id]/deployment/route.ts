/**
 * API routes for getting Arena project deployment status.
 *
 * GET /api/workspaces/:name/arena/projects/:id/deployment - Get deployment status
 *
 * Returns the ArenaSource and ConfigMap status for a deployed project.
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { listCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import { getWorkspaceCoreApi, withTokenRefresh } from "@/lib/k8s/workspace-k8s-client-factory";
import type { WorkspaceAccess, WorkspaceRole } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource } from "@/types/arena";

const RESOURCE_TYPE = "ArenaProjectDeployment";
const PROJECT_LABEL = "arena.omnia.altairalabs.ai/project-id";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

interface ConfigMapInfo {
  name: string;
  namespace: string;
  fileCount: number;
  createdAt?: string;
  updatedAt?: string;
}

interface DeploymentStatus {
  deployed: boolean;
  source?: ArenaSource;
  configMap?: ConfigMapInfo;
  lastDeployedAt?: string;
}

/**
 * Get ConfigMap info for a project
 */
async function getConfigMapInfo(
  options: { workspace: string; namespace: string; role: WorkspaceRole },
  configMapName: string
): Promise<ConfigMapInfo | null> {
  return withTokenRefresh(options, async () => {
    const coreApi = await getWorkspaceCoreApi(options);

    try {
      const result = await coreApi.readNamespacedConfigMap({
        namespace: options.namespace,
        name: configMapName,
      });

      const data = result.data || {};
      const fileCount = Object.keys(data).length;

      return {
        name: configMapName,
        namespace: options.namespace,
        fileCount,
        createdAt: result.metadata?.creationTimestamp?.toISOString(),
        updatedAt: result.metadata?.resourceVersion
          ? new Date().toISOString() // K8s doesn't track update time, use now
          : undefined,
      };
    } catch (error) {
      if (isNotFoundError(error)) {
        return null;
      }
      throw error;
    }
  });
}

/**
 * Check if error is a 404 Not Found
 */
function isNotFoundError(error: unknown): boolean {
  if (typeof error === "object" && error !== null) {
    if ("statusCode" in error && (error as { statusCode?: number }).statusCode === 404) {
      return true;
    }
    if ("response" in error) {
      const response = (error as { response?: { statusCode?: number } }).response;
      if (response?.statusCode === 404) {
        return true;
      }
    }
  }
  return false;
}

/**
 * GET /api/workspaces/:name/arena/projects/:id/deployment
 *
 * Get the deployment status of a project.
 */
export const GET = withWorkspaceAccess<{ name: string; id: string }>(
  "viewer",
  async (
    _request: NextRequest,
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

      // Find ArenaSource for this project
      const sources = await listCrd<ArenaSource>(result.clientOptions, CRD_ARENA_SOURCES);
      const projectSource = sources.find(
        (s) => s.metadata.labels?.[PROJECT_LABEL] === projectId
      );

      if (!projectSource) {
        const response: DeploymentStatus = {
          deployed: false,
        };
        auditSuccess(auditCtx, "get", projectId, { deployed: false });
        return NextResponse.json(response);
      }

      // Get ConfigMap info
      const configMapName = projectSource.spec.configMap?.name;
      let configMapInfo: ConfigMapInfo | null = null;

      if (configMapName) {
        configMapInfo = await getConfigMapInfo(result.clientOptions, configMapName);
      }

      const response: DeploymentStatus = {
        deployed: true,
        source: projectSource,
        configMap: configMapInfo || undefined,
        lastDeployedAt:
          projectSource.status?.artifact?.lastUpdateTime ||
          projectSource.metadata.creationTimestamp,
      };

      auditSuccess(auditCtx, "get", projectId, {
        deployed: true,
        sourceName: projectSource.metadata.name,
        phase: projectSource.status?.phase,
      });

      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", projectId, error, 500);
      }
      return handleK8sError(error, "get deployment status");
    }
  }
);
