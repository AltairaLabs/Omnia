/**
 * SSE endpoint for live arena job statistics.
 *
 * GET /api/workspaces/:name/arena/jobs/:jobName/live-stats
 *
 * Streams JSON snapshots of job stats from Redis every 2 seconds.
 * The browser connects via EventSource and receives `data:` frames.
 *
 * Closes automatically when:
 * - The client disconnects (abort signal)
 * - Redis is not configured (returns 501)
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_JOBS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaJob } from "@/types/arena";
import { getArenaRedisClient } from "@/lib/redis/client";
import { readArenaStats } from "@/lib/redis/arena-stats";

type RouteParams = { name: string; jobName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const SSE_INTERVAL_MS = 2000;
const CRD_KIND = "ArenaJob";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, jobName } = await context.params;
    let auditCtx;

    try {
      // Validate access to the job
      const result = await getWorkspaceResource<ArenaJob>(
        name,
        access.role!,
        CRD_ARENA_JOBS,
        jobName,
        "Arena job"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const redis = getArenaRedisClient();
      if (!redis) {
        auditError(auditCtx, "get", jobName, new Error("Redis not configured"), 501);
        return NextResponse.json(
          { error: "Redis is not configured for live stats" },
          { status: 501 }
        );
      }

      auditSuccess(auditCtx, "get", jobName, { subresource: "live-stats" });

      const encoder = new TextEncoder();
      const stream = new ReadableStream({
        async start(controller) {
          let closed = false;
          let consecutiveErrors = 0;

          const close = () => {
            if (closed) return;
            closed = true;
            clearInterval(interval);
            try {
              controller.close();
            } catch {
              // stream may already be closed
            }
          };

          // Send initial data frame immediately
          try {
            const initialStats = await readArenaStats(redis, jobName);
            if (initialStats) {
              controller.enqueue(encoder.encode(`data: ${JSON.stringify(initialStats)}\n\n`));
            }
          } catch {
            // Non-fatal: the interval will retry
          }

          const interval = setInterval(async () => {
            if (closed) return;
            try {
              const stats = await readArenaStats(redis, jobName);
              if (stats) {
                controller.enqueue(encoder.encode(`data: ${JSON.stringify(stats)}\n\n`));
                consecutiveErrors = 0;
              }
            } catch {
              consecutiveErrors++;
              if (consecutiveErrors >= 5) {
                close();
              }
            }
          }, SSE_INTERVAL_MS);

          request.signal.addEventListener("abort", close);
        },
      });

      // NextResponse extends Response, so we can construct it from a ReadableStream.
      // The SSE headers tell the browser to treat this as an event stream.
      return new NextResponse(stream, {
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
        },
      });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", jobName, error, 500);
      }
      return handleK8sError(error, "stream live stats for this arena job");
    }
  }
);
