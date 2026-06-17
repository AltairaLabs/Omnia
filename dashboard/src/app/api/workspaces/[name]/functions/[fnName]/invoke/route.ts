/**
 * POST /api/workspaces/:name/functions/:fnName/invoke
 *
 * Invokes a function-mode AgentRuntime, mirroring how the Console reaches an
 * agent's facade: address the operator-created Service `<fnName>.<namespace>`
 * on spec.facade.port and POST the input to the facade's `/functions/<fnName>`
 * endpoint. Auth mirrors the Console WS proxy — a minted mgmt-plane JWT when a
 * signing key is configured, otherwise unauthenticated (dev fallback).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getWorkspaceResource, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import { mgmtPlaneAuthHeaders } from "@/lib/functions/invoke-token";
import { isFunctionMode } from "@/types/agent-runtime";
import type { AgentRuntime } from "@/types";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type Params = { name: string; fnName: string };

const DEFAULT_FACADE_PORT = 8080;
const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";

interface RouteContext {
  params: Promise<Params>;
}

export const POST = withWorkspaceAccess<Params>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User,
  ): Promise<NextResponse> => {
    const { name, fnName } = await context.params;

    // Resolve the function's AgentRuntime to get the backing namespace, the
    // facade port, and to confirm it is actually a function.
    const res = await getWorkspaceResource<AgentRuntime>(
      name,
      access.role!,
      CRD_AGENTS,
      fnName,
      "Function",
    );
    if (!res.ok) return res.response;

    if (!isFunctionMode(res.resource.spec)) {
      return NextResponse.json(
        { error: "not_a_function", detail: `${fnName} is not a function-mode AgentRuntime` },
        { status: 400 },
      );
    }

    const namespace = res.workspace.spec.namespace.name;
    const port = res.resource.spec.facade?.port ?? DEFAULT_FACADE_PORT;
    const target = `http://${fnName}.${namespace}.${SERVICE_DOMAIN}:${port}/functions/${encodeURIComponent(fnName)}`;

    const body = await request.text();
    const subject = user.email || user.id || "dashboard";

    let response: Response;
    try {
      response = await fetch(target, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...mgmtPlaneAuthHeaders(fnName, namespace, subject) },
        body,
      });
    } catch (err) {
      return NextResponse.json(
        {
          error: "facade_unreachable",
          detail: `Could not reach the function facade (${target}): ${err instanceof Error ? err.message : "unknown error"}`,
        },
        { status: 502 },
      );
    }

    // Pass the facade's response straight through — it already validates input
    // against inputSchema and returns typed errors + usage.
    const text = await response.text();
    return new NextResponse(text, {
      status: response.status,
      headers: { "Content-Type": response.headers.get("Content-Type") || "application/json" },
    });
  },
);
