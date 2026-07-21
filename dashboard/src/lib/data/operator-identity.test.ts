import { afterAll, beforeAll, describe, expect, it } from "vitest";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import type { User } from "@/lib/auth/types";
import { OperatorApiError, asOperatorError, mintOperatorIdentityToken, operatorBaseURL } from "./operator-identity";

class SubError extends OperatorApiError {
  constructor(message: string, status: number) {
    super(message, status);
    this.name = "SubError";
  }
}

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

beforeAll(() => {
  const { privateKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  const pemPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "omnia-operator-identity-")), "key.pem");
  fs.writeFileSync(pemPath, privateKey, { mode: 0o600 });
  process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = pemPath;
});

afterAll(() => {
  delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
});

function decodeClaims(token: string): Record<string, unknown> {
  return JSON.parse(Buffer.from(token.split(".")[1], "base64url").toString("utf8"));
}

describe("mintOperatorIdentityToken", () => {
  it("mints a token with omnia-operator audience, dashboard issuer, workspace and identity claims", () => {
    const token = mintOperatorIdentityToken("team-a", user);
    const claims = decodeClaims(token);
    expect(claims.aud).toBe("omnia-operator");
    expect(claims.iss).toBe("omnia-dashboard");
    expect(claims.workspace).toBe("team-a");
    expect(claims.identity).toBe("user@example.com");
    expect(claims.groups).toEqual(["eng", "admins"]);
  });

  it("mints an anonymous token (no identity/groups) for anonymous users", () => {
    const token = mintOperatorIdentityToken("team-a", anonUser);
    const claims = decodeClaims(token);
    expect(claims.anonymous).toBe(true);
    expect(claims.identity).toBeUndefined();
    expect(claims.sub).toBe("anonymous");
  });

  it("throws OperatorApiError 500 when no signing key is configured", () => {
    const saved = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    delete process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH;
    try {
      expect(() => mintOperatorIdentityToken("team-a", user)).toThrowError(
        expect.objectContaining({ name: "OperatorApiError", status: 500 }),
      );
    } finally {
      process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH = saved;
    }
  });
});

describe("operatorBaseURL", () => {
  const ENV_VAR = "OPERATOR_TEST_URL";

  afterAll(() => {
    delete process.env[ENV_VAR];
  });

  it("trims trailing slashes", () => {
    process.env[ENV_VAR] = "http://operator.test:8084///";
    expect(operatorBaseURL(ENV_VAR)).toBe("http://operator.test:8084");
  });

  it("throws OperatorApiError(500) when the env var is unset", () => {
    delete process.env[ENV_VAR];
    expect(() => operatorBaseURL(ENV_VAR)).toThrowError(
      expect.objectContaining({ name: "OperatorApiError", status: 500 }),
    );
  });
});

describe("OperatorApiError", () => {
  it("is an Error instance carrying a status", () => {
    const err = new OperatorApiError("boom", 403);
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("OperatorApiError");
    expect(err.status).toBe(403);
  });
});

describe("asOperatorError", () => {
  const wrap = (message: string, status: number) => new SubError(message, status);

  it("returns the value when fn does not throw", () => {
    expect(asOperatorError(() => 42, wrap)).toBe(42);
  });

  it("re-wraps a base OperatorApiError via the factory", () => {
    expect(() =>
      asOperatorError(() => {
        throw new OperatorApiError("config missing", 500);
      }, wrap),
    ).toThrowError(expect.objectContaining({ name: "SubError", status: 500, message: "config missing" }));
  });

  it("passes an already-specific subclass error through untouched", () => {
    const original = new SubError("already specific", 403);
    let caught: unknown;
    try {
      asOperatorError(() => {
        throw original;
      }, wrap);
    } catch (err) {
      caught = err;
    }
    expect(caught).toBe(original);
  });

  it("passes a non-OperatorApiError through untouched", () => {
    const original = new Error("unrelated");
    let caught: unknown;
    try {
      asOperatorError(() => {
        throw original;
      }, wrap);
    } catch (err) {
      caught = err;
    }
    expect(caught).toBe(original);
  });
});
