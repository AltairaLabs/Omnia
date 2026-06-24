/**
 * CLI browser-login grant.
 *
 * POST /api/cli/grant  { flow, workspace }
 *
 * Same-origin + authenticated. Re-checks the user's deploy access to the chosen
 * workspace, mints a one-time exchange code, consumes the flow record, and
 * 303-redirects the browser to the loopback callback with ?code=&state=.
 * The token is NEVER issued here — only the one-time code crosses the redirect.
 */
import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { getSessionStore } from "@/lib/auth/session-store";
import { checkWorkspaceAccess } from "@/lib/auth/workspace-authz";
import { isSameOrigin } from "@/lib/cli/same-origin";
import { newOneTimeCode } from "@/lib/cli/ids";
import { CLI_CODE_TTL_SECONDS } from "@/lib/cli/config";

async function readFields(request: NextRequest): Promise<{ flow: string; workspace: string } | null> {
  const ct = request.headers.get("content-type") || "";
  let flow: unknown;
  let workspace: unknown;
  if (ct.includes("application/json")) {
    const body = await request.json().catch(() => ({}));
    flow = body?.flow;
    workspace = body?.workspace;
  } else {
    const form = await request.formData();
    flow = form.get("flow");
    workspace = form.get("workspace");
  }
  if (typeof flow !== "string" || typeof workspace !== "string" || !flow || !workspace) return null;
  return { flow, workspace };
}

export async function POST(request: NextRequest): Promise<NextResponse> {
  if (!isSameOrigin(request)) {
    return NextResponse.json({ error: "cross_origin" }, { status: 403 });
  }

  const user = await getUser();
  if (user.provider === "anonymous") {
    return NextResponse.json({ error: "unauthenticated" }, { status: 401 });
  }

  const fields = await readFields(request);
  if (!fields) return NextResponse.json({ error: "missing_fields" }, { status: 400 });

  const store = getSessionStore();
  const flow = await store.getCliFlow(fields.flow);
  if (!flow) return NextResponse.json({ error: "invalid_or_expired_flow" }, { status: 400 });

  const access = await checkWorkspaceAccess(fields.workspace, "editor");
  if (!access.granted || !access.role) {
    return NextResponse.json({ error: "forbidden_workspace" }, { status: 403 });
  }

  const code = newOneTimeCode();
  await store.putCliCode(
    code,
    {
      userId: user.id,
      email: user.email || user.username,
      groups: user.groups,
      userRole: user.role,
      workspace: fields.workspace,
      workspaceRole: access.role,
      createdAt: Date.now(),
    },
    CLI_CODE_TTL_SECONDS
  );
  await store.consumeCliFlow(fields.flow);

  const target = new URL(flow.callback);
  target.searchParams.set("code", code);
  target.searchParams.set("state", flow.cliState);
  // 303 so the browser follows with GET (POST -> loopback GET).
  return NextResponse.redirect(target, 303);
}
