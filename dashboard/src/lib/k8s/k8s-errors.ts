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

/**
 * Extract the Kubernetes Status `message` from a client error's response body
 * (a JSON string or object), falling back to the error's own message, then to
 * `fallback`. This is what turns an opaque ApiException ("HTTP-Code: 409 …
 * Unknown API Status Code!") into the real, user-actionable reason (e.g. a CRD
 * validation message or "… already exists").
 */
// messageFromBody pulls a non-empty `message` from a k8s error body that is
// either a JSON string or an already-parsed object. Returns null when absent.
function messageFromBody(body: unknown): string | null {
  if (typeof body === "string") {
    try {
      const parsed = JSON.parse(body) as { message?: unknown };
      return typeof parsed.message === "string" && parsed.message ? parsed.message : null;
    } catch {
      return null;
    }
  }
  if (body && typeof (body as { message?: unknown }).message === "string") {
    return (body as { message: string }).message || null;
  }
  return null;
}

export function extractStatusMessage(error: unknown, fallback: string): string {
  if (typeof error !== "object" || error === null) {
    return fallback;
  }
  const fromBody = messageFromBody((error as { body?: unknown }).body);
  if (fromBody) {
    return fromBody;
  }
  const msg = (error as { message?: unknown }).message;
  return typeof msg === "string" && msg ? msg : fallback;
}
