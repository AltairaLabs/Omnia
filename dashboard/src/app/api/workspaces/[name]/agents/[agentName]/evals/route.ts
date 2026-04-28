/**
 * API route for updating agent eval configuration.
 *
 * PUT /api/workspaces/:name/agents/:agentName/evals - Update eval config
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { patchCrd } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

interface EvalConfigBody {
  enabled?: boolean;
  sampling?: {
    defaultRate?: number;
    extendedRate?: number;
  };
  // inline.groups / worker.groups route which evals run on which path
  // (issue #988). The CRD's EvalPathConfig treats an absent or empty
  // list as "use the built-in default for that path", so callers
  // wanting to drop back to defaults can omit the field or send an
  // empty array.
  inline?: { groups?: string[] };
  worker?: { groups?: string[] };
}

// Sanity bound on the number of groups in one path. Realistic configs
// have 1–4; refusing >32 keeps a malformed client from writing
// pathological lists into the CRD.
const MAX_GROUPS_PER_PATH = 32;

// Group names follow the same shape as Kubernetes label values:
// alphanumeric with `_`, `-`, `.`, 1–63 chars. Tightening the regex
// here means we never write an unrenderable group name to the CRD,
// which would force a `kubectl edit` to recover.
const GROUP_NAME_RE = /^[A-Za-z0-9]([A-Za-z0-9_.-]{0,61}[A-Za-z0-9])?$/;

// ERR_BAD_REQUEST is the shared response shape; SonarCloud flags
// duplicated string literals at 3+ occurrences.
const ERR_BAD_REQUEST = "Bad Request";

function isValidRate(value: unknown): boolean {
  return typeof value === "number" && value >= 0 && value <= 100;
}

function validateGroups(groups: unknown, fieldName: string): string | null {
  if (groups === undefined) return null;
  if (!Array.isArray(groups)) {
    return `${fieldName} must be an array of strings`;
  }
  if (groups.length > MAX_GROUPS_PER_PATH) {
    return `${fieldName} has ${groups.length} entries; maximum is ${MAX_GROUPS_PER_PATH}`;
  }
  const seen = new Set<string>();
  for (const g of groups) {
    if (typeof g !== "string") {
      return `${fieldName} entries must be strings`;
    }
    if (!GROUP_NAME_RE.test(g)) {
      return `${fieldName} entry ${JSON.stringify(g)} is not a valid group name (alphanumeric, _, -, . — up to 63 chars)`;
    }
    if (seen.has(g)) {
      return `${fieldName} contains duplicate entry ${JSON.stringify(g)}`;
    }
    seen.add(g);
  }
  return null;
}

/**
 * validateBody returns the first validation error message, or null
 * when the body shape is acceptable. Extracted so the request handler
 * stays under SonarCloud's cognitive-complexity ceiling.
 */
function validateBody(body: EvalConfigBody): string | null {
  if (body.sampling) {
    if (body.sampling.defaultRate !== undefined && !isValidRate(body.sampling.defaultRate)) {
      return "sampling.defaultRate must be a number between 0 and 100";
    }
    if (body.sampling.extendedRate !== undefined && !isValidRate(body.sampling.extendedRate)) {
      return "sampling.extendedRate must be a number between 0 and 100";
    }
  }
  if (body.inline) {
    const err = validateGroups(body.inline.groups, "inline.groups");
    if (err) return err;
  }
  if (body.worker) {
    const err = validateGroups(body.worker.groups, "worker.groups");
    if (err) return err;
  }
  return null;
}

/**
 * buildPatch composes the JSON-Merge-Patch body. Each top-level key
 * is included only when the caller supplied it so a sampling-only
 * update doesn't clobber existing groups (and vice versa).
 */
function buildPatch(body: EvalConfigBody): Record<string, unknown> {
  return {
    spec: {
      evals: {
        ...(body.enabled !== undefined && { enabled: body.enabled }),
        ...(body.sampling && { sampling: body.sampling }),
        ...(body.inline && { inline: body.inline }),
        ...(body.worker && { worker: body.worker }),
      },
    },
  };
}

export const PUT = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, agentName } = await context.params;
      const body: EvalConfigBody = await request.json();

      const validationError = validateBody(body);
      if (validationError) {
        return NextResponse.json(
          { error: ERR_BAD_REQUEST, message: validationError },
          { status: 400 },
        );
      }

      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      const patched = await patchCrd<AgentRuntime>(
        result.clientOptions,
        CRD_AGENTS,
        agentName,
        buildPatch(body),
      );
      return NextResponse.json(patched);
    } catch (error) {
      return handleK8sError(error, "update eval configuration");
    }
  }
);
