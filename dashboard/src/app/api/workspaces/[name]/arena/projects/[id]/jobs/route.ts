/**
 * API routes for listing Arena jobs for a specific project.
 *
 * GET /api/workspaces/:name/arena/projects/:id/jobs - List jobs for project
 *
 * Returns all ArenaJobs that reference this project's ArenaSource.
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
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaJob, ArenaJobType, ArenaJobPhase } from "@/types/arena";

const RESOURCE_TYPE = "ArenaProjectJobs";
const PROJECT_LABEL = "arena.omnia.altairalabs.ai/project-id";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

interface ProjectJobsResponse {
  jobs: ArenaJob[];
  source?: ArenaSource;
  deployed: boolean;
}

/**
 * GET /api/workspaces/:name/arena/projects/:id/jobs
 *
 * List all jobs for a project.
 */
export const GET = withWorkspaceAccess<{ name: string; id: string }>(
  "viewer",
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

      // Find ArenaSource for this project
      const sources = await listCrd<ArenaSource>(result.clientOptions, CRD_ARENA_SOURCES);
      const projectSource = sources.find(
        (s) => s.metadata.labels?.[PROJECT_LABEL] === projectId
      );

      if (!projectSource) {
        // Project not deployed yet, no jobs possible
        const response: ProjectJobsResponse = {
          jobs: [],
          deployed: false,
        };
        auditSuccess(auditCtx, "list", projectId, { count: 0, deployed: false });
        return NextResponse.json(response);
      }

      // Get all jobs
      let jobs = await listCrd<ArenaJob>(result.clientOptions, CRD_ARENA_JOBS);

      // Filter jobs that reference this project's source
      // Either by sourceRef.name matching, or by project label
      jobs = jobs.filter(
        (job) =>
          job.spec.sourceRef.name === projectSource.metadata.name ||
          job.metadata.labels?.[PROJECT_LABEL] === projectId
      );

      // Apply query parameter filters
      const { searchParams } = new URL(request.url);
      const typeFilter = searchParams.get("type") as ArenaJobType | null;
      const statusFilter = searchParams.get("status") as ArenaJobPhase | null;
      const limit = searchParams.get("limit");

      if (typeFilter) {
        jobs = jobs.filter((job) => job.spec.type === typeFilter);
      }
      if (statusFilter) {
        jobs = jobs.filter((job) => job.status?.phase === statusFilter);
      }

      // Sort by creation time, newest first
      jobs.sort((a, b) => {
        const aTime = a.metadata.creationTimestamp || "";
        const bTime = b.metadata.creationTimestamp || "";
        return bTime.localeCompare(aTime);
      });

      // Apply limit
      if (limit) {
        const limitNum = parseInt(limit, 10);
        if (!isNaN(limitNum) && limitNum > 0) {
          jobs = jobs.slice(0, limitNum);
        }
      }

      const response: ProjectJobsResponse = {
        jobs,
        source: projectSource,
        deployed: true,
      };

      auditSuccess(auditCtx, "list", projectId, {
        count: jobs.length,
        deployed: true,
        sourceName: projectSource.metadata.name,
      });

      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", projectId, error, 500);
      }
      return handleK8sError(error, "list project jobs");
    }
  }
);
