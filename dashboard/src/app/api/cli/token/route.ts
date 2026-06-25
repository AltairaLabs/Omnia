/**
 * CLI browser-login back-channel exchange.
 *
 * POST /api/cli/token  { code }
 *
 * No browser session. Atomically consumes the one-time code, mints a scoped,
 * short-lived omnia_sk_ token for the captured workspace, assembles the deploy
 * profile, and returns { token, profile }. This is the ONLY place a token is
 * issued in the CLI login flow.
 */
import { NextRequest, NextResponse } from "next/server";
import { getSessionStore } from "@/lib/auth/session-store";
import { getApiKeyStore, getApiKeyConfig } from "@/lib/auth/api-keys";
import { validateWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { buildDeployProfile, resolveApiEndpoint } from "@/lib/data/deploy-profile";
import { cliTokenTtlSeconds } from "@/lib/cli/config";
import { shortSuffix } from "@/lib/cli/ids";

function invalidCode(): NextResponse {
  return NextResponse.json({ error: "invalid_or_expired_code" }, { status: 400 });
}

export async function POST(request: NextRequest): Promise<NextResponse> {
  if (!getApiKeyConfig().allowCreate) {
    return NextResponse.json({ error: "api_key_creation_disabled" }, { status: 503 });
  }

  const body = await request.json().catch(() => null);
  const code = body?.code;
  if (typeof code !== "string" || !code) return invalidCode();

  const record = await getSessionStore().consumeCliCode(code);
  if (!record) return invalidCode();

  const result = await validateWorkspace(record.workspace, record.workspaceRole);
  if (!result.ok) return result.response;

  const newKey = await getApiKeyStore().create(record.userId, {
    name: `cli-deploy-${record.workspace}-${shortSuffix()}`,
    role: record.userRole,
    workspaces: [record.workspace],
    expiresInSeconds: cliTokenTtlSeconds(),
    ownerEmail: record.email,
    ownerGroups: record.groups,
  });

  const profile = await buildDeployProfile(
    result.clientOptions,
    record.workspace,
    resolveApiEndpoint(request)
  );

  return NextResponse.json({ token: newKey.key, profile });
}
