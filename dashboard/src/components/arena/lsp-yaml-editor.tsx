"use client";

import { useCallback, useEffect, useRef, useState, useMemo } from "react";
import Editor, { type OnMount, type BeforeMount } from "@monaco-editor/react";
import type { editor, IDisposable } from "monaco-editor";
import { cn } from "@/lib/utils";
import { Loader2, AlertTriangle, CheckCircle, Circle } from "lucide-react";
import type { FileType } from "@/types/arena-project";
import { getRuntimeConfig } from "@/lib/config";
import { yamlMonarchLanguage } from "@/lib/lsp/yaml-monarch";

// Note: MonacoLanguageClient is dynamically imported via @/lib/lsp/monaco-lsp-client
// to avoid SSR issues with Node.js-specific modules in the vscode package
// Type defined inline to avoid any import that might trigger Turbopack analysis
interface MonacoLanguageClient {
  start(): void;
  stop(): Promise<void>;
}

interface LspYamlEditorProps {
  readonly value: string;
  readonly onChange?: (value: string) => void;
  readonly onSave?: () => void;
  readonly readOnly?: boolean;
  readonly language?: string;
  readonly fileType?: FileType;
  readonly className?: string;
  readonly loading?: boolean;
  /** Workspace name for LSP context */
  readonly workspace?: string;
  /** Project ID for LSP context */
  readonly projectId?: string;
  /** File path within the project */
  readonly filePath?: string;
}

interface DiagnosticInfo {
  count: number;
  severity: "error" | "warning" | "info" | "hint";
}

// How many times to retry a dropped LSP connection before giving up silently.
const MAX_RECONNECT_ATTEMPTS = 3;

/**
 * Get Monaco language from file type
 */
function getLanguage(fileType: FileType | undefined): string {
  switch (fileType) {
    case "arena":
    case "prompt":
    case "provider":
    case "scenario":
    case "tool":
    case "persona":
    case "yaml":
      return "yaml";
    case "json":
      return "json";
    case "markdown":
      return "markdown";
    default:
      return "plaintext";
  }
}

/**
 * Create WebSocket connection to LSP server
 */
async function createLspConnection(
  workspace: string,
  projectId: string
): Promise<WebSocket> {
  // Get the LSP URL from runtime config or construct from current origin
  const config = await getRuntimeConfig();

  // Determine WebSocket URL
  // The WebSocket proxy runs on a separate port (wsProxyUrl)
  // We connect to /api/lsp on that proxy server
  let wsUrl: string;

  if (globalThis.location === undefined) {
    throw new TypeError("Cannot create WebSocket connection outside browser");
  }

  // Check if we have a configured WebSocket proxy URL
  if (config.wsProxyUrl) {
    // wsProxyUrl is typically like "ws://host:3002" or with a path
    // We replace any existing path with /api/lsp
    const url = new URL(config.wsProxyUrl);
    url.pathname = "/api/lsp";
    url.searchParams.set("workspace", workspace);
    url.searchParams.set("project", projectId);
    wsUrl = url.toString();
  } else {
    // No proxy URL configured: use a relative URL on the *page's* host so the
    // connection rides the same 443 the dashboard is served on, and the gateway
    // path-routes /api/lsp to the WS proxy (:3002). Never hardcode the proxy
    // port — it is not exposed on the public gateway LoadBalancer, so dialing
    // it directly fails behind an ingress/gateway. Mirrors the dev-console
    // (use-dev-console.ts). See Omnia#1243.
    const protocol = globalThis.location.protocol === "https:" ? "wss:" : "ws:";
    const host = globalThis.location.host;
    wsUrl = `${protocol}//${host}/api/lsp?workspace=${encodeURIComponent(workspace)}&project=${encodeURIComponent(projectId)}`;
  }

  return new WebSocket(wsUrl);
}


/**
 * Monaco-based YAML editor with LSP support for real-time validation,
 * autocomplete, hover, and go-to-definition.
 */
export function LspYamlEditor({
  value,
  onChange,
  onSave,
  readOnly = false,
  language,
  fileType,
  className,
  loading = false,
  workspace,
  projectId,
  filePath,
}: LspYamlEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<typeof import("monaco-editor") | null>(null);
  const clientRef = useRef<MonacoLanguageClient | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const disposablesRef = useRef<IDisposable[]>([]);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Bounded reconnect: the LSP is an optional enhancement, so when it's
  // unavailable we degrade silently to the basic editor rather than retry
  // forever. Reset to 0 on a successful connect.
  const reconnectAttemptsRef = useRef(0);

  const [diagnostics, setDiagnostics] = useState<DiagnosticInfo | null>(null);
  const [reconnectTrigger, setReconnectTrigger] = useState(0);
  const [monacoReady, setMonacoReady] = useState(false);
  const [servicesReady, setServicesReady] = useState(false);

  const monacoLanguage = language || getLanguage(fileType);
  const isYaml = monacoLanguage === "yaml";
  const canUseLsp = isYaml && workspace && projectId;
  // Extract values for use in async functions where TypeScript narrowing doesn't persist
  const wsWorkspace = workspace ?? "";
  const wsProjectId = projectId ?? "";

  // Initialize the vscode services BEFORE the editor is created. The @codingame
  // editor boots a minimal service set on creation; if that wins the race, our
  // full initServices (which wires localExtensionHost) is skipped and the
  // language client fails with "Default api is not ready". Gating the editor on
  // this makes our services the first to touch the singleton.
  useEffect(() => {
    if (!canUseLsp) {
      setServicesReady(true);
      return;
    }
    let cancelled = false;
    import("@/lib/lsp/monaco-lsp-client")
      .then(({ ensureServicesInitialized }) => ensureServicesInitialized())
      .then(() => {
        if (!cancelled) setServicesReady(true);
      })
      .catch(() => {
        // Services failed to init — still show the editor (it degrades to a
        // plain editor without LSP rather than never rendering).
        if (!cancelled) setServicesReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, [canUseLsp]);

  // Initialize and manage LSP connection
  useEffect(() => {
    if (!canUseLsp || !monacoReady) {
      return;
    }

    let mounted = true;

    // Retry a dropped connection a few times (the server may be momentarily
    // unavailable), then give up silently. The basic editor keeps working.
    const scheduleReconnect = () => {
      if (reconnectAttemptsRef.current >= MAX_RECONNECT_ATTEMPTS) {
        return;
      }
      reconnectAttemptsRef.current += 1;
      reconnectTimeoutRef.current = setTimeout(
        () => mounted && setReconnectTrigger(Date.now()),
        5000
      );
    };

    const connect = async () => {
      // Clean up existing connection
      if (clientRef.current) {
        try {
          await clientRef.current.stop();
        } catch {
          // Ignore errors during cleanup
        }
        clientRef.current = null;
      }
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }

      if (!mounted) return;

      try {
        // Dynamically import LSP client module to avoid SSR issues
        const { ensureServicesInitialized, createLanguageClient } = await import(
          "@/lib/lsp/monaco-lsp-client"
        );

        // Ensure VSCode services are initialized before creating the language client
        await ensureServicesInitialized();

        const webSocket = await createLspConnection(wsWorkspace, wsProjectId);
        if (!mounted) {
          webSocket.close();
          return;
        }
        socketRef.current = webSocket;

        webSocket.onopen = () => {
          if (!mounted) return;

          try {
            const client = createLanguageClient(webSocket);
            if (!mounted) return;
            clientRef.current = client;

            // Start the client; a clean connect resets the retry budget.
            client.start();
            reconnectAttemptsRef.current = 0;
          } catch (error) {
            // Language client failed to start — degrade silently to the
            // basic editor (no status badge). The socket is left to close.
            console.warn("LSP unavailable: language client init failed", error);
          }
        };

        // Connection-level errors surface via onclose; degrade is silent.
        webSocket.onerror = () => {};

        webSocket.onclose = () => {
          if (!mounted) return;
          clientRef.current = null;
          socketRef.current = null;
          scheduleReconnect();
        };
      } catch (error) {
        // Module load / service init failed (won't recover on retry) — stay on
        // the basic editor silently.
        console.warn("LSP unavailable: initialization failed", error);
      }
    };

    connect();

    return () => {
      mounted = false;
      // Cleanup on unmount
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (clientRef.current) {
        clientRef.current.stop();
        clientRef.current = null;
      }
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
      disposablesRef.current.forEach((d) => d.dispose());
      disposablesRef.current = [];
    };
  }, [canUseLsp, monacoReady, wsWorkspace, wsProjectId, reconnectTrigger]);

  // Track diagnostics from Monaco markers
  useEffect(() => {
    if (!monacoRef.current || !editorRef.current) {
      return;
    }

    const monaco = monacoRef.current;
    const model = editorRef.current.getModel();
    if (!model) {
      return;
    }

    const updateDiagnostics = () => {
      const markers = monaco.editor.getModelMarkers({ resource: model.uri });
      if (markers.length === 0) {
        setDiagnostics(null);
        return;
      }

      // Find highest severity (error > warning > info > hint)
      let severityLevel = 0; // 0=hint, 1=info, 2=warning, 3=error
      for (const marker of markers) {
        if (marker.severity === monaco.MarkerSeverity.Error) {
          severityLevel = 3;
          break; // Can't get worse than error
        } else if (marker.severity === monaco.MarkerSeverity.Warning && severityLevel < 2) {
          severityLevel = 2;
        } else if (marker.severity === monaco.MarkerSeverity.Info && severityLevel < 1) {
          severityLevel = 1;
        }
      }
      const severityMap: DiagnosticInfo["severity"][] = ["hint", "info", "warning", "error"];
      const severity = severityMap[severityLevel];

      setDiagnostics({ count: markers.length, severity });
    };

    // Listen for marker changes
    const disposable = monaco.editor.onDidChangeMarkers((uris) => {
      if (uris.some((uri) => uri.toString() === model.uri.toString())) {
        updateDiagnostics();
      }
    });

    disposablesRef.current.push(disposable);
    updateDiagnostics();

    return () => {
      disposable.dispose();
    };
  }, []);

  // Define custom theme before editor mounts to ensure semantic token colors are ready
  const handleEditorWillMount: BeforeMount = useCallback((monaco) => {
    // Configure semantic token highlighting for template variables
    // This makes {{variable}} patterns stand out in prompt templates
    monaco.editor.defineTheme("vs-dark-promptkit", {
      base: "vs-dark",
      inherit: true,
      rules: [
        // Template variable delimiters {{ and }}
        { token: "variable", foreground: "4EC9B0", fontStyle: "bold" },
        // Variable names inside template expressions
        { token: "parameter", foreground: "9CDCFE", fontStyle: "italic" },
      ],
      colors: {},
    });
  }, []);

  const handleEditorDidMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;

      // Configure editor keybindings
      // Ctrl/Cmd+S to save
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
        onSave?.();
      });

      // Configure YAML-specific settings
      if (isYaml) {
        // The @codingame editor-api (monaco-editor alias) ships a stripped
        // editor without the basic languages, so "yaml" isn't registered the
        // way it is in vanilla monaco-editor. Register it before configuring it,
        // otherwise setLanguageConfiguration throws "unknown language yaml".
        const yamlRegistered = monaco.languages
          .getLanguages()
          .some((lang) => lang.id === "yaml");
        if (!yamlRegistered) {
          monaco.languages.register({
            id: "yaml",
            extensions: [".yaml", ".yml"],
            aliases: ["YAML", "yaml"],
          });
          // Provide a tokenizer for syntax colouring (the @codingame editor
          // ships no YAML grammar of its own).
          monaco.languages.setMonarchTokensProvider("yaml", yamlMonarchLanguage);
        }
        monaco.languages.setLanguageConfiguration("yaml", {
          comments: { lineComment: "#" },
          brackets: [
            ["{", "}"],
            ["[", "]"],
          ],
          autoClosingPairs: [
            { open: "{", close: "}" },
            { open: "[", close: "]" },
            { open: '"', close: '"' },
            { open: "'", close: "'" },
          ],
          indentationRules: {
            increaseIndentPattern: /^.*:\s*$/,
            decreaseIndentPattern: /^\s*$/,
          },
        });
      }

      // Mark Monaco as ready to trigger LSP connection
      setMonacoReady(true);
    },
    [onSave, isYaml]
  );

  const handleChange = useCallback(
    (newValue: string | undefined) => {
      onChange?.(newValue || "");
    },
    [onChange]
  );

  // Notify LSP of document changes
  useEffect(() => {
    if (!clientRef.current || !filePath) {
      return;
    }

    // The MonacoLanguageClient handles document sync automatically
    // through Monaco's model change events when properly configured.
    // We just need to ensure the model URI matches what the LSP expects.
  }, [value, filePath]);

  // The LSP connection is deliberately invisible: it's an optional enhancement
  // that connects in the background, so there's no connection-status badge.
  // When it works, diagnostics appear below; when it can't, the basic editor
  // is unchanged. See #1293.

  // Diagnostics indicator
  const DiagnosticsIndicator = useMemo(() => {
    if (!diagnostics) {
      return (
        <div className="flex items-center gap-1">
          <CheckCircle className="h-3.5 w-3.5 text-green-500" />
          <span className="text-muted-foreground">No issues</span>
        </div>
      );
    }

    const severityConfig = {
      error: { icon: AlertTriangle, color: "text-red-500" },
      warning: { icon: AlertTriangle, color: "text-amber-500" },
      info: { icon: Circle, color: "text-blue-500" },
      hint: { icon: Circle, color: "text-muted-foreground" },
    };

    const config = severityConfig[diagnostics.severity];
    const Icon = config.icon;

    return (
      <div className="flex items-center gap-1">
        <Icon className={cn("h-3.5 w-3.5", config.color)} />
        <span className={config.color}>
          {diagnostics.count} {diagnostics.count === 1 ? "issue" : "issues"}
        </span>
      </div>
    );
  }, [diagnostics]);

  if (loading || !servicesReady) {
    return (
      <div className={cn("flex items-center justify-center h-full", className)}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-sm text-muted-foreground">Loading editor…</span>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col h-full", className)}>
      {/* Editor */}
      <div className="flex-1 min-h-0">
        <Editor
          height="100%"
          language={monacoLanguage}
          value={value}
          onChange={handleChange}
          beforeMount={handleEditorWillMount}
          onMount={handleEditorDidMount}
          theme={isYaml ? "vs-dark-promptkit" : "vs-dark"}
          options={{
            readOnly,
            minimap: { enabled: false },
            fontSize: 13,
            lineNumbers: "on",
            wordWrap: "on",
            scrollBeyondLastLine: false,
            automaticLayout: true,
            tabSize: 2,
            insertSpaces: true,
            formatOnPaste: true,
            formatOnType: true,
            folding: true,
            foldingStrategy: "indentation",
            scrollbar: {
              verticalScrollbarSize: 10,
              horizontalScrollbarSize: 10,
            },
            padding: { top: 8, bottom: 8 },
            // Enable features that work with LSP
            quickSuggestions: true,
            suggestOnTriggerCharacters: true,
            parameterHints: { enabled: true },
            // Enable semantic highlighting for template variables
            "semanticHighlighting.enabled": true,
          }}
          loading={
            <div className="flex items-center justify-center h-full">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          }
        />
      </div>

      {/* Status bar */}
      {isYaml && (
        <div className="flex items-center justify-between gap-4 px-3 py-1.5 border-t bg-muted/30 text-xs">
          <div className="flex items-center gap-4">
            {DiagnosticsIndicator}
          </div>
          <div className="flex items-center gap-4">
            {filePath && (
              <span className="text-muted-foreground truncate max-w-[200px]" title={filePath}>
                {filePath}
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * Empty state when no file is selected
 */
export function LspYamlEditorEmptyState() {
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground bg-muted/10">
      <div className="text-center">
        <p className="text-sm">No file selected</p>
        <p className="text-xs mt-1">Select a file from the tree to view and edit</p>
      </div>
    </div>
  );
}
