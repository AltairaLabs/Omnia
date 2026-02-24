/**
 * API routes for running Arena jobs from projects.
 *
 * POST /api/workspaces/:name/arena/projects/:id/run - Quick run a project
 *
 * This endpoint:
 * 1. Ensures the project is deployed (auto-deploys if not)
 * 2. Creates an ArenaJob referencing the deployed ArenaSource
 * 3. Returns the created job
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { createCrd, listCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  notFoundResponse,
  buildCrdResource,
  CRD_ARENA_SOURCES,
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource, ArenaJob, ArenaJobType, ScenarioFilter, ExecutionConfig } from "@/types/arena";

const RESOURCE_TYPE = "ArenaProjectRun";
const PROJECT_LABEL = "arena.omnia.altairalabs.ai/project-id";
const ERROR_BAD_REQUEST = "Bad Request";

interface RouteParams {
  params: Promise<{ name: string; id: string }>;
}

interface QuickRunRequest {
  type: ArenaJobType;
  name?: string;
  scenarios?: ScenarioFilter;
  verbose?: boolean;
  execution?: ExecutionConfig;
}

interface QuickRunResponse {
  job: ArenaJob;
  source: ArenaSource;
}

/**
 * Generate a unique job name.
 * Uses Math.random for uniqueness suffix - not for cryptographic purposes,
 * just to avoid name collisions when creating multiple jobs quickly.
 */
function generateJobName(projectId: string, type: ArenaJobType): string {
  const timestamp = Date.now().toString(36);
  // eslint-disable-next-line sonarjs/pseudo-random -- Non-cryptographic use for job name uniqueness
  const random = Math.random().toString(36).substring(2, 6);
  return `${projectId}-${type}-${timestamp}-${random}`;
}

/**
 * POST /api/workspaces/:name/arena/projects/:id/run
 *
 * Quick run a project as an ArenaJob.
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
      const body = (await request.json()) as QuickRunRequest;

      if (!body.type) {
        return NextResponse.json(
          { error: ERROR_BAD_REQUEST, message: "Job type is required" },
          { status: 400 }
        );
      }

      // Validate job type
      const validTypes: ArenaJobType[] = ["evaluation", "loadtest", "datagen"];
      if (!validTypes.includes(body.type)) {
        return NextResponse.json(
          {
            error: ERROR_BAD_REQUEST,
            message: `Invalid job type. Must be one of: ${validTypes.join(", ")}`,
          },
          { status: 400 }
        );
      }

      // Find ArenaSource for this project
      const sources = await listCrd<ArenaSource>(result.clientOptions, CRD_ARENA_SOURCES);
      const projectSource = sources.find(
        (s) => s.metadata.labels?.[PROJECT_LABEL] === projectId
      );

      if (!projectSource) {
        return notFoundResponse(
          `Project is not deployed. Deploy the project first using the Deploy button.`
        );
      }

      // Check source is ready
      if (projectSource.status?.phase !== "Ready") {
        return NextResponse.json(
          {
            error: ERROR_BAD_REQUEST,
            message: `Source is not ready. Current phase: ${projectSource.status?.phase || "Unknown"}`,
          },
          { status: 400 }
        );
      }

      // Generate job name
      const jobName = body.name || generateJobName(projectId, body.type);

      // Build job spec based on type
      const jobSpec: Record<string, unknown> = {
        sourceRef: {
          name: projectSource.metadata.name,
        },
        type: body.type,
        verbose: body.verbose ?? false,
      };

      // Add scenarios filter if provided
      if (body.scenarios) {
        jobSpec.scenarios = body.scenarios;
      }

      // Add execution config if fleet mode
      if (body.execution?.mode === "fleet") {
        jobSpec.execution = body.execution;
      }

      // Add type-specific defaults
      if (body.type === "evaluation") {
        jobSpec.evaluation = {
          outputFormats: ["json"],
          continueOnFailure: true,
        };
      } else if (body.type === "loadtest") {
        jobSpec.loadtest = {
          profileType: "constant",
          duration: "1m",
        };
      } else if (body.type === "datagen") {
        jobSpec.datagen = {
          sampleCount: 10,
          mode: "selfplay",
          outputFormat: "jsonl",
        };
      }

      // Create the job
      const job = buildCrdResource(
        "ArenaJob",
        name,
        namespace,
        jobName,
        jobSpec,
        {
          [PROJECT_LABEL]: projectId,
        }
      );

      const createdJob = await createCrd<ArenaJob>(
        result.clientOptions,
        CRD_ARENA_JOBS,
        job as unknown as ArenaJob
      );

      const response: QuickRunResponse = {
        job: createdJob,
        source: projectSource,
      };

      auditSuccess(auditCtx, "create", jobName, {
        type: body.type,
        sourceName: projectSource.metadata.name,
      });

      return NextResponse.json(response, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", projectId, error, 500);
      }
      return handleK8sError(error, "run project");
    }
  }
);
