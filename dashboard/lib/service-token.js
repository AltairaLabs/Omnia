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

// parseServiceAccount splits a k8s ServiceAccount username
// (`system:serviceaccount:<namespace>:<name>`) into its parts, or returns
// null when the username is not a service account.
function parseServiceAccount(username) {
  const prefix = "system:serviceaccount:";
  if (!username || !username.startsWith(prefix)) {
    return null;
  }
  const parts = username.slice(prefix.length).split(":");
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    return null;
  }
  return { namespace: parts[0], name: parts[1] };
}

// DEFAULT_MINT_SERVICE_ACCOUNTS is the implicit mgmtPlaneMintServiceAccounts
// list when a Workspace doesn't set one — keeps the operator-created Arena
// worker SA working without every Workspace having to spell it out. Kept in
// sync with the Go-side default (api/v1alpha1 Workspace consumers).
const DEFAULT_MINT_SERVICE_ACCOUNTS = ["arena-worker"];

// Workspace CRD coordinates (cluster-scoped).
const WORKSPACE_API = "/apis/omnia.altairalabs.ai/v1alpha1/workspaces";

// defaultWorkspaceLookup finds the (cluster-scoped) Workspace whose
// spec.namespace.name equals the caller's namespace — the authoritative
// "is this a real, operator-managed workspace namespace" check. Matching on
// the Workspace's own declared namespace (not a namespace label) prevents a
// tenant from spoofing membership by labelling their namespace.
//
// Returns { found, name, mintServiceAccounts }. mintServiceAccounts falls back
// to DEFAULT_MINT_SERVICE_ACCOUNTS when the Workspace omits the field.
//
// Tests inject a stub via opts.workspaceLookup; this in-cluster default is
// excluded from coverage (exercised only against a real apiserver).
/* c8 ignore start */
async function defaultWorkspaceLookup(namespace) {
  const host = process.env.KUBERNETES_SERVICE_HOST;
  const port = process.env.KUBERNETES_SERVICE_PORT || "443";
  if (!host) {
    throw new Error("no in-cluster API server (KUBERNETES_SERVICE_HOST unset)");
  }
  const saToken = fs.readFileSync(
    "/var/run/secrets/kubernetes.io/serviceaccount/token",
    "utf8",
  );
  let caCert;
  try {
    caCert = fs.readFileSync(
      "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
    );
  } catch {
    caCert = undefined;
  }

  const body = await new Promise((resolve, reject) => {
    const req = https.request(
      {
        host,
        port,
        method: "GET",
        path: WORKSPACE_API,
        headers: {
          Authorization: `Bearer ${saToken.trim()}`,
          Accept: "application/json",
        },
        ca: caCert,
      },
      (resp) => {
        const chunks = [];
        resp.on("data", (c) => chunks.push(c));
        resp.on("end", () => {
          const text = Buffer.concat(chunks).toString("utf8");
          if (resp.statusCode < 200 || resp.statusCode >= 300) {
            reject(
              new Error(
                `list workspaces returned ${resp.statusCode}: ${text.slice(0, 200)}`,
              ),
            );
            return;
          }
          resolve(text);
        });
      },
    );
    req.on("error", reject);
    req.end();
  });

  const list = JSON.parse(body);
  const items = (list && list.items) || [];
  const ws = items.find(
    (w) => w && w.spec && w.spec.namespace && w.spec.namespace.name === namespace,
  );
  if (!ws) {
    return { found: false };
  }
  const configured = ws.spec.mgmtPlaneMintServiceAccounts;
  const mintServiceAccounts =
    Array.isArray(configured) && configured.length > 0
      ? configured
      : DEFAULT_MINT_SERVICE_ACCOUNTS;
  return { found: true, name: ws.metadata.name, mintServiceAccounts };
}
/* c8 ignore stop */

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
// resolveWorkspaceScope authorizes a non-allowlisted caller via the
// workspace-gated path: a Workspace must exist for the caller's namespace and
// the caller SA must be in its mgmtPlaneMintServiceAccounts. Returns
// { ok:true, workspace } on success, or { ok:false, status, error } describing
// the rejection (403 unauthorized, 502 lookup failure).
async function resolveWorkspaceScope(lookup, sa, allowKey) {
  let ws;
  try {
    ws = await lookup(sa.namespace);
  } catch (err) {
    return { ok: false, status: 502, error: `workspace lookup failed: ${err.message}` };
  }
  const allowed =
    ws &&
    ws.found &&
    Array.isArray(ws.mintServiceAccounts) &&
    ws.mintServiceAccounts.includes(sa.name);
  if (!allowed) {
    return {
      ok: false,
      status: 403,
      error: `service account ${JSON.stringify(allowKey)} is not in the static mint allowlist, and ${JSON.stringify(sa.namespace)} is not a workspace that permits it to mint`,
    };
  }
  return { ok: true, workspace: ws.name };
}

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

  const sa = parseServiceAccount(review.username || "");
  if (!sa) {
    writeJSON(res, 403, {
      error: `presented identity ${JSON.stringify(review.username)} is not a service account`,
    });
    return true;
  }
  const allowKey = `${sa.namespace}/${sa.name}`;

  // Authorize via one of two independent paths and resolve the authoritative
  // workspace scope:
  //   1. Static allowlist (fixed infra SAs, e.g. Doctor) — the body sets scope.
  //   2. Workspace-gated: a Workspace exists for the caller's namespace AND the
  //      caller SA is in its mgmtPlaneMintServiceAccounts (default
  //      ["arena-worker"]). The token is then scoped to that workspace,
  //      ignoring the body — a tenant can't mint for someone else's workspace.
  let forcedWorkspace = "";
  if (!opts.allowlist.has(allowKey)) {
    const lookup = opts.workspaceLookup || defaultWorkspaceLookup;
    const scope = await resolveWorkspaceScope(lookup, sa, allowKey);
    if (!scope.ok) {
      writeJSON(res, scope.status, { error: scope.error });
      return true;
    }
    forcedWorkspace = scope.workspace;
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
      // Workspace-gated mints are pinned to the resolved workspace; static
      // allowlist callers keep the body-supplied (or allowKey) scope.
      workspace: forcedWorkspace || workspace || allowKey,
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
  parseServiceAccount,
  saUsernameToAllowlistKey,
  defaultWorkspaceLookup,
  DEFAULT_MINT_SERVICE_ACCOUNTS,
};
