"use client";

import { loader } from "@monaco-editor/react";

// `monaco-editor` is aliased to @codingame/monaco-vscode-editor-api (see
// package.json) because monaco-languageclient v8 requires the editor and the
// language client to share ONE Monaco instance. We hand @monaco-editor/react
// that exact ESM instance instead of loading the AMD build from a path, so:
//   - the LSP editor's language client operates on the same Monaco the editor
//     renders (the v6/v7 self-hosted-AMD setup gave two separate instances and
//     the language client could never attach);
//   - Monaco is bundled by webpack (same-origin), so the tight CSP
//     (script-src 'self') is satisfied without the self-hosted /monaco/vs copy.
//
// The @codingame editor-api touches `window` at module load, so it MUST NOT be
// evaluated during SSR/prerender. We import it client-only; the chunk resolves
// during hydration, long before any lazy (ssr:false) Editor mounts, so the
// config is in place by the time @monaco-editor/react's loader initialises.
if (typeof window !== "undefined") {
  // Monaco needs a worker factory; without it it warns and runs worker code on
  // the main thread (UI jank). Point it at the webpack-bundled editor worker.
  // Language features here come from the LSP server over WebSocket, so the base
  // editor worker is sufficient for every label.
  (self as unknown as { MonacoEnvironment?: { getWorker: () => Worker } }).MonacoEnvironment = {
    getWorker: () =>
      new Worker(
        new URL("monaco-editor/esm/vs/editor/editor.worker.js", import.meta.url),
        { type: "module" },
      ),
  };

  import("monaco-editor")
    .then((monaco) => {
      loader.config({ monaco });
    })
    .catch(() => {
      // If Monaco fails to load, @monaco-editor/react falls back to its default
      // loader; the editor degrades rather than crashing the app.
    });
}
