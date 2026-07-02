/**
 * White-label branding configuration.
 *
 * One schema resolved from three sources: Helm/ConfigMap env (prod), the
 * runtime /api/config payload (client), and preset JSON files (local dev).
 * Colors are a curated allowlist mapped onto design tokens — never arbitrary
 * CSS variable injection. See css-vars.ts for the mapping.
 */

export interface BrandColorTokens {
  primary?: string;
  accent?: string;
  sidebar?: string;
  // Series palette (charts / time-series)
  chart1?: string;
  chart2?: string;
  chart3?: string;
  chart4?: string;
  chart5?: string;
  // Categorical palette (entity categories: memory categories, node types)
  category1?: string;
  category2?: string;
  category3?: string;
  category4?: string;
  category5?: string;
  category6?: string;
  category7?: string;
  category8?: string;
  success?: string;
  warning?: string;
  info?: string;
}

export interface BrandConfig {
  productName: string;
  logo: { light: string; dark: string };
  favicon: string;
  colors?: BrandColorTokens;
  fonts?: { family?: string; url?: string };
  links?: {
    docsBaseUrl?: string;
    support?: string;
    sales?: string;
    upgradeUrl?: string;
  };
  copy?: { loginTagline?: string; signupTagline?: string };
  /**
   * Optional escape hatch: raw CSS appended to :root only. Documented as
   * "override design tokens; targeting internal selectors is unsupported and
   * may break on upgrade."
   */
  customCss?: string;
}

/** The built-in Omnia brand — the fail-closed default when unentitled/unset. */
export const OMNIA_BRAND: BrandConfig = {
  productName: "Omnia",
  logo: { light: "/logo.svg", dark: "/logo-dark.svg" },
  favicon: "/favicon.svg",
  links: {
    docsBaseUrl: "https://omnia.altairalabs.ai",
    upgradeUrl: "https://altairalabs.ai/enterprise",
    sales: "sales@altairalabs.ai",
  },
};
