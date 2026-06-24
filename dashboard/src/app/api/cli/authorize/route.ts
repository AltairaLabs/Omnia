/**
 * CLI browser-login entry point.
 *
 * GET /api/cli/authorize?callback=<loopback>&state=<nonce>
 *
 * Validates the loopback callback + CLI state, stashes a flow record, then
 * routes the browser through the existing OIDC login (if needed) and on to the
 * workspace picker. No token is ever issued here.
 */
import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getUser } from "@/lib/auth";
import { getSessionStore } from "@/lib/auth/session-store";
import { parseLoopbackCallback, isValidCliState } from "@/lib/cli/validate-callback";
import { newFlowId } from "@/lib/cli/ids";

function badRequest(error: string): NextResponse {
  return NextResponse.json({ error }, { status: 400 });
}

async function isAuthenticated(): Promise<boolean> {
  try {
    const user = await getUser();
    return user.provider !== "anonymous";
  } catch {
    return false;
  }
}

export async function GET(request: NextRequest): Promise<NextResponse> {
  const config = getAuthConfig();
  if (config.mode !== "oauth") return badRequest("oauth_required");

  const callback = parseLoopbackCallback(request.nextUrl.searchParams.get("callback"));
  if (!callback) return badRequest("invalid_callback");

  const state = request.nextUrl.searchParams.get("state");
  if (!isValidCliState(state)) return badRequest("invalid_state");

  const flowId = newFlowId();
  await getSessionStore().putCliFlow(
    flowId,
    { callback: callback.toString(), cliState: state, createdAt: Date.now() },
    config.session.pkceTtl
  );

  const selectPath = `/cli/select?flow=${encodeURIComponent(flowId)}`;
  const target = (await isAuthenticated())
    ? selectPath
    : `/api/auth/login?returnTo=${encodeURIComponent(selectPath)}`;
  return NextResponse.redirect(new URL(target, config.baseUrl));
}
