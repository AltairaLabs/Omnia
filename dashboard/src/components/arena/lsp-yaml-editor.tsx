"use client";

import { useCallback, useEffect, useRef, useState, useMemo } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { editor, IDisposable } from "monaco-editor";
import { cn } from "@/lib/utils";
import { Loader2, AlertTriangle, CheckCircle, Circle, Wifi, WifiOff } from "lucide-react";
import type { FileType } from "@/types/arena-project";
import { getRuntimeConfig } from "@/lib/config";

// Monaco languageclient imports
import { MonacoLanguageClient } from "monaco-languageclient";
import {
  CloseAction,
  ErrorAction,
  type MessageTransports,
} from "vscode-languageclient/lib/common/client.js";
import {
  toSocket,
  WebSocketMessageReader,
  WebSocketMessageWriter,
} from "vscode-ws-jsonrpc";

interface LspYamlEditorProps {
  value: string;
  onChange?: (value: string) => void;
  onSave?: () => void;
  readOnly?: boolean;
  language?: string;
  fileType?: FileType;
  className?: string;
  loading?: boolean;
  /** Workspace name for LSP context */
  workspace?: string;
  /** Project ID for LSP context */
  projectId?: string;
  /** File path within the project */
  filePath?: string;
}

interface DiagnosticInfo {
  count: number;
  severity: "error" | "warning" | "info" | "hint";
}

type ConnectionStatus = "disconnected" | "connecting" | "connected" | "error";

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

  if (typeof window === "undefined") {
    throw new Error("Cannot create WebSocket connection outside browser");
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
    // Fallback: construct URL from current host with default WebSocket proxy port
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const hostname = window.location.hostname;
    // Default WebSocket proxy port is 3002
    const wsProxyPort = "3002";
    wsUrl = `${protocol}//${hostname}:${wsProxyPort}/api/lsp?workspace=${encodeURIComponent(workspace)}&project=${encodeURIComponent(projectId)}`;
  }

  return new WebSocket(wsUrl);
}

/**
 * Create language client with WebSocket transport
 */
function createLanguageClient(
  transports: MessageTransports
): MonacoLanguageClient {
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
      get: () => Promise.resolve(transports),
    },
  });
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

  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>("disconnected");
  const [diagnostics, setDiagnostics] = useState<DiagnosticInfo | null>(null);
  const [reconnectTrigger, setReconnectTrigger] = useState(0);
  const [monacoReady, setMonacoReady] = useState(false);

  const monacoLanguage = language || getLanguage(fileType);
  const isYaml = monacoLanguage === "yaml";
  const canUseLsp = isYaml && workspace && projectId;

  // Initialize and manage LSP connection
  useEffect(() => {
    if (!canUseLsp || !monacoReady) {
      return;
    }

    let mounted = true;

    const scheduleReconnect = () => {
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
      setConnectionStatus("connecting");

      try {
        const webSocket = await createLspConnection(workspace!, projectId!);
        if (!mounted) {
          webSocket.close();
          return;
        }
        socketRef.current = webSocket;

        webSocket.onopen = () => {
          if (!mounted) return;
          const socket = toSocket(webSocket);
          const reader = new WebSocketMessageReader(socket);
          const writer = new WebSocketMessageWriter(socket);

          const client = createLanguageClient({ reader, writer });
          clientRef.current = client;

          // Start the client
          client.start();
          setConnectionStatus("connected");
        };

        webSocket.onerror = () => {
          if (!mounted) return;
          setConnectionStatus("error");
        };

        webSocket.onclose = () => {
          if (!mounted) return;
          setConnectionStatus("disconnected");
          clientRef.current = null;
          socketRef.current = null;

          // Trigger reconnection after delay
          scheduleReconnect();
        };
      } catch (error) {
        console.error("Failed to connect to LSP server:", error);
        if (mounted) {
          setConnectionStatus("error");
        }
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
  }, [canUseLsp, monacoReady, workspace, projectId, reconnectTrigger]);

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

  // Connection status indicator
  const StatusIndicator = useMemo(() => {
    if (!canUseLsp) {
      return null;
    }

    const statusConfig = {
      disconnected: { icon: WifiOff, color: "text-muted-foreground", label: "LSP Disconnected" },
      connecting: { icon: Circle, color: "text-yellow-500 animate-pulse", label: "Connecting..." },
      connected: { icon: Wifi, color: "text-green-500", label: "LSP Connected" },
      error: { icon: AlertTriangle, color: "text-red-500", label: "LSP Error" },
    };

    const config = statusConfig[connectionStatus];
    const Icon = config.icon;

    return (
      <div className="flex items-center gap-1" title={config.label}>
        <Icon className={cn("h-3 w-3", config.color)} />
        <span className="text-xs text-muted-foreground">{config.label}</span>
      </div>
    );
  }, [canUseLsp, connectionStatus]);

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

  if (loading) {
    return (
      <div className={cn("flex items-center justify-center h-full", className)}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-sm text-muted-foreground">Loading file...</span>
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
          onMount={handleEditorDidMount}
          theme="vs-dark"
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
            {StatusIndicator}
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
