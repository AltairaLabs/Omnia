// Copies Monaco's `min/vs` assets into `public/monaco/vs` so the dashboard
// self-hosts the editor instead of loading it from the jsdelivr CDN. The CSP
// (script-src 'self') blocks the CDN, and a self-hosted copy also works in
// air-gapped / egress-restricted clusters.
//
// Runs via the `prebuild` / `predev` npm hooks (NOT postinstall — that fires
// during the Docker deps-only layer before this script is copied into the
// image, breaking `npm ci`). By the time build/dev runs, the source tree and
// monaco-editor devDependency are both present. The destination is git-ignored
// and regenerated from node_modules.
import { existsSync, rmSync, cpSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const src = join(root, "node_modules", "monaco-editor", "min", "vs");
const dest = join(root, "public", "monaco", "vs");

if (!existsSync(src)) {
  // Fail loudly: build/dev runs with devDependencies installed, so a missing
  // source means a broken setup. Silently skipping would ship a dashboard
  // where the editor never loads — the exact failure this script prevents.
  console.error(`[copy-monaco] monaco-editor assets not found: ${src}\nRun \`npm install\` first (monaco-editor is a devDependency).`);
  process.exit(1);
}

rmSync(dest, { recursive: true, force: true });
cpSync(src, dest, { recursive: true });
console.log(`[copy-monaco] copied ${src} -> ${dest}`);
