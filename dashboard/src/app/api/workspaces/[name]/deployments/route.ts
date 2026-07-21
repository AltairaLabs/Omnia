/**
 * DeployIntent forwarding endpoint.
 *
 * POST /api/workspaces/:name/deployments
 *
 * Editor-gated proxy that forwards an opaque, versioned DeployIntent body
 * from the promptarena-deploy-omnia adapter to the operator's deploy-intent
 * API (via deploy-api-service). The dashboard does not validate or interpret
 * the intent — the operator's `internal/api/deploy` package owns the schema.
 * Part of the deploy-intent decoupling epic (#1863, Plan C, #1866).
 */

import { NextResponse, type NextRequest } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { postDeployment, DeployApiError } from "@/lib/data/deploy-api-service";

export const POST = withWorkspaceAccess<{ name: string }>(
  "editor",
  async (request: NextRequest, context, _access, user) => {
    const { name: workspace } = await context.params;
    let intent: unknown;
    try {
      intent = await request.json();
    } catch {
      return NextResponse.json({ error: "invalid JSON body" }, { status: 400 });
    }
    try {
      const { status, result } = await postDeployment(workspace, user, intent);
      return NextResponse.json(result, { status });
    } catch (err) {
      if (err instanceof DeployApiError) {
        return NextResponse.json({ error: err.message }, { status: err.status });
      }
      throw err;
    }
  },
);
