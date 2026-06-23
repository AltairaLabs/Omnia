/* eslint-disable @typescript-eslint/no-require-imports */
/**
 * Resolve an agent's workspace NAME from its K8s namespace + name.
 *
 * The dashboard WS proxy only learns the namespace and agent name from the
 * upgrade path (`/api/agents/{namespace}/{name}/ws`), but the facade's
 * mgmt-plane JWT validator expects the `workspace` claim to be the workspace
 * NAME — the value Go computes in `pkg/k8s/getters.go` `ResolveWorkspaceName`:
 * the AgentRuntime's `omnia.altairalabs.ai/workspace` label, falling back to
 * the namespace's label. Minting with the namespace instead (the old bug,
 * #1552) fails closed with 401 whenever name != namespace.
 *
 * This module mirrors that resolution so the proxy mints a claim the facade
 * will admit. The in-cluster API lookups follow the same SA-token + CA-cert
 * `https.request` pattern as lib/service-token.js and are excluded from
 * coverage (exercised only against a real apiserver); the resolution logic on
 * top of them is unit-tested via injected lookups.
 *
 * Pure CJS so server.js can require() it directly.
 */

const fs = require("node:fs");
const https = require("node:https");

// Canonical workspace label. Kept in sync with the Go-side constant in
// pkg/k8s/getters.go (workspaceLabel).
const WORKSPACE_LABEL = "omnia.altairalabs.ai/workspace";

/**
 * resolveWorkspaceName mirrors Go ResolveWorkspaceName: the resource's own
 * `omnia.altairalabs.ai/workspace` label wins; otherwise fall back to the
 * namespace's label. Returns the workspace name (possibly "" when nothing is
 * labelled — which the facade also computes, so an empty claim still matches).
 *
 * Lookups are injectable for testing:
 *   opts.agentRuntimeLabel(namespace, name) -> Promise<string>
 *   opts.namespaceLabel(namespace)          -> Promise<string>
 * Both default to the in-cluster API implementations below.
 */
async function resolveWorkspaceName(namespace, name, opts = {}) {
  const agentRuntimeLabel = opts.agentRuntimeLabel || defaultAgentRuntimeLabel;
  const namespaceLabel = opts.namespaceLabel || defaultNamespaceLabel;

  const own = await agentRuntimeLabel(namespace, name);
  if (own) {
    return own;
  }
  return namespaceLabel(namespace);
}

// k8sGetJSON performs an authenticated in-cluster GET against the kube API
// using the mounted service-account token + CA bundle, returning the parsed
// JSON body. Mirrors lib/service-token.js's request shape.
/* c8 ignore start */
function k8sGetJSON(apiPath) {
  const host = process.env.KUBERNETES_SERVICE_HOST;
  const port = process.env.KUBERNETES_SERVICE_PORT || "443";
  if (!host) {
    return Promise.reject(
      new Error("no in-cluster API server (KUBERNETES_SERVICE_HOST unset)"),
    );
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

  return new Promise((resolve, reject) => {
    const req = https.request(
      {
        host,
        port,
        method: "GET",
        path: apiPath,
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
            reject(new Error(`GET ${apiPath} returned ${resp.statusCode}: ${text.slice(0, 200)}`));
            return;
          }
          try {
            resolve(JSON.parse(text));
          } catch (err) {
            reject(new Error(`GET ${apiPath} body parse: ${err.message}`));
          }
        });
      },
    );
    req.on("error", reject);
    req.end();
  });
}

// defaultAgentRuntimeLabel reads the workspace label off the named
// (namespaced) AgentRuntime. Returns "" when the label is absent.
async function defaultAgentRuntimeLabel(namespace, name) {
  const path = `/apis/omnia.altairalabs.ai/v1alpha1/namespaces/${encodeURIComponent(namespace)}/agentruntimes/${encodeURIComponent(name)}`;
  const obj = await k8sGetJSON(path);
  return (obj && obj.metadata && obj.metadata.labels && obj.metadata.labels[WORKSPACE_LABEL]) || "";
}

// defaultNamespaceLabel reads the workspace label off the Namespace object.
// Returns "" when the label is absent.
async function defaultNamespaceLabel(namespace) {
  const path = `/api/v1/namespaces/${encodeURIComponent(namespace)}`;
  const obj = await k8sGetJSON(path);
  return (obj && obj.metadata && obj.metadata.labels && obj.metadata.labels[WORKSPACE_LABEL]) || "";
}
/* c8 ignore stop */

module.exports = {
  resolveWorkspaceName,
  WORKSPACE_LABEL,
  defaultAgentRuntimeLabel,
  defaultNamespaceLabel,
};
