/**
 * Health check endpoint for Kubernetes probes.
 */

import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({
    status: "healthy",
    timestamp: new Date().toISOString(),
  });
}
