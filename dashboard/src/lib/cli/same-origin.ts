/** Same-origin (CSRF) check for state-changing CLI routes. */
import type { NextRequest } from "next/server";

/**
 * True when the request has no Origin (native navigation / non-browser) or its
 * Origin host matches the request host (x-forwarded-host preferred, behind a
 * proxy the Host header is the internal name).
 */
export function isSameOrigin(request: NextRequest): boolean {
  const origin = request.headers.get("origin");
  if (!origin) return true;
  let originHost: string;
  try {
    originHost = new URL(origin).host;
  } catch {
    return false;
  }
  const host = request.headers.get("x-forwarded-host") || request.headers.get("host") || "";
  return originHost === host;
}
