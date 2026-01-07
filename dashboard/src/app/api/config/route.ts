/**
 * Runtime configuration endpoint.
 *
 * Returns configuration values that need to be read at runtime
 * rather than build time. This is necessary for Kubernetes deployments
 * where config is provided via ConfigMaps/environment variables.
 */

import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({
    demoMode: process.env.NEXT_PUBLIC_DEMO_MODE === "true",
    readOnlyMode: process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true",
    readOnlyMessage: process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || "This dashboard is in read-only mode",
  });
}
