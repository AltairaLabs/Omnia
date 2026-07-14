---
title: "White-label the dashboard"
description: "Re-brand the Omnia dashboard for an SI partner: colors, logos, fonts, and copy via the design-token contract"
sidebar:
  order: 12
---

White-labeling re-skins the entire dashboard from a single, deploy-time brand
configuration. Every UI surface reads **design tokens** (CSS variables), so
re-pointing those tokens re-themes the whole app — no component changes.

White-labeling is an **Enterprise feature**, gated by the `whiteLabel` license
entitlement. Without it, the branding config is ignored and the dashboard falls
back to the Omnia defaults (fail-closed) — see [Install with a License](/how-to/operations/install-license/).

## What re-themes

One brand config controls all of the following. Anything not set keeps the Omnia
default.

| Aspect | Tokens / fields | Notes |
|--------|-----------------|-------|
| Product name | `productName` | Sidebar title, page `<title>`, login/upgrade copy |
| Logo | `logo.light`, `logo.dark` | Rendered in the sidebar; dark logo on the dark sidebar |
| Favicon | `favicon` | Browser tab icon (server-rendered metadata) |
| Brand color | `--primary` | Primary buttons, links, focus rings, active nav |
| Accent | `--accent` | Secondary accents |
| Sidebar | `--sidebar` | Sidebar surface |
| Surfaces | `background`, `card`, `foreground`, `muted`, `mutedForeground`, `border` | The page/card canvas + text + borders. Curated (not arbitrary CSS). Provide dark-mode values via `colorsDark` — see below |
| Status | `--success` `--warning` `--info` `--destructive` | Semantic status (success/warning/info/error) |
| Categorical | `--category-1` … `--category-8` | Entity/node/memory-category colors (graphs, badges) |
| Chart series | `--chart-1` … `--chart-5` | Time-series / data-series colors |
| Fonts | `fonts.family`, `fonts.url` | Interface font family + the stylesheet that loads it |
| Copy | `copy.loginTagline`, `copy.signupTagline` | Auth screens |
| Links | `links.docsBaseUrl` `links.support` `links.sales` `links.upgradeUrl` | Upgrade banners, docs links, sales contact |
| Escape hatch | `customCss` | Raw CSS appended to `:root` — token overrides only |

**Status semantics stay fixed by design.** Success is green, error is red, etc.,
regardless of brand — those are status tokens, not brand tokens, so they remain
legible and meaningful across brands.

## Configure it (Helm / env)

Set branding under `dashboard.branding` in your Helm values. The operator emits
it to the dashboard as `NEXT_PUBLIC_BRAND_*` environment variables **only when
`enterprise.enabled=true` and a `whiteLabel`-entitled license is present.**

```yaml
enterprise:
  enabled: true

dashboard:
  branding:
    productName: "Acme Cloud"          # required — the entitlement gate
    logo:
      light: "/brand/acme-light.svg"
      dark:  "/brand/acme-dark.svg"
    favicon: "/brand/acme-favicon.svg"
    colors:
      primary: "#EA580C"
      accent:  "#DC2626"
      sidebar: "#7C2D12"
    fonts:
      family: "Poppins"
      url:    "https://fonts.googleapis.com/css2?family=Poppins:wght@400;600;700&display=swap"
    links:
      docsBaseUrl: "https://docs.acme.example"
      support:     "https://acme.example/support"
      sales:       "sales@acme.example"
      upgradeUrl:  "https://acme.example/enterprise"
    copy:
      loginTagline:  "Sign in to Acme Cloud"
      signupTagline: "Sign up to get started with Acme Cloud"
```

### Environment variable reference

| Env var | Field |
|---------|-------|
| `NEXT_PUBLIC_BRAND_PRODUCT_NAME` | `productName` (unset ⇒ Omnia default, entire brand ignored) |
| `NEXT_PUBLIC_BRAND_LOGO_LIGHT` / `_DARK` | `logo.light` / `logo.dark` |
| `NEXT_PUBLIC_BRAND_FAVICON` | `favicon` |
| `NEXT_PUBLIC_BRAND_COLOR_PRIMARY` / `_ACCENT` / `_SIDEBAR` | `colors.primary` / `.accent` / `.sidebar` |
| `NEXT_PUBLIC_BRAND_FONT_FAMILY` / `_URL` | `fonts.family` / `fonts.url` |
| `NEXT_PUBLIC_BRAND_DOCS_URL` / `_SUPPORT` / `_SALES` / `_UPGRADE_URL` | `links.*` |
| `NEXT_PUBLIC_BRAND_LOGIN_TAGLINE` / `_SIGNUP_TAGLINE` | `copy.*` |
| `NEXT_PUBLIC_BRAND_CUSTOM_CSS` | `customCss` |

The env/Helm surface exposes **`primary`, `accent`, and `sidebar`** directly.
To tune the full palette (`--category-*`, `--chart-*`, status), use `customCss`:

```yaml
dashboard:
  branding:
    customCss: >-
      --category-1: #EA580C; --category-2: #DC2626; --chart-1: #EA580C;
```

`customCss` is appended to `:root` only — it overrides design tokens, never
arbitrary selectors. Targeting internal component classes is unsupported and may
break on upgrade.

### Dark-mode surfaces

A single override can't be both a light and a dark surface, so surface tokens
that must differ by theme go in a separate `colorsDark` block. Light/shared
values live in `colors` (`:root`); `colorsDark` is injected under `.dark` and
wins in dark mode. This lets a brand ship, say, a warm-charcoal dark canvas
instead of the default navy:

```yaml
dashboard:
  branding:
    colors:      # light + shared (accents apply in both modes)
      primary: "#EA580C"
      background: "#FFF7ED"
      card: "#FFFFFF"
      foreground: "#431407"
    colorsDark:  # dark-mode surface tones
      background: "#1A120B"
      card: "#251A11"
      foreground: "#FFF7ED"
      mutedForeground: "#FDBA74"
      border: "rgba(253, 186, 116, 0.14)"
```

Accents (primary/category/chart/status) generally read fine in both modes, so
they only need to be set once in `colors`.

## Logos & favicon

- SVG is preferred (crisp at any density). The sidebar renders the **dark**
  logo variant on its dark surface.
- `logo.light`, `logo.dark`, and `favicon` are **URLs the browser fetches** —
  not files the chart uploads. Each value can be either an absolute URL on an
  allowed host, or a path served by the dashboard origin (e.g. `/brand/logo.svg`).

### Serve logos from the dashboard (mount via Helm)

If you don't want to host the assets on an external CDN, mount them into the
dashboard container and reference them by path. The dashboard serves everything
under `/app/public/` at the site root, so a file at `/app/public/brand/logo.svg`
is served at `/brand/logo.svg` — exactly what `logo.light` points at above.

1. **Put the assets in a ConfigMap** (or Secret) in the release namespace:

   ```bash
   kubectl create configmap dashboard-logos -n omnia-system \
     --from-file=logo.svg=./acme-light.svg \
     --from-file=logo-dark.svg=./acme-dark.svg \
     --from-file=favicon.svg=./acme-favicon.svg
   ```

2. **Mount each file** into `/app/public/brand/` and point branding at those
   paths, using the dashboard's `extraVolumes` / `extraVolumeMounts` hooks:

   ```yaml
   enterprise:
     enabled: true

   dashboard:
     branding:
       productName: "Acme Cloud"        # required — the entitlement gate
       logo:
         light: "/brand/logo.svg"
         dark:  "/brand/logo-dark.svg"
       favicon: "/brand/favicon.svg"

     extraVolumes:
       - name: brand-logos
         configMap:
           name: dashboard-logos

     extraVolumeMounts:
       - name: brand-logos
         mountPath: /app/public/brand/logo.svg
         subPath: logo.svg
         readOnly: true
       - name: brand-logos
         mountPath: /app/public/brand/logo-dark.svg
         subPath: logo-dark.svg
         readOnly: true
       - name: brand-logos
         mountPath: /app/public/brand/favicon.svg
         subPath: favicon.svg
         readOnly: true
   ```

   `podOverrides.extraVolumes` / `podOverrides.extraVolumeMounts` work too and
   append to the same lists — use whichever your values file already favors.

:::note
Mount **per file with `subPath`**, targeting a subdirectory like `/brand/`.
Mounting a volume over `/app/public` itself would hide the bundled assets
(default favicon, presets). A `subPath` ConfigMap mount does **not** hot-reload —
after changing a logo, roll the dashboard (`kubectl rollout restart`) or rename
the ConfigMap so the new content is picked up.
:::

The same license gate applies: filesystem-mounted logos still only render when
`enterprise.enabled=true` **and** the active license carries the `whiteLabel`
entitlement. Mounting a file over the built-in `/logo.svg` to bypass that gate is
a license violation, not a supported configuration — keep custom assets on a
distinct path (`/brand/…`) behind the branding fields above.

## Fonts

`fonts.family` re-points the interface font; `fonts.url` loads the stylesheet
that provides it.

- `fonts.url` must be a **CSS stylesheet URL** (e.g. a Google Fonts `href`), not
  a raw font file.
- The brand's font host must be allowed by the dashboard **Content-Security-Policy**.
  The default CSP already allows Google Fonts (`fonts.googleapis.com`,
  `fonts.gstatic.com`); for another host, extend the CSP via `OMNIA_CSP_POLICY`.
- If the font fails to load, the interface falls back to the bundled sans stack.

:::note
The font model is planned to move to a **governed sans/mono pair** (a forced
choice from a curated set, rather than a free font field) as part of the
component redesign, so machine data and interface type stay coherent. The
free-field form documented here is the current behavior.
:::

## Preview locally (dev / demo)

You don't need a cluster to develop or check a brand. In dev or demo mode the
dashboard exposes:

- A **brand preset switcher** (palette icon, next to the theme toggle) to flip
  between built-in presets (`omnia`, `acme`, `nebula`) live.
- A **`/dev/theme`** kitchen-sink route that renders every token-driven primitive
  (status badges, buttons, cards, categorical + chart swatches, a graph sample)
  so a brand switch is visible at a glance.

Pin a preset server-side in demo mode with `NEXT_PUBLIC_BRAND_PRESET=acme`.

## Guardrail

Dashboard components must use **design tokens**, never hardcoded Tailwind
palette classes (`bg-blue-600`, `text-green-500`, …) — otherwise those elements
ignore the brand. The `hack/check-no-hardcoded-palette.sh` pre-commit guard
enforces this: a new palette class in a non-allowlisted file fails the commit.
The allowlist (`hack/no-hardcoded-palette.allowlist`) covers intentional,
non-themeable identity (third-party vendor/framework brand colors).

## Semantic aliases (for component authors)

On top of the brand tokens above sits a thin **semantic-alias layer** — role
named variables that components read instead of the raw tokens:

| Role | Alias | Resolves to |
|------|-------|-------------|
| Page / surfaces | `--bg-app`, `--surface-1`, `--surface-2`, `--surface-code` | `--background`, `--card`, `--muted`, (code surface) |
| Borders | `--border-default`, `--border-strong` | `--border` |
| Text | `--text-heading`, `--text-body`, `--text-muted`, `--text-faint`, `--text-link` | `--foreground`, `--muted-foreground`, `--primary` |
| Accents | `--accent-primary` (gold), `--accent-inter`, `--accent-node` | (gold), `--primary`, `--category-1` |
| Status | `--status-healthy`, `--status-pending`, `--status-error` | `--success`, `--warning`, `--destructive` |
| Node kinds | `--node-prompt`, `--node-tool`, `--node-agent`, `--node-output` | `--category-2`, `--category-6`, `--category-1`, `--accent-primary` |

Because each alias points at a brand token, a white-label override flows through
to every alias that references it. Author components against the aliases; theme
by overriding the brand tokens.
