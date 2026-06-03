/**
 * Shared helpers for interpreting Kubernetes client errors.
 *
 * @kubernetes/client-node surfaces the HTTP status in several shapes depending
 * on the call/version — a `statusCode` property, a nested `response.statusCode`,
 * an `HTTP-Code: <n>` line in the Error message, or a JSON/object `body.code`.
 * Centralising the extraction here keeps every status check (404/403/401)
 * consistent, so e.g. token refresh on 401 fires regardless of which shape the
 * client threw (issue #1194).
 */

/**
 * Extract the HTTP status code from the various Kubernetes client error formats.
 * Returns null when no status can be determined.
 */
export function extractStatusCode(error: unknown): number | null {
  if (typeof error !== "object" || error === null) {
    return null;
  }

  const err = error as Record<string, unknown>;

  // Direct statusCode property
  if (typeof err.statusCode === "number") {
    return err.statusCode;
  }

  // Response statusCode
  if (err.response && typeof (err.response as Record<string, unknown>).statusCode === "number") {
    return (err.response as Record<string, unknown>).statusCode as number;
  }

  // Kubernetes client error format: "HTTP-Code: 404" in message
  if (typeof err.message === "string") {
    const match = /HTTP-Code:\s*(\d+)/.exec(err.message);
    if (match) {
      return Number.parseInt(match[1], 10);
    }
  }

  // Kubernetes API response body
  if (typeof err.body === "string") {
    try {
      const parsed = JSON.parse(err.body) as Record<string, unknown>;
      if (typeof parsed.code === "number") {
        return parsed.code;
      }
    } catch {
      // Not JSON, ignore
    }
  } else if (err.body && typeof (err.body as Record<string, unknown>).code === "number") {
    return (err.body as Record<string, unknown>).code as number;
  }

  return null;
}

/** True when the error is an authentication failure (HTTP 401). */
export function isAuthError(error: unknown): boolean {
  return extractStatusCode(error) === 401;
}
