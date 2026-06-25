import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth/session-store", () => ({ getSessionStore: vi.fn() }));
vi.mock("@/lib/auth/api-keys", () => ({ getApiKeyStore: vi.fn(), getApiKeyConfig: vi.fn() }));
vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return { ...actual, validateWorkspace: vi.fn() };
});
vi.mock("@/lib/data/deploy-profile", () => ({
  buildDeployProfile: vi.fn(),
  resolveApiEndpoint: vi.fn().mockReturnValue("https://omnia.example.com"),
}));

const codeRec = {
  userId: "u1", email: "u@e.com", groups: ["g"], userRole: "editor",
  workspace: "team-acme", workspaceRole: "editor", createdAt: 1,
};
const store = { consumeCliCode: vi.fn() };
const apiKeyStore = { create: vi.fn() };
const profile = { api_endpoint: "https://omnia.example.com", workspace: "team-acme", providers: [], skills: [] };

function jsonReq(body: unknown): NextRequest {
  return new NextRequest("https://omnia.example.com/api/cli/token", {
    method: "POST",
    headers: { "content-type": "application/json", "x-forwarded-host": "omnia.example.com" },
    body: JSON.stringify(body),
  });
}

async function arrange(opts: { code?: unknown; ws?: unknown } = {}) {
  const { getSessionStore } = await import("@/lib/auth/session-store");
  const { getApiKeyStore, getApiKeyConfig } = await import("@/lib/auth/api-keys");
  const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  const { buildDeployProfile } = await import("@/lib/data/deploy-profile");
  vi.mocked(getSessionStore).mockReturnValue(store as never);
  vi.mocked(getApiKeyStore).mockReturnValue(apiKeyStore as never);
  vi.mocked(getApiKeyConfig).mockReturnValue({ allowCreate: true } as never);
  store.consumeCliCode.mockResolvedValue(opts.code === undefined ? codeRec : opts.code);
  vi.mocked(validateWorkspace).mockResolvedValue(
    (opts.ws ?? { ok: true, workspace: {}, clientOptions: { workspace: "team-acme", namespace: "ns", role: "editor" } }) as never
  );
  apiKeyStore.create.mockResolvedValue({ key: "omnia_sk_secret" });
  vi.mocked(buildDeployProfile).mockResolvedValue(profile as never);
}

describe("POST /api/cli/token", () => {
  beforeEach(() => { vi.resetModules(); store.consumeCliCode.mockReset(); apiKeyStore.create.mockReset(); });
  afterEach(() => vi.resetAllMocks());

  it("400 on a missing/invalid body", async () => {
    await arrange();
    const { POST } = await import("./route");
    expect((await POST(jsonReq({}))).status).toBe(400);
  });

  it("400 on an invalid/expired/replayed code", async () => {
    await arrange({ code: null });
    const { POST } = await import("./route");
    expect((await POST(jsonReq({ code: "nope" }))).status).toBe(400);
  });

  it("mints a scoped short-lived token and returns { token, profile }", async () => {
    vi.stubEnv("OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS", "900");
    await arrange();
    const { POST } = await import("./route");
    const res = await POST(jsonReq({ code: "c1" }));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.token).toBe("omnia_sk_secret");
    expect(body.profile).toEqual(profile);
    expect(apiKeyStore.create).toHaveBeenCalledWith(
      "u1",
      expect.objectContaining({
        role: "editor",
        workspaces: ["team-acme"],
        expiresInSeconds: 900,
        ownerEmail: "u@e.com",
        ownerGroups: ["g"],
      })
    );
    expect((apiKeyStore.create.mock.calls[0][1] as { name: string }).name).toMatch(/^cli-deploy-team-acme-/);
    vi.unstubAllEnvs();
  });

  it("returns the workspace-not-found response from validateWorkspace", async () => {
    const { NextResponse } = await import("next/server");
    await arrange({ ws: { ok: false, response: NextResponse.json({ error: "Not Found" }, { status: 404 }) } });
    const { POST } = await import("./route");
    expect((await POST(jsonReq({ code: "c1" }))).status).toBe(404);
  });
});
