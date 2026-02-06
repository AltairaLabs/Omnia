/**
 * Monaco LSP Client module.
 * This module uses fully dynamic imports to avoid Turbopack static analysis issues.
 * All imports are done at runtime to prevent Node.js module resolution errors.
 */

/* eslint-disable sonarjs/no-duplicate-string -- Dynamic imports require string literals for TypeScript type inference */

// Module paths for dynamic imports
const MLC_MODULE = "monaco-languageclient";
const WS_JSONRPC_MODULE = "vscode-ws-jsonrpc";
const CLIENT_MODULE_PATH = "vscode-languageclient/lib/common/client.js";

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
    import(/* webpackIgnore: true */ MLC_MODULE),
    import("monaco-languageclient/vscode/services"),
    import(/* webpackIgnore: true */ WS_JSONRPC_MODULE),
    import(/* webpackIgnore: true */ CLIENT_MODULE_PATH),
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
      console.error("Failed to initialize monaco-languageclient services:", error);
      throw error;
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
      documentSelector: [
        { scheme: "file", language: "yaml" },
        { scheme: "inmemory", language: "yaml" },
      ],
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
