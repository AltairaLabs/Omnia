import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import type { User } from "@/lib/auth/types";
import { DeployApiError, postDeployment } from "./deploy-api-service";

const BASE_URL = "http://operator.test:8084";

const user = {
  id: "u",
  username: "user",
  email: "user@example.com",
  groups: ["eng", "admins"],
  role: "viewer",
  provider: "oauth",
} as unknown as User;

const intent = { apiVersion: "deploy.omnia.altairalabs.ai/v1", pack: { name: "p" }, agents: [] };

let fetchMock: ReturnType<typeof vi.fn>;

beforeAll(() => {
  const { privateKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  const pemPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "omnia-deploy-")), "key.pem");
  fs.writeFileSync(pemPath, privateKey, { mode: 0o600 });
  process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = pemPath;
});

afterAll(() => {
  delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
});

beforeEach(() => {
  process.env.OPERATOR_DEPLOY_API_URL = BASE_URL;
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function okJson(body: unknown, status = 200) {
  return { ok: true, status, json: async () => body };
}

function decodeAuthClaims(): Record<string, unknown> {
  const init = fetchMock.mock.calls[0][1] as { headers: Record<string, string> };
  const token = init.headers.Authorization.replace(/^Bearer /, "");
  return JSON.parse(Buffer.from(token.split(".")[1], "base64url").toString("utf8"));
}

describe("postDeployment", () => {
  it("POSTs the intent to the workspace deployments URL with a bearer identity token", async () => {
    fetchMock.mockResolvedValue(okJson({ succeeded: true, results: [] }));
    await postDeployment("demo", user, intent);

    const [url, init] = fetchMock.mock.calls[0] as [string, { method: string; body: string; headers: Record<string, string> }];
    expect(url).toBe(`${BASE_URL}/api/v1/workspaces/demo/deployments`);
    expect(init.method).toBe("POST");
    expect(init.headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(init.body)).toEqual(intent);

    const claims = decodeAuthClaims();
    expect(claims.aud).toBe("omnia-operator");
    expect(claims.iss).toBe("omnia-dashboard");
    expect(claims.workspace).toBe("demo");
    expect(claims.identity).toBe(user.email);
  });

  it("returns { status: 200, result } on a successful deploy", async () => {
    const result = { succeeded: true, results: [{ kind: "AgentRuntime", name: "a", action: "created" }] };
    fetchMock.mockResolvedValue(okJson(result));
    const res = await postDeployment("demo", user, intent);
    expect(res).toEqual({ status: 200, result });
  });

  it("returns { status: 207, result } without throwing on a partial failure", async () => {
    const result = {
      succeeded: false,
      results: [
        { kind: "AgentRuntime", name: "a", action: "created" },
        { kind: "AgentRuntime", name: "b", action: "failed", error: "boom" },
      ],
    };
    fetchMock.mockResolvedValue(okJson(result, 207));
    const res = await postDeployment("demo", user, intent);
    expect(res.status).toBe(207);
    expect(res.result.succeeded).toBe(false);
  });

  it("throws DeployApiError with the operator status on a 403", async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 403, json: async () => ({}) });
    await expect(postDeployment("demo", user, intent)).rejects.toMatchObject({
      name: "DeployApiError",
      status: 403,
    });
  });

  it("throws DeployApiError 500 when OPERATOR_DEPLOY_API_URL is unset", async () => {
    delete process.env.OPERATOR_DEPLOY_API_URL;
    await expect(postDeployment("demo", user, intent)).rejects.toMatchObject({
      name: "DeployApiError",
      status: 500,
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("throws DeployApiError 500 when no signing key is configured", async () => {
    const saved = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    try {
      await expect(postDeployment("demo", user, intent)).rejects.toMatchObject({
        name: "DeployApiError",
        status: 500,
      });
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = saved;
    }
  });

  it("is a DeployApiError instance carrying a status", () => {
    const err = new DeployApiError("boom", 502);
    expect(err).toBeInstanceOf(Error);
    expect(err.status).toBe(502);
  });
});
