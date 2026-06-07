// Copies Monaco's `min/vs` assets into `public/monaco/vs` so the dashboard
// self-hosts the editor instead of loading it from the jsdelivr CDN. The CSP
// (script-src 'self') blocks the CDN, and a self-hosted copy also works in
// air-gapped / egress-restricted clusters.
//
// Runs on `postinstall` so the assets exist for both `next dev` and
// `next build`. The destination is git-ignored and regenerated from
// node_modules on every install.
import { existsSync, rmSync, cpSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const src = join(root, "node_modules", "monaco-editor", "min", "vs");
const dest = join(root, "public", "monaco", "vs");

if (!existsSync(src)) {
  // Don't fail the install — monaco-editor may not be installed yet in some
  // flows (e.g. partial installs). The editor simply won't render until it is.
  console.warn(`[copy-monaco] source not found, skipping: ${src}`);
  process.exit(0);
}

rmSync(dest, { recursive: true, force: true });
cpSync(src, dest, { recursive: true });
console.log(`[copy-monaco] copied ${src} -> ${dest}`);
