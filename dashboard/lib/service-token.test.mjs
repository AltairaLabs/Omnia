/**
 * Tests for the service-token mint endpoint — proves the dashboard
 * gates mgmt-plane minting on a TokenReview-validated SA whose name
 * appears in the configured allowlist. Replaces the original "every
 * service mounts the private key" design (Doctor pre-#1057).
 */

import { describe, it, expect, beforeAll } from "vitest";
import { createRequire } from "node:module";
import crypto from "node:crypto";

const require = createRequire(import.meta.url);
const {
  handleServiceTokenRequest,
  parseAllowlist,
  parseServiceAccount,
  saUsernameToAllowlistKey,
  DEFAULT_MINT_SERVICE_ACCOUNTS,
} = require("./service-token.js");

// Constants extracted to satisfy SonarCloud's no-duplicate-string rule
// — these strings appear in many specs as the test fixture identity.
const DOCTOR_KEY = "omnia-system/omnia-doctor";
const DOCTOR_USERNAME = `system:serviceaccount:${DOCTOR_KEY.replace("/", ":")}`;
const BEARER_FAKE = "Bearer fake";
const ALICE_USERNAME = "alice@example.com";
const WORKER_SA = "arena-worker";
const WORKER_USERNAME = `system:serviceaccount:omnia-demo:${WORKER_SA}`;

let signingKey;
let allowlist;

// Stub mintToken — keeps these tests focused on the auth + allowlist
// gating. The actual mint path is covered by mgmt-plane-token.test.
const mintTokenStub = (opts) =>
  `stub-token:sub=${opts.subject}:agent=${opts.agent}:ws=${opts.workspace}`;

function mockReq({ method = "POST", auth, body }) {
  // Minimal IncomingMessage-shape stub. The handler reads
  // headers.authorization, .method, and treats `req` as a Readable —
  // we fake the data/end events with setImmediate so async handlers
  // can register listeners first.
  const listeners = {};
  const req = {
    method,
    headers: auth ? { authorization: auth } : {},
    on(event, fn) {
      listeners[event] = fn;
      return this;
    },
    destroy() {},
  };
  setImmediate(() => {
    if (body !== undefined && listeners.data) {
      listeners.data(Buffer.from(body, "utf8"));
    }
    if (listeners.end) {
      listeners.end();
    }
  });
  return req;
}

function mockRes() {
  const res = {
    statusCode: 0,
    headers: {},
    body: "",
    writeHead(status, headers) {
      this.statusCode = status;
      Object.assign(this.headers, headers);
    },
    end(body) {
      if (body !== undefined) this.body = body;
    },
  };
  return res;
}

beforeAll(() => {
  const { privateKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
  signingKey = privateKey;
  allowlist = new Set([DOCTOR_KEY]);
});

describe("saUsernameToAllowlistKey", () => {
  it("converts the canonical SA username to namespace/name", () => {
    expect(saUsernameToAllowlistKey(DOCTOR_USERNAME))
      .toBe(DOCTOR_KEY);
  });

  it("returns null for non-SA identities", () => {
    expect(saUsernameToAllowlistKey(ALICE_USERNAME)).toBeNull();
    expect(saUsernameToAllowlistKey("system:node:foo")).toBeNull();
    expect(saUsernameToAllowlistKey("")).toBeNull();
  });

  it("returns null when the SA suffix is malformed", () => {
    expect(saUsernameToAllowlistKey("system:serviceaccount:onlyone")).toBeNull();
    expect(saUsernameToAllowlistKey("system:serviceaccount:a:b:c")).toBeNull();
  });
});

describe("parseServiceAccount", () => {
  it("splits a canonical SA username into namespace + name", () => {
    expect(parseServiceAccount(WORKER_USERNAME))
      .toEqual({ namespace: "omnia-demo", name: WORKER_SA });
  });

  it("returns null for non-SA or malformed identities", () => {
    expect(parseServiceAccount(ALICE_USERNAME)).toBeNull();
    expect(parseServiceAccount("system:serviceaccount:onlyone")).toBeNull();
    expect(parseServiceAccount("system:serviceaccount:a:b:c")).toBeNull();
    expect(parseServiceAccount("")).toBeNull();
  });
});

describe("DEFAULT_MINT_SERVICE_ACCOUNTS", () => {
  it("defaults to the operator-created arena worker SA", () => {
    expect(DEFAULT_MINT_SERVICE_ACCOUNTS).toEqual([WORKER_SA]);
  });
});

describe("parseAllowlist", () => {
  it("splits a comma list and trims entries", () => {
    expect(Array.from(parseAllowlist(" omnia-system/omnia-doctor , dev/x ")).sort())
      .toEqual(["dev/x", DOCTOR_KEY]);
  });

  it("returns an empty Set for unset / blank", () => {
    expect(parseAllowlist("").size).toBe(0);
    expect(parseAllowlist(undefined).size).toBe(0);
  });
});

describe("handleServiceTokenRequest", () => {
  function opts(overrides = {}) {
    return {
      signingKey,
      mintToken: mintTokenStub,
      allowlist,
      ttlSeconds: 300,
      ...overrides,
    };
  }

  it("returns 405 on GET (POST-only)", async () => {
    const req = mockReq({ method: "GET" });
    const res = mockRes();
    const handled = await handleServiceTokenRequest(opts(), req, res);
    expect(handled).toBe(true);
    expect(res.statusCode).toBe(405);
    expect(res.headers.Allow).toBe("POST");
  });

  it("returns 401 when Authorization header is missing", async () => {
    const req = mockReq({});
    const res = mockRes();
    await handleServiceTokenRequest(opts(), req, res);
    expect(res.statusCode).toBe(401);
    expect(JSON.parse(res.body).error).toMatch(/missing Bearer/);
  });

  it("returns 401 when Bearer token is empty", async () => {
    const req = mockReq({ auth: "Bearer    " });
    const res = mockRes();
    await handleServiceTokenRequest(opts(), req, res);
    expect(res.statusCode).toBe(401);
  });

  it("returns 401 when TokenReview rejects the token", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({ authenticated: false, error: "expired" }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(401);
    expect(JSON.parse(res.body).error).toMatch(/not authenticated.*expired/);
  });

  it("returns 502 when TokenReview itself errors", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => {
          throw new Error("ECONNREFUSED");
        },
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(502);
    expect(JSON.parse(res.body).error).toMatch(/ECONNREFUSED/);
  });

  it("returns 403 when the authenticated identity is not a service account", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: ALICE_USERNAME,
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(403);
    expect(JSON.parse(res.body).error).toMatch(/not a service account/);
  });

  it("returns 403 when SA is valid but not in the allowlist", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: "system:serviceaccount:dev/some-other-sa",
        }),
      }),
      req,
      res,
    );
    // Even with a slightly malformed username (no `:` between the
    // ns/name parts above), we should still reject with 403, not 200.
    // The slash form is rejected by saUsernameToAllowlistKey, so we
    // get the 403 "not a service account" path. This pin guards
    // against accidentally widening the parse.
    expect(res.statusCode).toBe(403);
  });

  it("403s a non-allowlisted SA whose namespace is not a workspace", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: "system:serviceaccount:omnia-system:other-service",
        }),
        // Not allowlisted → falls to the workspace-gated path; no Workspace
        // for this namespace → denied.
        workspaceLookup: async () => ({ found: false }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(403);
    expect(JSON.parse(res.body).error).toMatch(/not a workspace that permits it to mint/);
  });

  it("mints for a workspace SA listed in mgmtPlaneMintServiceAccounts, scoped to that workspace", async () => {
    const req = mockReq({ auth: BEARER_FAKE, body: JSON.stringify({ agent: "rag-hero" }) });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: WORKER_USERNAME,
        }),
        workspaceLookup: async (ns) => {
          expect(ns).toBe("omnia-demo");
          return { found: true, name: "demo", mintServiceAccounts: [WORKER_SA] };
        },
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(200);
    const out = JSON.parse(res.body);
    // Token scoped to the resolved workspace (not the body), agent from body,
    // subject embeds the calling SA.
    expect(out.token).toBe(
      "stub-token:sub=system-service:omnia-demo/arena-worker:agent=rag-hero:ws=demo",
    );
  });

  it("pins workspace scope to the Workspace even if the body asks for another", async () => {
    const req = mockReq({
      auth: BEARER_FAKE,
      body: JSON.stringify({ agent: "rag-hero", workspace: "victim" }),
    });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: WORKER_USERNAME,
        }),
        workspaceLookup: async () => ({
          found: true,
          name: "demo",
          mintServiceAccounts: [WORKER_SA],
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(200);
    expect(JSON.parse(res.body).token).toMatch(/:ws=demo$/);
  });

  it("403s a workspace SA not listed in mgmtPlaneMintServiceAccounts", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: "system:serviceaccount:omnia-demo:rogue-sa",
        }),
        workspaceLookup: async () => ({
          found: true,
          name: "demo",
          mintServiceAccounts: [WORKER_SA],
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(403);
  });

  it("502s when the workspace lookup errors", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: WORKER_USERNAME,
        }),
        workspaceLookup: async () => {
          throw new Error("list workspaces returned 403");
        },
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(502);
    expect(JSON.parse(res.body).error).toMatch(/workspace lookup failed/);
  });

  it("mints a token for an allowlisted SA and returns expires_at", async () => {
    const req = mockReq({ auth: BEARER_FAKE, body: '{"agent":"a1","workspace":"w1"}' });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: DOCTOR_USERNAME,
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(200);
    const body = JSON.parse(res.body);
    expect(body.token).toContain("stub-token");
    expect(body.token).toContain("sub=system-service:omnia-system/omnia-doctor");
    expect(body.token).toContain("agent=a1");
    expect(body.token).toContain("ws=w1");
    expect(typeof body.expires_at).toBe("number");
    // expires_at should be within ~5s of now+ttl.
    const expected = Math.floor(Date.now() / 1000) + 300;
    expect(Math.abs(body.expires_at - expected)).toBeLessThanOrEqual(5);
  });

  it("falls back to SA name as agent/workspace when body is empty", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: DOCTOR_USERNAME,
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(200);
    const body = JSON.parse(res.body);
    // No body → defaults to the allowlist key for both — the facade's
    // mgmt-plane validator accepts any agent/workspace claim, so this
    // just keeps the audit log meaningful.
    expect(body.token).toContain("agent=omnia-system/omnia-doctor");
    expect(body.token).toContain("ws=omnia-system/omnia-doctor");
  });

  it("returns 400 on malformed JSON body", async () => {
    const req = mockReq({ auth: BEARER_FAKE, body: "{bad json" });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: DOCTOR_USERNAME,
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(400);
  });

  it("returns 400 when the body exceeds the size cap", async () => {
    // Build a req that streams 8KB — more than the 4096-byte cap. We
    // can't share the helper because it sends the body in one chunk;
    // simulate a streaming sender so readBody trips the cap on the
    // FIRST data event.
    const listeners = {};
    const req = {
      method: "POST",
      headers: { authorization: BEARER_FAKE },
      on(event, fn) {
        listeners[event] = fn;
        return this;
      },
      destroy() {},
    };
    const big = "x".repeat(8192);
    setImmediate(() => {
      if (listeners.data) listeners.data(Buffer.from(big, "utf8"));
      if (listeners.end) listeners.end();
    });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: DOCTOR_USERNAME,
        }),
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(400);
    expect(JSON.parse(res.body).error).toMatch(/body too large/);
  });

  it("returns 500 if the mint function throws", async () => {
    const req = mockReq({ auth: BEARER_FAKE });
    const res = mockRes();
    await handleServiceTokenRequest(
      opts({
        tokenReview: async () => ({
          authenticated: true,
          username: DOCTOR_USERNAME,
        }),
        mintToken: () => {
          throw new Error("key invalid");
        },
      }),
      req,
      res,
    );
    expect(res.statusCode).toBe(500);
    expect(JSON.parse(res.body).error).toMatch(/key invalid/);
  });
});
