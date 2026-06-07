"use client";

import { loader } from "@monaco-editor/react";

// Self-host Monaco from /monaco/vs (copied from node_modules at install time by
// scripts/copy-monaco.mjs) instead of @monaco-editor/react's default jsdelivr
// CDN. The dashboard CSP (script-src 'self') blocks the CDN, so the editor's
// scripts/workers fail to load against it; serving them same-origin keeps the
// editor working under a tight CSP and in air-gapped clusters.
//
// Imported for its side effect from the client root (components/providers), so
// it runs once before any Editor mounts. loader.config must be called before
// the loader initialises. It only stores config (no DOM access), so it's safe
// during the SSR import of this "use client" module.
loader.config({ paths: { vs: "/monaco/vs" } });
