/**
 * Security response headers applied to every dashboard response by the
 * Next.js middleware. The set below is the defence-in-depth baseline:
 *
 * - Strict-Transport-Security: forces HTTPS for two years; prevents
 *   downgrade / SSL-strip.
 * - Content-Security-Policy: limits script / style / connect sources to
 *   the same origin by default. Next.js needs `'unsafe-inline'` for its
 *   hydration scripts + runtime-injected styles until we wire nonces.
 *   Operators can override with `OMNIA_CSP_POLICY` when they have a
 *   tighter policy drafted for their deployment.
 * - X-Frame-Options / frame-ancestors: prevents clickjacking via framing.
 * - X-Content-Type-Options: blocks MIME sniffing on responses the server
 *   did not type.
 * - Referrer-Policy: limits referer leakage to cross-origin targets.
 * - Permissions-Policy: disables APIs the dashboard does not use
 *   (camera / microphone / geolocation).
 *
 * Notes:
 * - HSTS is safe to emit over plaintext (browsers ignore it on non-HTTPS
 *   responses), but is only meaningful when the edge terminates TLS.
 * - `x-powered-by: Next.js` is disabled via `next.config.ts`
 *   `poweredByHeader: false` — keeping the version-leak fix adjacent to
 *   the other security-header config.
 */

import type { NextResponse } from "next/server";

// Default Content-Security-Policy. Next.js inline scripts + runtime
// styles force `'unsafe-inline'` until we add per-request nonces. WebSocket
// agent connections use `ws:`/`wss:` same-origin.
const DEFAULT_CSP = [
  "default-src 'self'",
  "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https:",
  "font-src 'self' data:",
  "connect-src 'self' ws: wss:",
  "media-src 'self' blob: data:",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
].join("; ");

interface SecurityHeaderEnv {
  cspPolicy?: string;
}

function envOverride(): SecurityHeaderEnv {
  return {
    cspPolicy: process.env.OMNIA_CSP_POLICY,
  };
}

export const SECURITY_HEADER_NAMES = [
  "Strict-Transport-Security",
  "Content-Security-Policy",
  "X-Frame-Options",
  "X-Content-Type-Options",
  "Referrer-Policy",
  "Permissions-Policy",
] as const;

/**
 * Apply the security-header baseline to a response. Intended for the
 * Next.js middleware; middleware runs on every matched path, so the
 * headers ride every response.
 */
export function applySecurityHeaders(response: NextResponse): NextResponse {
  const env = envOverride();
  const headers = response.headers;
  headers.set(
    "Strict-Transport-Security",
    "max-age=63072000; includeSubDomains; preload",
  );
  headers.set("Content-Security-Policy", env.cspPolicy ?? DEFAULT_CSP);
  headers.set("X-Frame-Options", "DENY");
  headers.set("X-Content-Type-Options", "nosniff");
  headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  headers.set(
    "Permissions-Policy",
    "camera=(), microphone=(), geolocation=(), payment=()",
  );
  return response;
}
