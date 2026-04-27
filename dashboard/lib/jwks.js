/* eslint-disable @typescript-eslint/no-require-imports */
/**
 * JWKS helpers for the dashboard's mgmt-plane signing key.
 *
 * The dashboard signs short-lived JWTs for the "Try this agent" debug
 * view (see lib/mgmt-plane-token.js) and exposes the matching public
 * key over HTTP at `/api/auth/jwks` so each facade pod can fetch it
 * directly. This replaces the older "operator mirrors a ConfigMap into
 * every workspace namespace" plumbing, which silently went stale every
 * time the source signing Secret was overwritten — see issue notes
 * around the signing-keypair drift bug.
 *
 * Pure CJS so server.js can require() it directly. Tested via vitest
 * (lib/jwks.test.mjs).
 */

const crypto = require("node:crypto");
const fs = require("node:fs");

/**
 * Build the canonical RFC 7638 thumbprint for an RSA JWK. The
 * thumbprint is the SHA-256 of `{"e":...,"kty":...,"n":...}` with keys
 * in lexicographic order and no whitespace, base64url-encoded. Used as
 * the JWK `kid` so consumers can identify the key without trusting
 * issuer-supplied identifiers.
 */
function jwkThumbprint(jwk) {
  if (!jwk || jwk.kty !== "RSA") {
    throw new Error("jwkThumbprint: only RSA JWKs are supported");
  }
  const canonical = JSON.stringify({ e: jwk.e, kty: jwk.kty, n: jwk.n });
  return crypto.createHash("sha256").update(canonical).digest("base64url");
}

/**
 * Convert a Node KeyObject (RSA public key) into a JWK suitable for a
 * `keys` array. Adds the standard `alg`, `use`, and `kid` fields the
 * facade validator looks at. Node's built-in JWK export gives us
 * {kty, n, e}; we layer the rest on top.
 */
function publicJwkFromKey(keyObj) {
  if (!keyObj || keyObj.asymmetricKeyType !== "rsa") {
    throw new Error("publicJwkFromKey: keyObj must be an RSA public key");
  }
  const baseJwk = keyObj.export({ format: "jwk" });
  if (baseJwk.kty !== "RSA") {
    throw new Error(`publicJwkFromKey: exported kty ${baseJwk.kty}, want RSA`);
  }
  const kid = jwkThumbprint(baseJwk);
  return {
    kty: baseJwk.kty,
    n: baseJwk.n,
    e: baseJwk.e,
    alg: "RS256",
    use: "sig",
    kid,
  };
}

/**
 * Build the standard JWKS response envelope ({keys: [...]}) for a
 * single public key. We only ever ship one key today; rotation
 * (overlapping kids) is a follow-up.
 */
function buildJwksResponse(publicKey) {
  return { keys: [publicJwkFromKey(publicKey)] };
}

/**
 * Read a PEM file containing either an x509 CERTIFICATE block (the
 * shape Helm's signing-keypair Secret ships as `tls.crt`) or a bare
 * PUBLIC KEY block, and return a Node KeyObject for the contained RSA
 * public key. Errors propagate so misconfiguration surfaces at boot
 * rather than silently disabling JWKS.
 */
function loadPublicKeyFromCertPath(path) {
  const pem = fs.readFileSync(path, { encoding: "utf8" });
  // crypto.createPublicKey accepts both PEM-encoded certificates and
  // PUBLIC KEY blocks since Node 16, returning the embedded RSA pubkey
  // either way. createPrivateKey would reject — we want the public half.
  return crypto.createPublicKey({ key: pem, format: "pem" });
}

/**
 * JWKS_PATH is the URL path the dashboard exposes the keyset on. We
 * deliberately do NOT use `/.well-known/jwks.json` because Next.js owns
 * the well-known prefix for other purposes; `/api/auth/jwks` keeps it
 * inside the existing API surface and matches the auth-related routes.
 */
const JWKS_PATH = "/api/auth/jwks";

/**
 * JWKS_CACHE_MAX_AGE_SECONDS controls how long downstream consumers
 * (the facade JWKS validator) may cache the keyset. Short enough that a
 * key rotation is picked up quickly; long enough that the dashboard
 * isn't hit on every facade JWT verification.
 */
const JWKS_CACHE_MAX_AGE_SECONDS = 300;

/**
 * Handle an HTTP request against the JWKS endpoint. Returns true when
 * the request was handled (so the caller can short-circuit Next.js).
 * GET returns the keyset; other methods return 405.
 */
function serveJwks(publicKey, req, res) {
  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405, { Allow: "GET, HEAD" });
    res.end();
    return true;
  }
  const body = JSON.stringify(buildJwksResponse(publicKey));
  res.writeHead(200, {
    "Content-Type": "application/jwk-set+json",
    "Cache-Control": `public, max-age=${JWKS_CACHE_MAX_AGE_SECONDS}`,
    "Content-Length": Buffer.byteLength(body, "utf8"),
  });
  if (req.method === "HEAD") {
    res.end();
  } else {
    res.end(body);
  }
  return true;
}

module.exports = {
  jwkThumbprint,
  publicJwkFromKey,
  buildJwksResponse,
  loadPublicKeyFromCertPath,
  serveJwks,
  JWKS_PATH,
  JWKS_CACHE_MAX_AGE_SECONDS,
};
