/**
 * Tests for the mgmt-plane token minter — proves the JWTs we ship to
 * facade pods carry the exact claim shape facade auth.MgmtPlaneValidator
 * expects (RS256, iss=omnia-dashboard, aud=omnia-facade,
 * origin=management-plane, exp/nbf/iat populated).
 */

import { describe, it, expect, beforeAll } from "vitest";
import { createRequire } from "node:module";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);
const {
  loadSigningKey,
  mintToken,
  DEFAULT_ISSUER,
  DEFAULT_AUDIENCE,
  DEFAULT_TTL_SECONDS,
  MGMT_PLANE_ORIGIN,
} = require("./mgmt-plane-token.js");

let signingKey;
let publicKey;
let pemPath;

const tmpPrefix = "omnia-mgmt-";

beforeAll(() => {
  const { privateKey, publicKey: pubKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    publicKeyEncoding: { type: "spki", format: "pem" },
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
  });
  pemPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), tmpPrefix)), "key.pem");
  fs.writeFileSync(pemPath, privateKey, { mode: 0o600 });
  signingKey = loadSigningKey(pemPath);
  publicKey = crypto.createPublicKey(pubKey);
});

function decodeJwt(jwt) {
  const [headerB64, payloadB64, sigB64] = jwt.split(".");
  return {
    header: JSON.parse(Buffer.from(headerB64, "base64url").toString("utf8")),
    payload: JSON.parse(Buffer.from(payloadB64, "base64url").toString("utf8")),
    signingInput: Buffer.from(`${headerB64}.${payloadB64}`, "utf8"),
    signature: Buffer.from(sigB64, "base64url"),
  };
}

describe("loadSigningKey", () => {
  it("loads a PKCS#8 RSA private key", () => {
    expect(signingKey.asymmetricKeyType).toBe("rsa");
  });

  it("rejects non-RSA keys with a clear error", () => {
    const ec = crypto.generateKeyPairSync("ec", {
      namedCurve: "P-256",
      privateKeyEncoding: { type: "pkcs8", format: "pem" },
      publicKeyEncoding: { type: "spki", format: "pem" },
    });
    const ecPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), tmpPrefix)), "ec.pem");
    fs.writeFileSync(ecPath, ec.privateKey);
    expect(() => loadSigningKey(ecPath)).toThrowError(/expected rsa/);
  });

  it("propagates ENOENT for missing files", () => {
    expect(() => loadSigningKey("/no/such/file.pem")).toThrow();
  });

  it("rejects malformed PEM", () => {
    const garbagePath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), tmpPrefix)), "garbage.pem");
    fs.writeFileSync(garbagePath, "this is not pem");
    expect(() => loadSigningKey(garbagePath)).toThrow();
  });
});

describe("mintToken", () => {
  const baseOpts = {
    subject: "admin@example.com",
    agent: "test-agent",
    workspace: "default",
  };

  it("includes the JWK thumbprint as kid in the header", () => {
    const { publicJwkFromKey } = require("./jwks.js");
    const expectedKid = publicJwkFromKey(publicKey).kid;
    const { header } = decodeJwt(mintToken({ key: signingKey, ...baseOpts }));
    expect(header.kid).toBe(expectedKid);
  });

  it("returns a three-segment JWT", () => {
    const jwt = mintToken({ key: signingKey, ...baseOpts });
    expect(jwt.split(".")).toHaveLength(3);
  });

  it("signs with RS256", () => {
    const { header } = decodeJwt(mintToken({ key: signingKey, ...baseOpts }));
    expect(header.alg).toBe("RS256");
    expect(header.typ).toBe("JWT");
  });

  it("includes the facade-required claims", () => {
    const { payload } = decodeJwt(mintToken({ key: signingKey, ...baseOpts }));
    expect(payload.iss).toBe(DEFAULT_ISSUER);
    expect(payload.aud).toBe(DEFAULT_AUDIENCE);
    expect(payload.origin).toBe(MGMT_PLANE_ORIGIN);
    expect(payload.sub).toBe("admin@example.com");
    expect(payload.agent).toBe("test-agent");
    expect(payload.workspace).toBe("default");
    expect(typeof payload.exp).toBe("number");
    expect(typeof payload.iat).toBe("number");
    expect(typeof payload.nbf).toBe("number");
  });

  it("respects custom issuer / audience overrides", () => {
    const { payload } = decodeJwt(
      mintToken({
        key: signingKey,
        ...baseOpts,
        issuer: "custom-iss",
        audience: "custom-aud",
      }),
    );
    expect(payload.iss).toBe("custom-iss");
    expect(payload.aud).toBe("custom-aud");
  });

  it("defaults exp to now + DEFAULT_TTL_SECONDS", () => {
    const fixedNow = 1_700_000_000_000;
    const { payload } = decodeJwt(
      mintToken({ key: signingKey, ...baseOpts, now: () => fixedNow }),
    );
    const expectedNowSec = Math.floor(fixedNow / 1000);
    expect(payload.iat).toBe(expectedNowSec);
    expect(payload.exp).toBe(expectedNowSec + DEFAULT_TTL_SECONDS);
  });

  it("honours explicit ttlSeconds", () => {
    const fixedNow = 1_700_000_000_000;
    const { payload } = decodeJwt(
      mintToken({ key: signingKey, ...baseOpts, ttlSeconds: 30, now: () => fixedNow }),
    );
    expect(payload.exp - payload.iat).toBe(30);
  });

  it("produces a signature the public key verifies", () => {
    const jwt = mintToken({ key: signingKey, ...baseOpts });
    const { signingInput, signature } = decodeJwt(jwt);
    const ok = crypto.verify("RSA-SHA256", signingInput, publicKey, signature);
    expect(ok).toBe(true);
  });

  it("rejects calls without a key", () => {
    expect(() => mintToken({ ...baseOpts })).toThrowError(/opts.key is required/);
  });

  it.each(["subject", "agent", "workspace"])("rejects calls without %s", (missing) => {
    const opts = { key: signingKey, ...baseOpts };
    delete opts[missing];
    expect(() => mintToken(opts)).toThrowError(new RegExp(`opts.${missing} is required`));
  });
});
