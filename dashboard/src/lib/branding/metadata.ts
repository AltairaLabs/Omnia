import type { Metadata } from "next";
import { resolveBrandFromEnv } from "./resolve-server";

const DASHBOARD_DESCRIPTION =
  "AI Agent Operations Platform - Monitor and manage your Kubernetes-native AI agents";

/**
 * Build Next.js page metadata (title, description, favicon) from brand env.
 *
 * Read server-side by generateMetadata in the root layout, which cannot access
 * the client /api/config channel. Title/favicon are cosmetic and read directly
 * from env; the strongly license-gated branding is the in-app config delivered
 * via /api/config.
 */
export function buildBrandMetadata(env: Record<string, string | undefined>): Metadata {
  const brand = resolveBrandFromEnv(env);
  return {
    title: `${brand.productName} Dashboard`,
    description: DASHBOARD_DESCRIPTION,
    icons: { icon: brand.favicon },
  };
}
