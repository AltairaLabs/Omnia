/**
 * SSE endpoint for live AgentRuntime rollout status.
 *
 * GET /api/workspaces/:name/agents/:agentName/rollout/stream
 *
 * Streams the agent's rollout config + status every 2s as `data:` JSON frames.
 * The browser connects via EventSource (use-event-source). Because the stream
 * stays open for the life of the page, the rollout panel sees a rollout *start*
 * without a manual refresh (unlike a client refetchInterval that only polls
 * once a rollout is already active).
 *
 * Closes on client disconnect. Protected by workspace access checks.
 */
import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  CRD_AGENTS,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { RolloutConfig, RolloutStatus } from "@/types/agent-runtime";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const SSE_INTERVAL_MS = 2000;
const MAX_CONSECUTIVE_ERRORS = 5;
/** Keep emitting a finished rollout's terminal (Promoted/Rolled-back) frame for
 * this long after the operator clears status.rollout, so the outcome stays
 * visible in the panel instead of flashing for one reconcile. */
const TERMINAL_LINGER_MS = 8000;

/** Minimal shape we read off the AgentRuntime CRD for the rollout panel. */
interface AgentRolloutShape {
  spec?: { rollout?: RolloutConfig };
  status?: { rollout?: RolloutStatus };
}

/** The frame the panel consumes: the rollout spec (steps) + live status. */
export interface RolloutStreamFrame {
  spec: RolloutConfig | null;
  status: RolloutStatus | null;
}

function snapshot(agent: AgentRolloutShape | undefined): RolloutStreamFrame {
  return {
    spec: agent?.spec?.rollout ?? null,
    status: agent?.status?.rollout ?? null,
  };
}

/** A finished rollout that still carries a terminal message. */
function isTerminal(frame: RolloutStreamFrame): boolean {
  return !!frame.status && frame.status.active === false && !!frame.status.message;
}

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
  ): Promise<NextResponse> => {
    const { name, agentName } = await context.params;

    // Validate access + that the agent exists before opening the stream.
    const initial = await getWorkspaceResource<AgentRolloutShape>(
      name,
      access.role!,
      CRD_AGENTS,
      agentName,
      "Agent",
    );
    if (!initial.ok) return initial.response;

    const encoder = new TextEncoder();
    const send = (controller: ReadableStreamDefaultController, frame: RolloutStreamFrame) =>
      controller.enqueue(encoder.encode(`data: ${JSON.stringify(frame)}\n\n`));

    const stream = new ReadableStream({
      async start(controller) {
        let closed = false;
        let consecutiveErrors = 0;
        // Server-side terminal linger: keep emitting the last Promoted/Rolled-
        // back frame until the window elapses, so the panel doesn't need any
        // client-side timer to keep the outcome on screen.
        let held: { frame: RolloutStreamFrame; until: number } | null = null;
        const withLinger = (raw: RolloutStreamFrame): RolloutStreamFrame => {
          if (raw.status?.active) {
            held = null;
            return raw;
          }
          if (isTerminal(raw)) {
            held = { frame: raw, until: Date.now() + TERMINAL_LINGER_MS };
            return raw;
          }
          if (held && Date.now() < held.until) {
            return held.frame;
          }
          held = null;
          return raw;
        };
        const close = () => {
          if (closed) return;
          closed = true;
          clearInterval(interval);
          try {
            controller.close();
          } catch {
            // already closed
          }
        };

        // Immediate first frame from the resource we already fetched.
        send(controller, withLinger(snapshot(initial.resource)));

        const interval = setInterval(async () => {
          if (closed) return;
          try {
            const next = await getWorkspaceResource<AgentRolloutShape>(
              name,
              access.role!,
              CRD_AGENTS,
              agentName,
              "Agent",
            );
            if (next.ok) {
              send(controller, withLinger(snapshot(next.resource)));
              consecutiveErrors = 0;
            }
          } catch {
            consecutiveErrors++;
            if (consecutiveErrors >= MAX_CONSECUTIVE_ERRORS) close();
          }
        }, SSE_INTERVAL_MS);

        request.signal.addEventListener("abort", close);
      },
    });

    return new NextResponse(stream, {
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
      },
    });
  },
);
