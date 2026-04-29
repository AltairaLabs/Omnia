/* eslint-disable @typescript-eslint/no-require-imports */
/**
 * Service-principal token endpoint.
 *
 * In-cluster services (Doctor today, Arena dev-console likely next)
 * need mgmt-plane JWTs to dial agent facades. The first cut had each
 * service mount the dashboard's signing-keypair Secret and mint
 * locally; that duplicated the private key into another pod and
 * defeated the point of the JWKS migration (single issuer, many
 * verifiers). This endpoint inverts the flow: services prove their
 * identity with their k8s service-account token, the dashboard calls
 * TokenReview to validate, and the dashboard mints with the only
 * private-key copy in the cluster.
 *
 * Auth: Bearer token in Authorization header MUST be a valid k8s
 *       service-account token whose subject is in the allowlist.
 *
 * Authorization model: an explicit allowlist of service-account
 *       names (configured via OMNIA_DASHBOARD_SERVICE_TOKEN_ALLOWED_SAS,
 *       comma-separated `namespace/sa-name` entries). No ambient
 *       authority — adding a new service is a chart change, not a
 *       runtime grant.
 *
 * Body: optional JSON `{ "agent": "<name>", "workspace": "<name>" }`.
 *       Both default to empty (the facade allows agent/workspace
 *       claims to be absent for service principals).
 *
 * Response: 200 `{ "token": "<jwt>", "expires_at": <unix> }`.
 *           401 missing/invalid Bearer.
 *           403 valid token but SA not allowlisted.
 *           5xx if TokenReview itself errors (network, RBAC).
 *
 * Pure CJS so server.js can require() it directly.
 */

const fs = require("node:fs");
const https = require("node:https");

const SERVICE_TOKEN_PATH = "/api/auth/service-token";

// Default subject when the caller doesn't override. Audit logs see
// this verbatim, so it should make the source obvious.
const DEFAULT_SERVICE_SUBJECT_PREFIX = "system-service:";

// k8s ServiceAccount usernames have the shape
//   `system:serviceaccount:<namespace>:<name>`
// The allowlist takes the shorter `<namespace>/<name>` form because
// that's what an operator types into a Helm value without thinking
// about the prefix.
function saUsernameToAllowlistKey(username) {
  const prefix = "system:serviceaccount:";
  if (!username.startsWith(prefix)) {
    return null;
  }
  const parts = username.slice(prefix.length).split(":");
  if (parts.length !== 2) {
    return null;
  }
  return `${parts[0]}/${parts[1]}`;
}

function parseAllowlist(raw) {
  if (!raw) return new Set();
  return new Set(
    raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean),
  );
}

/**
 * Default in-cluster TokenReview caller. Uses the mounted
 * service-account token to authenticate to the kube API. Skips
 * verification of the apiserver cert because the in-cluster CA
 * bundle is mounted by the kubelet at the same path the kube SDK
 * would normally consume (Node.js doesn't have an opinion).
 *
 * Returns `{ authenticated: bool, username?: string, error?: string }`.
 *
 * Tests inject a stub via opts.tokenReview to bypass cluster IO; this
 * default is therefore unreachable from the unit-test environment.
 * Excluded from coverage — exercised only on a real cluster.
 */
/* c8 ignore start */
async function defaultTokenReview(token) {
  const host = process.env.KUBERNETES_SERVICE_HOST;
  const port = process.env.KUBERNETES_SERVICE_PORT || "443";
  if (!host) {
    return {
      authenticated: false,
      error: "no in-cluster API server (KUBERNETES_SERVICE_HOST unset)",
    };
  }

  let saToken = "";
  try {
    saToken = fs.readFileSync(
      "/var/run/secrets/kubernetes.io/serviceaccount/token",
      "utf8",
    );
  } catch (err) {
    return {
      authenticated: false,
      error: `failed to read service-account token: ${err.message}`,
    };
  }

  let caCert;
  try {
    caCert = fs.readFileSync(
      "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
    );
  } catch {
    // Best-effort: if the CA isn't mounted, fall through and let
    // Node's default trust store handle it. Should never happen
    // in-cluster but we don't want a missing file to be the reason
    // a real cluster's TokenReview never succeeds.
    caCert = undefined;
  }

  const body = JSON.stringify({
    apiVersion: "authentication.k8s.io/v1",
    kind: "TokenReview",
    spec: { token },
  });

  // Built-in node:https avoids pulling in undici as a dep — we'd
  // need it only for fetch's CA-pinning path, and a single TokenReview
  // call doesn't justify it. Standard https.request handles ca via
  // the request options.
  return new Promise((resolve) => {
    const req = https.request(
      {
        host,
        port,
        method: "POST",
        path: "/apis/authentication.k8s.io/v1/tokenreviews",
        headers: {
          Authorization: `Bearer ${saToken.trim()}`,
          "Content-Type": "application/json",
          "Content-Length": Buffer.byteLength(body, "utf8"),
        },
        ca: caCert,
      },
      (resp) => {
        const chunks = [];
        resp.on("data", (c) => chunks.push(c));
        resp.on("end", () => {
          const text = Buffer.concat(chunks).toString("utf8");
          if (resp.statusCode < 200 || resp.statusCode >= 300) {
            resolve({
              authenticated: false,
              error: `TokenReview returned ${resp.statusCode}: ${text.slice(0, 200)}`,
            });
            return;
          }
          let result;
          try {
            result = JSON.parse(text);
          } catch (err) {
            resolve({
              authenticated: false,
              error: `TokenReview body parse: ${err.message}`,
            });
            return;
          }
          const status = (result && result.status) || {};
          if (!status.authenticated) {
            resolve({
              authenticated: false,
              error: status.error || "token rejected by TokenReview",
            });
            return;
          }
          resolve({
            authenticated: true,
            username: status.user && status.user.username,
          });
        });
      },
    );
    req.on("error", (err) => {
      resolve({
        authenticated: false,
        error: `TokenReview request failed: ${err.message}`,
      });
    });
    req.write(body);
    req.end();
  });
}
/* c8 ignore stop */

/**
 * readBody collects the request body up to a small size cap. The
 * mint endpoint takes <200 bytes of legitimate input; anything more
 * is a misuse.
 */
function readBody(req, maxBytes = 4096) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let total = 0;
    req.on("data", (chunk) => {
      total += chunk.length;
      if (total > maxBytes) {
        reject(new Error("body too large"));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}

/**
 * handleServiceTokenRequest is the HTTP handler. Mirrors the shape of
 * lib/jwks.js's serveJwks: returns true when the request was handled
 * (so server.js can short-circuit Next.js).
 *
 * opts.signingKey      — KeyObject loaded by the caller (required).
 * opts.mintToken       — function(opts) → JWT string (required;
 *                        passes through to lib/mgmt-plane-token).
 * opts.allowlist       — Set<string> of "<ns>/<sa>" keys (required).
 * opts.tokenReview     — async (token) → { authenticated, username, error }
 *                        Defaults to the in-cluster TokenReview call.
 * opts.ttlSeconds      — JWT TTL passed to mintToken.
 */
async function handleServiceTokenRequest(opts, req, res) {
  if (req.method !== "POST") {
    res.writeHead(405, { Allow: "POST" });
    res.end();
    return true;
  }

  const auth = req.headers && req.headers.authorization;
  if (!auth || !auth.startsWith("Bearer ")) {
    writeJSON(res, 401, { error: "missing Bearer token" });
    return true;
  }
  const presentedToken = auth.slice("Bearer ".length).trim();
  if (!presentedToken) {
    writeJSON(res, 401, { error: "empty Bearer token" });
    return true;
  }

  const tokenReview = opts.tokenReview || defaultTokenReview;
  let review;
  try {
    review = await tokenReview(presentedToken);
  } catch (err) {
    writeJSON(res, 502, {
      error: `TokenReview call failed: ${err.message}`,
    });
    return true;
  }
  if (!review.authenticated) {
    writeJSON(res, 401, {
      error: `presented token is not authenticated: ${review.error || "unknown"}`,
    });
    return true;
  }

  const allowKey = saUsernameToAllowlistKey(review.username || "");
  if (!allowKey) {
    writeJSON(res, 403, {
      error: `presented identity ${JSON.stringify(review.username)} is not a service account`,
    });
    return true;
  }
  if (!opts.allowlist.has(allowKey)) {
    writeJSON(res, 403, {
      error: `service account ${JSON.stringify(allowKey)} is not in the mgmt-plane mint allowlist`,
    });
    return true;
  }

  let body = {};
  try {
    const raw = await readBody(req);
    if (raw) {
      body = JSON.parse(raw);
    }
  } catch (err) {
    writeJSON(res, 400, { error: `bad request body: ${err.message}` });
    return true;
  }

  const agent = typeof body.agent === "string" ? body.agent : "";
  const workspace = typeof body.workspace === "string" ? body.workspace : "";
  // Subject embeds the SA so audit logs can attribute the call back
  // to the service even when the JWT travels through the facade
  // chain. ToolPolicy still keys off identity.origin to gate
  // service-principal traffic separately from human admins.
  const subject = `${DEFAULT_SERVICE_SUBJECT_PREFIX}${allowKey}`;

  let token;
  try {
    token = opts.mintToken({
      key: opts.signingKey,
      subject,
      agent: agent || allowKey,
      workspace: workspace || allowKey,
      ttlSeconds: opts.ttlSeconds,
    });
  } catch (err) {
    writeJSON(res, 500, { error: `mint failed: ${err.message}` });
    return true;
  }

  // expires_at lets the client cache without parsing the JWT body.
  const ttl = opts.ttlSeconds || 300;
  writeJSON(res, 200, {
    token,
    expires_at: Math.floor(Date.now() / 1000) + ttl,
    subject,
  });
  return true;
}

function writeJSON(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(payload, "utf8"),
  });
  res.end(payload);
}

module.exports = {
  SERVICE_TOKEN_PATH,
  handleServiceTokenRequest,
  parseAllowlist,
  saUsernameToAllowlistKey,
};
