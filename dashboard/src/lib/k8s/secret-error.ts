/** Map a Kubernetes secrets error to an accurate, non-masking HTTP response. */
import { NextResponse } from "next/server";
import { extractStatusCode } from "./k8s-errors";

function messageOf(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) {
    const err = error as unknown as Record<string, unknown>;
    if (typeof err.body === "string") {
      try {
        const parsed = JSON.parse(err.body) as Record<string, unknown>;
        if (typeof parsed.message === "string" && parsed.message) {
          return parsed.message;
        }
      } catch {
        // Not JSON, fall through
      }
    } else if (err.body && typeof (err.body as Record<string, unknown>).message === "string") {
      return (err.body as Record<string, unknown>).message as string;
    }
    return error.message;
  }
  return fallback;
}

export function secretErrorResponse(error: unknown, fallback: string): NextResponse {
  const message = messageOf(error, fallback);
  if (message.includes("not a managed credential secret")) {
    return NextResponse.json({ error: message }, { status: 409 });
  }
  const code = extractStatusCode(error);
  if (code === 403) return NextResponse.json({ error: message }, { status: 403 });
  if (code === 404) return NextResponse.json({ error: message }, { status: 404 });
  return NextResponse.json({ error: message }, { status: 500 });
}
