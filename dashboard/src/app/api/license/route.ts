/**
 * API route for license information.
 *
 * GET /api/license - Get current license information
 *
 * Returns the current license with features and limits. License resolution
 * (demo mode, operator fetch, open-core fallback) lives in
 * lib/license/resolve-server so it can be shared with /api/config.
 */

import { NextResponse } from "next/server";
import { getEffectiveLicense } from "@/lib/license/resolve-server";

export async function GET(): Promise<NextResponse> {
  return NextResponse.json(await getEffectiveLicense());
}
