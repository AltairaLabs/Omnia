/**
 * Maps a content-API client error to a Next.js JSON response, preserving the
 * operator's HTTP status (404/400/403/409/413) instead of collapsing
 * everything to 500. Shared by every workspace route that calls
 * content-api-service.
 */

import { NextResponse } from "next/server";

import { ContentApiError } from "./content-api-service";

const STATUS_LABELS: Record<number, string> = {
  400: "Bad Request",
  403: "Forbidden",
  404: "Not Found",
  409: "Conflict",
  413: "Payload Too Large",
};

/** Build a JSON error response from a thrown content-API error. */
export function contentErrorResponse(error: unknown, fallbackMessage: string): NextResponse {
  const status = error instanceof ContentApiError ? error.status : 500;
  const label = STATUS_LABELS[status] ?? "Internal Server Error";
  const message = error instanceof Error ? error.message : fallbackMessage;
  return NextResponse.json({ error: label, message }, { status });
}
