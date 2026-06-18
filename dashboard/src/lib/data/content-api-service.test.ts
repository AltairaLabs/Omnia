import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import type { User } from "@/lib/auth/types";
import {
  ContentApiError,
  deleteContent,
  getContent,
  isContentFile,
  isContentListing,
  makeContentDir,
  writeContentFile,
} from "./content-api-service";

const BASE_URL = "http://operator.test:8084";

const user = {
  id: "u",
  username: "user",
  email: "user@example.com",
  groups: ["eng", "admins"],
  role: "viewer",
  provider: "oauth",
} as unknown as User;

const anonUser = {
  id: "anonymous",
  username: "anonymous",
  groups: [],
  role: "viewer",
  provider: "anonymous",
} as unknown as User;

let fetchMock: ReturnType<typeof vi.fn>;

beforeAll(() => {
  const { privateKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  const pemPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "omnia-content-")), "key.pem");
  fs.writeFileSync(pemPath, privateKey, { mode: 0o600 });
  process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = pemPath;
});

afterAll(() => {
  delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
});

beforeEach(() => {
  process.env.OPERATOR_CONTENT_API_URL = BASE_URL;
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

describe("getContent", () => {
  it("requests the confined URL with a content-API bearer token", async () => {
    fetchMock.mockResolvedValue(okJson({ path: "arena", entries: [] }));
    const node = await getContent("team-a", user, "arena/projects");

    expect(isContentListing(node)).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`${BASE_URL}/api/v1/workspaces/team-a/content/arena/projects`);
    expect((init as { method: string }).method).toBe("GET");

    const claims = decodeAuthClaims();
    expect(claims.aud).toBe("omnia-operator");
    expect(claims.workspace).toBe("team-a");
    expect(claims.identity).toBe("user@example.com");
    expect(claims.groups).toEqual(["eng", "admins"]);
  });

  it("requests the workspace root when relpath is empty", async () => {
    fetchMock.mockResolvedValue(okJson({ path: "", entries: [] }));
    await getContent("team-a", user, "");
    expect(fetchMock.mock.calls[0][0]).toBe(`${BASE_URL}/api/v1/workspaces/team-a/content`);
  });

  it("returns file content when the operator resolves a file", async () => {
    fetchMock.mockResolvedValue(
      okJson({ path: "f.yaml", content: "hello", encoding: "utf-8", size: 5, modifiedAt: "t" }),
    );
    const node = await getContent("team-a", user, "f.yaml");
    expect(isContentFile(node)).toBe(true);
    if (isContentFile(node)) {
      expect(node.content).toBe("hello");
    }
  });

  it("mints an anonymous token (no identity/groups) for anonymous users", async () => {
    fetchMock.mockResolvedValue(okJson({ path: "", entries: [] }));
    await getContent("team-a", anonUser, "");
    const claims = decodeAuthClaims();
    expect(claims.anonymous).toBe(true);
    expect(claims.identity).toBeUndefined();
    expect(claims.sub).toBe("anonymous");
  });

  it("throws ContentApiError carrying the operator status", async () => {
    fetchMock.mockResolvedValue({ ok: false, status: 404, json: async () => ({}) });
    await expect(getContent("team-a", user, "missing")).rejects.toMatchObject({
      name: "ContentApiError",
      status: 404,
    });
  });
});

describe("writeContentFile / makeContentDir / deleteContent", () => {
  it("PUTs the body and returns the write result", async () => {
    fetchMock.mockResolvedValue(okJson({ path: "f.txt", size: 3, modifiedAt: "t" }));
    const res = await writeContentFile("team-a", user, "dir/f.txt", "abc");

    const [url, init] = fetchMock.mock.calls[0] as [string, { method: string; body: string }];
    expect(url).toBe(`${BASE_URL}/api/v1/workspaces/team-a/content/dir/f.txt`);
    expect(init.method).toBe("PUT");
    expect(init.body).toBe("abc");
    expect(res.size).toBe(3);
  });

  it("POSTs to create a directory", async () => {
    fetchMock.mockResolvedValue(okJson({ path: "d", size: 0, modifiedAt: "t", directory: true }, 201));
    const res = await makeContentDir("team-a", user, "d");
    expect((fetchMock.mock.calls[0][1] as { method: string }).method).toBe("POST");
    expect(res.directory).toBe(true);
  });

  it("DELETEs and resolves on 204", async () => {
    fetchMock.mockResolvedValue({ ok: true, status: 204 });
    await expect(deleteContent("team-a", user, "f.txt")).resolves.toBeUndefined();
    expect((fetchMock.mock.calls[0][1] as { method: string }).method).toBe("DELETE");
  });
});

describe("configuration errors", () => {
  it("throws 500 when OPERATOR_CONTENT_API_URL is unset", async () => {
    delete process.env.OPERATOR_CONTENT_API_URL;
    await expect(getContent("team-a", user, "x")).rejects.toMatchObject({
      name: "ContentApiError",
      status: 500,
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("throws 500 when no signing key is configured", async () => {
    const saved = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    try {
      await expect(getContent("team-a", user, "x")).rejects.toMatchObject({
        name: "ContentApiError",
        status: 500,
      });
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = saved;
    }
  });

  it("is a ContentApiError instance with a status", () => {
    const err = new ContentApiError("boom", 403);
    expect(err).toBeInstanceOf(Error);
    expect(err.status).toBe(403);
  });
});
