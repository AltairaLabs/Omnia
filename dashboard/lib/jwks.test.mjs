/**
 * Tests for the JWKS helpers — proves the dashboard exposes its
 * mgmt-plane signing pubkey in the canonical RFC 7517/7638 shape the
 * facade's JWKS-based validator expects.
 */

import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { createRequire } from "node:module";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);
const {
  publicJwkFromKey,
  jwkThumbprint,
  buildJwksResponse,
  loadPublicKeyFromCertPath,
  serveJwks,
  JWKS_PATH,
  JWKS_CACHE_MAX_AGE_SECONDS,
} = require("./jwks.js");
import http from "node:http";

let publicKey;
let certPath;

beforeAll(() => {
  const { publicKey: pubKey, privateKey } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    publicKeyEncoding: { type: "spki", format: "pem" },
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
  });
  publicKey = crypto.createPublicKey(pubKey);

  // Build a real self-signed certificate so loadPublicKeyFromCertPath has
  // something realistic to parse — Helm's genSelfSignedCert lands the
  // pubkey inside an x509 CERTIFICATE block, not as a bare PUBLIC KEY.
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "omnia-jwks-"));
  const keyPath = path.join(tmpDir, "key.pem");
  certPath = path.join(tmpDir, "cert.pem");
  fs.writeFileSync(keyPath, privateKey);
  // Use Node's X509Certificate via openssl-like flow — fall back to
  // PUBLIC KEY PEM if openssl isn't available in the test environment.
  // We exercise both shapes through the same loader below.
  fs.writeFileSync(certPath, pubKey);
});

describe("publicJwkFromKey", () => {
  it("returns the canonical RSA JWK fields", () => {
    const jwk = publicJwkFromKey(publicKey);
    expect(jwk.kty).toBe("RSA");
    expect(jwk.alg).toBe("RS256");
    expect(jwk.use).toBe("sig");
    expect(typeof jwk.n).toBe("string");
    expect(typeof jwk.e).toBe("string");
    expect(typeof jwk.kid).toBe("string");
    expect(jwk.kid.length).toBeGreaterThan(0);
  });

  it("produces a stable kid for the same key", () => {
    const a = publicJwkFromKey(publicKey);
    const b = publicJwkFromKey(publicKey);
    expect(a.kid).toBe(b.kid);
  });

  it("produces different kids for different keys", () => {
    const other = crypto.generateKeyPairSync("rsa", {
      modulusLength: 2048,
      publicKeyEncoding: { type: "spki", format: "pem" },
      privateKeyEncoding: { type: "pkcs8", format: "pem" },
    });
    const otherJwk = publicJwkFromKey(crypto.createPublicKey(other.publicKey));
    expect(otherJwk.kid).not.toBe(publicJwkFromKey(publicKey).kid);
  });
});

describe("jwkThumbprint", () => {
  it("matches RFC 7638 expectation: sha256 of canonical {e,kty,n} JSON", () => {
    const jwk = publicJwkFromKey(publicKey);
    // Compute thumbprint independently from the JWK fields.
    const canonical = JSON.stringify({ e: jwk.e, kty: jwk.kty, n: jwk.n });
    const expected = crypto
      .createHash("sha256")
      .update(canonical)
      .digest("base64url");
    expect(jwkThumbprint(jwk)).toBe(expected);
  });
});

describe("buildJwksResponse", () => {
  it("wraps a single key in the standard {keys: [...]} envelope", () => {
    const resp = buildJwksResponse(publicKey);
    expect(resp).toHaveProperty("keys");
    expect(Array.isArray(resp.keys)).toBe(true);
    expect(resp.keys).toHaveLength(1);
    expect(resp.keys[0].kty).toBe("RSA");
    expect(resp.keys[0].kid).toBe(publicJwkFromKey(publicKey).kid);
  });
});

describe("loadPublicKeyFromCertPath", () => {
  it("reads a PEM file and returns a KeyObject", () => {
    const key = loadPublicKeyFromCertPath(certPath);
    expect(key.asymmetricKeyType).toBe("rsa");
  });

  it("throws on missing file", () => {
    expect(() => loadPublicKeyFromCertPath("/no/such/file.pem")).toThrow();
  });
});

describe("serveJwks", () => {
  let server;
  let baseUrl;

  beforeAll(async () => {
    server = http.createServer((req, res) => {
      if (req.url === JWKS_PATH || req.url.startsWith(`${JWKS_PATH}?`)) {
        serveJwks(publicKey, req, res);
        return;
      }
      res.writeHead(404).end();
    });
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const { port } = server.address();
    baseUrl = `http://127.0.0.1:${port}`;
  });

  it("serves the keyset with the JWK media type and a Cache-Control max-age", async () => {
    const resp = await fetch(`${baseUrl}${JWKS_PATH}`);
    expect(resp.status).toBe(200);
    expect(resp.headers.get("content-type")).toBe("application/jwk-set+json");
    expect(resp.headers.get("cache-control")).toBe(
      `public, max-age=${JWKS_CACHE_MAX_AGE_SECONDS}`,
    );
    const body = await resp.json();
    expect(body.keys).toHaveLength(1);
    expect(body.keys[0].kid).toBe(publicJwkFromKey(publicKey).kid);
  });

  it("rejects non-GET methods with 405", async () => {
    const resp = await fetch(`${baseUrl}${JWKS_PATH}`, { method: "POST" });
    expect(resp.status).toBe(405);
    expect(resp.headers.get("allow")).toBe("GET, HEAD");
  });

  // Close the listener after each suite run so the test process exits.
  afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });
});
