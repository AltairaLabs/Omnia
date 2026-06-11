/**
 * Shared helpers for dashboard → operator API communication.
 *
 * The operator's tool-related APIs authenticate callers via TokenReview.
 * This module provides the base URL and the SA token reader so that
 * multiple proxy routes can share the same auth logic without duplicating it.
 */

import { readFile } from "node:fs/promises";

const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";

export const OPERATOR_TOOL_TEST_URL =
  process.env.OPERATOR_TOOL_TEST_URL ||
  `http://omnia-operator.omnia-system.${SERVICE_DOMAIN}:8083`;

// Projected SA tokens rotate in place, so read fresh per request rather than
// caching. Falls back to an explicit token env or none (local dev).
const SA_TOKEN_PATH =
  process.env.SA_TOKEN_PATH ||
  "/var/run/secrets/kubernetes.io/serviceaccount/token";

/**
 * Returns the bearer token the dashboard should forward to the operator's
 * internal API endpoints (which authenticate via Kubernetes TokenReview).
 *
 * Resolution order:
 * 1. `OPERATOR_TOOL_TEST_TOKEN` env var (testing / explicit override)
 * 2. Projected ServiceAccount token file at `SA_TOKEN_PATH`
 * 3. null — running outside a cluster (local dev), omit the header
 */
export async function operatorAuthToken(): Promise<string | null> {
  if (process.env.OPERATOR_TOOL_TEST_TOKEN) {
    return process.env.OPERATOR_TOOL_TEST_TOKEN;
  }
  try {
    return (await readFile(SA_TOKEN_PATH, "utf-8")).trim();
  } catch {
    return null; // not running in-cluster (local dev) — send no auth
  }
}
