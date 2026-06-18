/* eslint-disable @typescript-eslint/no-require-imports */
/**
 * Management-plane token minter.
 *
 * server.js (CommonJS) calls this when proxying a "Try this agent" debug
 * WebSocket upgrade so the upstream facade sees an Authorization: Bearer
 * <jwt> header it can validate against the dashboard's signing key
 * (mounted into every facade pod by PR 1b's Workspace controller).
 *
 * Why mint server-side and not in the browser:
 *   - The signing key never leaves the dashboard pod.
 *   - The browser's WebSocket API can't set custom headers, so a
 *     browser-side token would have to ride a query param — an extra
 *     attack surface and an extra leakable secret in browser history.
 *   - The dashboard already authenticates the user before serving the
 *     debug page, so the proxy hop is the right boundary at which to
 *     mint.
 *
 * Pure CJS so server.js can require() it directly. Tested via vitest.
 */

const crypto = require("node:crypto");
const fs = require("node:fs");
const { publicJwkFromKey } = require("./jwks.js");

/**
 * Default issuer/audience values. Kept in sync with the Go-side defaults
 * in internal/facade/auth/mgmt_plane.go (DefaultMgmtPlaneIssuer /
 * DefaultMgmtPlaneAudience). Mismatches cause the facade to 401 with no
 * obvious clue, so override both sides together if you change either.
 */
const DEFAULT_ISSUER = "omnia-dashboard";
const DEFAULT_AUDIENCE = "omnia-facade";

/**
 * Audience for operator content-API identity tokens. Distinct from
 * DEFAULT_AUDIENCE so a content token can't be replayed against the facade
 * (which requires aud=omnia-facade) and vice versa. Kept in sync with the
 * Go-side AudienceContentAPI in internal/api/authz/identity.go.
 */
const CONTENT_API_AUDIENCE = "omnia-operator";

/** Origin claim the facade requires to admit a mgmt-plane JWT. */
const MGMT_PLANE_ORIGIN = "management-plane";

/**
 * Default token lifetime. Long enough that an admin's debug session
 * doesn't drop in the middle of a chat, short enough that a leaked token
 * isn't useful for long. The facade rejects expired tokens with 401 so
 * the dashboard would simply mint a fresh one on reconnect.
 */
const DEFAULT_TTL_SECONDS = 5 * 60;

/**
 * base64url encode a Buffer or string per RFC 7515 §2 / RFC 7519 §3.
 *
 * Node's Buffer.toString("base64url") does this in one shot; we wrap it
 * so the helper's callers don't need to remember the exact encoding name.
 */
function base64url(input) {
  const buf = typeof input === "string" ? Buffer.from(input, "utf8") : input;
  return buf.toString("base64url");
}

/**
 * Read and parse an RSA private key from a PEM file. Accepts either a
 * PKCS#1 ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") block; Helm's
 * genSelfSigned emits PKCS#8 alongside the certificate. Returns a
 * KeyObject suitable for crypto.sign.
 *
 * Throws if the file is missing, unreadable, or not a parseable RSA key
 * — the caller (server.js boot) treats that as fatal so we don't silently
 * skip mgmt-plane minting in production.
 */
function loadSigningKey(path) {
  const pem = fs.readFileSync(path, { encoding: "utf8" });
  const key = crypto.createPrivateKey({ key: pem, format: "pem" });
  if (key.asymmetricKeyType !== "rsa") {
    throw new Error(
      `mgmt-plane signing key at ${path} is ${key.asymmetricKeyType}, expected rsa`,
    );
  }
  return key;
}

/**
 * Mint a fresh mgmt-plane JWT (RS256) signed by the supplied private key.
 *
 * @param {Object} opts
 * @param {crypto.KeyObject} opts.key - RSA private key from loadSigningKey()
 * @param {string} opts.subject       - admin username — surfaces as identity.subject in ToolPolicy
 * @param {string} opts.agent         - target AgentRuntime name
 * @param {string} opts.workspace     - target workspace
 * @param {string} [opts.issuer]      - defaults to DEFAULT_ISSUER
 * @param {string} [opts.audience]    - defaults to DEFAULT_AUDIENCE
 * @param {number} [opts.ttlSeconds]  - defaults to DEFAULT_TTL_SECONDS
 * @param {() => number} [opts.now]   - clock injection for tests; returns ms since epoch
 * @returns {string} compact JWT (header.payload.signature)
 */
function mintToken(opts) {
  if (!opts || !opts.key) {
    throw new Error("mintToken: opts.key is required");
  }
  if (!opts.subject) {
    throw new Error("mintToken: opts.subject is required");
  }
  if (!opts.agent) {
    throw new Error("mintToken: opts.agent is required");
  }
  if (!opts.workspace) {
    throw new Error("mintToken: opts.workspace is required");
  }

  const issuer = opts.issuer || DEFAULT_ISSUER;
  const audience = opts.audience || DEFAULT_AUDIENCE;
  const ttlSeconds = opts.ttlSeconds || DEFAULT_TTL_SECONDS;
  const nowMs = opts.now ? opts.now() : Date.now();
  const nowSec = Math.floor(nowMs / 1000);

  const payload = {
    iss: issuer,
    sub: opts.subject,
    aud: audience,
    exp: nowSec + ttlSeconds,
    nbf: nowSec - 1,
    iat: nowSec,
    origin: MGMT_PLANE_ORIGIN,
    agent: opts.agent,
    workspace: opts.workspace,
  };
  return signJwt(opts.key, payload);
}

/**
 * Sign a JWT payload (RS256) with key, deriving the kid from the matching
 * public JWK so JWKS consumers can pick the right key during rotation.
 * createPublicKey accepts the private KeyObject and yields the public half.
 * Shared by mintToken and mintIdentityToken.
 */
function signJwt(key, payload) {
  const publicKey = crypto.createPublicKey(key);
  const kid = publicJwkFromKey(publicKey).kid;
  const header = { alg: "RS256", typ: "JWT", kid };
  const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(payload))}`;
  const signature = crypto.sign("RSA-SHA256", Buffer.from(signingInput, "utf8"), key);
  return `${signingInput}.${base64url(signature)}`;
}

/**
 * Mint a content-API identity JWT (RS256). Unlike mintToken (facade audience,
 * fixed agent), this carries the authenticated end-user's {identity, groups,
 * anonymous} so the operator content API can recompute the workspace role
 * server-side — it never trusts a role claim. The CONTENT_API_AUDIENCE keeps
 * content tokens and facade tokens non-interchangeable.
 *
 * @param {Object} opts
 * @param {crypto.KeyObject} opts.key       - RSA private key from loadSigningKey()
 * @param {string} opts.workspace           - target workspace (required)
 * @param {string} [opts.identity]          - email-or-username; omitted when anonymous
 * @param {string[]} [opts.groups]          - IdP groups
 * @param {boolean} [opts.anonymous]        - principal admitted anonymously
 * @param {string} [opts.issuer]            - defaults to DEFAULT_ISSUER
 * @param {string} [opts.audience]          - defaults to CONTENT_API_AUDIENCE
 * @param {number} [opts.ttlSeconds]        - defaults to DEFAULT_TTL_SECONDS
 * @param {() => number} [opts.now]         - clock injection for tests; ms since epoch
 * @returns {string} compact JWT (header.payload.signature)
 */
function mintIdentityToken(opts) {
  if (!opts || !opts.key) {
    throw new Error("mintIdentityToken: opts.key is required");
  }
  if (!opts.workspace) {
    throw new Error("mintIdentityToken: opts.workspace is required");
  }
  const issuer = opts.issuer || DEFAULT_ISSUER;
  const audience = opts.audience || CONTENT_API_AUDIENCE;
  const ttlSeconds = opts.ttlSeconds || DEFAULT_TTL_SECONDS;
  const nowMs = opts.now ? opts.now() : Date.now();
  const nowSec = Math.floor(nowMs / 1000);
  const anonymous = Boolean(opts.anonymous);
  const identity = anonymous ? "" : opts.identity || "";
  const groups = Array.isArray(opts.groups) ? opts.groups : [];

  const payload = {
    iss: issuer,
    sub: identity || "anonymous",
    aud: audience,
    exp: nowSec + ttlSeconds,
    nbf: nowSec - 1,
    iat: nowSec,
    workspace: opts.workspace,
  };
  if (identity) {
    payload.identity = identity;
  }
  if (groups.length > 0) {
    payload.groups = groups;
  }
  if (anonymous) {
    payload.anonymous = true;
  }
  return signJwt(opts.key, payload);
}

module.exports = {
  loadSigningKey,
  mintToken,
  mintIdentityToken,
  DEFAULT_ISSUER,
  DEFAULT_AUDIENCE,
  CONTENT_API_AUDIENCE,
  DEFAULT_TTL_SECONDS,
  MGMT_PLANE_ORIGIN,
};
