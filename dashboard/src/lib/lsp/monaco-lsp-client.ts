/**
 * Monaco LSP Client module.
 *
 * Loaded only on the client (dynamic import() from the "use client"
 * lsp-yaml-editor component; never evaluated during SSR — next.config.ts lists
 * these packages in serverExternalPackages). The npm modules are imported with
 * dynamic import() so they land in their own client chunk, using **static
 * string-literal specifiers with NO `webpackIgnore`** so both bundlers resolve
 * and bundle them: `next dev --turbopack` (dev) and `next build --webpack`
 * (prod). An earlier `webpackIgnore: true` form left raw runtime imports of
 * bare specifiers the browser can't resolve, which broke the prod build only
 * (Turbopack ignores webpackIgnore). The Node built-ins these packages reach
 * for (fs/path/os/perf_hooks/…) are stubbed in next.config.ts.
 */

/* eslint-disable sonarjs/no-duplicate-string -- type-only `import("…")` specifiers must be string literals, so package names recur in the type annotations below */

// Track if services have been initialized (singleton)
let servicesInitialized = false;
let servicesInitializing: Promise<void> | null = null;

// Lazy-loaded modules
let MonacoLanguageClient: typeof import("monaco-languageclient").MonacoLanguageClient;
let initServices: typeof import("monaco-languageclient/vscode/services").initServices;
let toSocket: typeof import("vscode-ws-jsonrpc").toSocket;
let WebSocketMessageReader: typeof import("vscode-ws-jsonrpc").WebSocketMessageReader;
let WebSocketMessageWriter: typeof import("vscode-ws-jsonrpc").WebSocketMessageWriter;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let ErrorAction: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let CloseAction: any;

/**
 * Dynamically load all required modules.
 */
async function loadModules(): Promise<void> {
  if (MonacoLanguageClient) return; // Already loaded

  const [mlcModule, servicesModule, wsModule, clientModule] = await Promise.all([
    import("monaco-languageclient"),
    import("monaco-languageclient/vscode/services"),
    import("vscode-ws-jsonrpc"),
    import("vscode-languageclient/lib/common/client.js"),
  ]);

  MonacoLanguageClient = mlcModule.MonacoLanguageClient;
  initServices = servicesModule.initServices;
  toSocket = wsModule.toSocket;
  WebSocketMessageReader = wsModule.WebSocketMessageReader;
  WebSocketMessageWriter = wsModule.WebSocketMessageWriter;
  ErrorAction = clientModule.ErrorAction;
  CloseAction = clientModule.CloseAction;
}

/**
 * Initialize monaco-languageclient services (must be called before creating a MonacoLanguageClient).
 * This is a singleton - it only runs once.
 */
export async function ensureServicesInitialized(): Promise<void> {
  if (servicesInitialized) {
    return;
  }

  if (servicesInitializing) {
    return servicesInitializing;
  }

  servicesInitializing = (async () => {
    try {
      await loadModules();
      await initServices({
        serviceConfig: {
          debugLogging: false,
        },
      });
      servicesInitialized = true;
    } catch (error) {
      // The vscode services are a process-wide singleton. They can be set up by
      // a concurrent editor mount or a React double-invoke before we get here;
      // initServices then throws "Services are already initialized". That's a
      // success for our purposes — the services exist, so proceed.
      if (error instanceof Error && /already initialized/i.test(error.message)) {
        servicesInitialized = true;
      } else {
        console.error("Failed to initialize monaco-languageclient services:", error);
        throw error;
      }
    } finally {
      servicesInitializing = null;
    }
  })();

  return servicesInitializing;
}

/**
 * Create a MonacoLanguageClient connected via WebSocket.
 * Must call ensureServicesInitialized() first.
 */
export function createLanguageClient(webSocket: WebSocket): InstanceType<typeof import("monaco-languageclient").MonacoLanguageClient> {
  if (!MonacoLanguageClient) {
    throw new Error("Modules not loaded. Call ensureServicesInitialized() first.");
  }

  const socket = toSocket(webSocket);
  const reader = new WebSocketMessageReader(socket);
  const writer = new WebSocketMessageWriter(socket);

  return new MonacoLanguageClient({
    name: "PromptKit LSP Client",
    clientOptions: {
      // Match by language only (any URI scheme). @monaco-editor/react + the
      // @codingame Monaco create the model under a scheme that isn't
      // file/inmemory, and a scheme-pinned selector silently never matches, so
      // the client never sends textDocument/didOpen and no features appear.
      documentSelector: [{ language: "yaml" }],
      errorHandler: {
        error: () => ({ action: ErrorAction.Continue }),
        closed: () => ({ action: CloseAction.Restart }),
      },
    },
    connectionProvider: {
      get: () => Promise.resolve({ reader, writer }),
    },
  });
}

// Type for the MonacoLanguageClient - defined here to avoid static import
export interface MonacoLanguageClient {
  start(): void;
  stop(): Promise<void>;
}
