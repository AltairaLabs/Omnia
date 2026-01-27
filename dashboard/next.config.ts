import type { NextConfig } from "next";

// Browser stub paths for Node.js built-in modules
const EMPTY_STUB = "./src/lib/stubs/empty.js";

const nextConfig: NextConfig = {
  // Note: We don't use standalone output because we have a custom server.js
  // that handles WebSocket proxying for agent connections.
  // The custom server wraps Next.js and intercepts WebSocket upgrades.

  // Note: Grafana proxy is handled by /app/grafana/[...path]/route.ts
  // This allows us to add auth headers when proxying to Grafana

  // Enable source maps in production for E2E code coverage collection.
  // This allows monocart-reporter to map V8 coverage back to original source files.
  // Source maps are only served when explicitly requested, so no security impact.
  productionBrowserSourceMaps: true,

  // Exclude monaco-languageclient and related packages from SSR bundling
  // These packages have Node.js-specific code that doesn't work in SSR context
  serverExternalPackages: [
    "monaco-languageclient",
    "vscode-languageclient",
    "vscode-ws-jsonrpc",
    "vscode",
  ],

  // Turbopack configuration for handling Node.js modules
  turbopack: {
    // Exclude problematic packages from Turbopack bundling
    // These will be loaded via script tags or handled differently
    resolveExtensions: [".tsx", ".ts", ".jsx", ".js", ".mjs", ".json"],
    resolveAlias: {
      // Stub Node.js built-in modules for browser builds
      // These are required by monaco-languageclient/vscode packages
      // Use proper browserify packages where available, stubs for others
      path: { browser: "path-browserify" },
      os: { browser: "os-browserify/browser" },
      buffer: { browser: "buffer" },
      process: { browser: "process/browser" },
      // These need custom stubs
      perf_hooks: { browser: "./src/lib/stubs/perf-hooks.js" },
      fs: { browser: "./src/lib/stubs/fs.js" },
      // These can be empty stubs
      crypto: { browser: EMPTY_STUB },
      stream: { browser: EMPTY_STUB },
      util: { browser: EMPTY_STUB },
      assert: { browser: EMPTY_STUB },
      http: { browser: EMPTY_STUB },
      https: { browser: EMPTY_STUB },
      zlib: { browser: EMPTY_STUB },
      net: { browser: EMPTY_STUB },
      tls: { browser: EMPTY_STUB },
      child_process: { browser: EMPTY_STUB },
    },
  },

  // Webpack configuration for production builds (fallback)
  webpack: (config, { isServer }) => {
    if (!isServer) {
      config.resolve = config.resolve || {};
      config.resolve.fallback = {
        ...config.resolve.fallback,
        perf_hooks: false,
        fs: false,
        path: false,
        os: false,
        crypto: false,
        stream: false,
        util: false,
        assert: false,
        http: false,
        https: false,
        zlib: false,
        net: false,
        tls: false,
        child_process: false,
      };
    }
    return config;
  },
};

export default nextConfig;
