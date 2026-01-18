/**
 * API routes for workspace prompt packs (PromptPack CRDs).
 *
 * GET /api/workspaces/:name/promptpacks - List prompt packs in workspace
 * POST /api/workspaces/:name/promptpacks - Create a new prompt pack
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_PROMPTPACKS,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { PromptPack } from "@/lib/data/types";

const CRD_KIND = "PromptPack";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const promptPacks = await listCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS);
      return NextResponse.json(promptPacks);
    } catch (error) {
      return serverErrorResponse(error, "Failed to list prompt packs");
    }
  }
);

export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const body = await request.json();
      const promptPack = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        body.metadata?.name || body.name,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS, promptPack as PromptPack);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      return serverErrorResponse(error, "Failed to create prompt pack");
    }
  }
);
