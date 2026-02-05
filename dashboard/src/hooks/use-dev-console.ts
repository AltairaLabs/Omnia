"use client";

import { useCallback, useEffect, useRef } from "react";
import type {
  ConsoleMessage,
  ConsoleState,
  ServerMessage,
  FileAttachment,
  ContentPart,
} from "@/types/websocket";
import { useConsoleStore, useSession } from "@/stores";

interface UseDevConsoleOptions {
  /** Unique session identifier for the dev console */
  sessionId?: string;
  /** Project ID for context (used in reload) */
  projectId?: string;
  /** Workspace name for context */
  workspace?: string;
  /** Namespace for provider/tool access */
  namespace?: string;
  /** Service name for dynamic ArenaDevSession (e.g., "arena-dev-console-test-session") */
  service?: string;
}

interface UseDevConsoleReturn extends ConsoleState {
  sendMessage: (content: string, attachments?: FileAttachment[]) => void;
  connect: () => void;
  disconnect: () => void;
  clearMessages: () => void;
  /** Reload the agent configuration */
  reload: (configPath: string) => void;
  /** Reset the conversation history */
  resetConversation: () => void;
  /** Set the active provider */
  setProvider: (providerId: string) => void;
}

// WebSocket proxy port (matches server.js)
const WS_PROXY_PORT = process.env.NEXT_PUBLIC_WS_PROXY_PORT || "3002";

// Generate unique IDs with counter to guarantee uniqueness
let idCounter = 0;
function generateId(): string {
  idCounter += 1;
  return `${Date.now()}-${idCounter}-${crypto.randomUUID().slice(0, 8)}`;
}

/**
 * Extract text content from content parts.
 */
function extractTextFromParts(parts: ContentPart[]): string {
  return parts
    .filter((part) => part.type === "text" && part.text)
    .map((part) => part.text!)
    .join("\n");
}

/**
 * Convert file attachments to content parts for sending to the server.
 */
function convertAttachmentsToParts(attachments: FileAttachment[]): ContentPart[] {
  return attachments.map((attachment) => {
    const match = /^data:([^;]+);base64,(.+)$/.exec(attachment.dataUrl);
    const mimeType = match?.[1] || attachment.type;
    const data = match?.[2] || "";

    let type: "image" | "audio" | "video" | "file" = "file";
    if (mimeType.startsWith("image/")) type = "image";
    else if (mimeType.startsWith("audio/")) type = "audio";
    else if (mimeType.startsWith("video/")) type = "video";

    return {
      type,
      media: {
        data,
        mime_type: mimeType,
        filename: attachment.name,
        size_bytes: attachment.size,
      },
    };
  });
}

/**
 * Convert content parts to file attachments.
 */
function extractAttachmentsFromParts(parts: ContentPart[]): FileAttachment[] {
  return parts
    .filter((part) => part.type !== "text" && part.media)
    .map((part) => {
      const media = part.media!;
      let dataUrl = "";
      if (media.data) {
        dataUrl = `data:${media.mime_type};base64,${media.data}`;
      } else if (media.url) {
        dataUrl = media.url;
      }

      return {
        id: generateId(),
        name: media.filename || `${part.type}-${Date.now()}`,
        type: media.mime_type,
        size: media.size_bytes || 0,
        dataUrl,
      };
    });
}

/**
 * Hook for managing Arena Dev Console WebSocket connections.
 *
 * Connects to the arena-dev-console service for interactive agent testing.
 * Supports configuration reload without disconnecting.
 */
export function useDevConsole({
  sessionId: customSessionId,
  projectId,
  workspace,
  namespace,
  service,
}: UseDevConsoleOptions = {}): UseDevConsoleReturn {
  const wsRef = useRef<WebSocket | null>(null);
  const mountedRef = useRef(true);

  // Use persistent store for state
  const tabId = customSessionId || `dev-console-${projectId || "default"}`;

  // Get session state
  const session = useSession(tabId);
  const { sessionId, status, messages, error } = session;

  // Get store actions
  const {
    addMessage,
    updateLastMessage,
    setStatus: storeSetStatus,
    setSessionId: storeSetSessionId,
    clearMessages: storeClearMessages,
  } = useConsoleStore();

  // Wrap actions with tabId
  const setStatus = useCallback(
    (newStatus: ConsoleState["status"], statusError?: string | null) => {
      storeSetStatus(tabId, newStatus, statusError);
    },
    [tabId, storeSetStatus]
  );

  const setSessionId = useCallback(
    (newSessionId: string | null) => {
      storeSetSessionId(tabId, newSessionId);
    },
    [tabId, storeSetSessionId]
  );

  const addMessageToStore = useCallback(
    (message: ConsoleMessage) => {
      addMessage(tabId, message);
    },
    [tabId, addMessage]
  );

  const updateLastMessageInStore = useCallback(
    (updater: (msg: ConsoleMessage) => ConsoleMessage) => {
      updateLastMessage(tabId, updater);
    },
    [tabId, updateLastMessage]
  );

  // Refs for current values in callbacks
  const statusRef = useRef(status);
  const messagesRef = useRef(messages);

  useEffect(() => {
    statusRef.current = status;
  }, [status]);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  // Handle incoming messages
  const handleMessage = useCallback((message: ServerMessage) => {
    switch (message.type) {
      case "connected": {
        const newSessionId = message.session_id || null;
        setSessionId(newSessionId);
        break;
      }

      case "chunk": {
        const lastMsg = messagesRef.current[messagesRef.current.length - 1];
        const isStreamingAssistant = lastMsg?.isStreaming && lastMsg.role === "assistant";
        if (isStreamingAssistant) {
          updateLastMessageInStore((msg) => ({ ...msg, content: msg.content + (message.content || "") }));
        } else {
          addMessageToStore({
            id: generateId(),
            role: "assistant",
            content: message.content || "",
            timestamp: new Date(message.timestamp),
            isStreaming: true,
            toolCalls: [],
          });
        }
        break;
      }

      case "done": {
        const hasParts = message.parts && message.parts.length > 0;
        const textFromParts = hasParts ? extractTextFromParts(message.parts!) : "";
        const attachments = hasParts ? extractAttachmentsFromParts(message.parts!) : [];
        const finalContent = textFromParts || message.content || "";

        updateLastMessageInStore((msg) => ({
          ...msg,
          isStreaming: false,
          content: finalContent || msg.content,
          attachments: attachments.length > 0 ? attachments : msg.attachments,
        }));
        break;
      }

      case "tool_call":
        if (!message.tool_call) break;
        updateLastMessageInStore((msg) => ({
          ...msg,
          toolCalls: [...(msg.toolCalls || []), {
            id: message.tool_call!.id,
            name: message.tool_call!.name,
            arguments: message.tool_call!.arguments,
            status: "pending" as const,
          }],
        }));
        break;

      case "tool_result":
        if (!message.tool_result) break;
        updateLastMessageInStore((msg) => {
          const resultId = message.tool_result!.id;
          const resultStatus = message.tool_result!.error ? "error" as const : "success" as const;
          const toolCalls = msg.toolCalls?.map((tc) =>
            tc.id === resultId
              ? { ...tc, result: message.tool_result!.result, error: message.tool_result!.error, status: resultStatus }
              : tc
          );
          return { ...msg, toolCalls };
        });
        break;

      case "error": {
        const errorMsg = message.error?.message || "Unknown error";
        setStatus("error", errorMsg);
        break;
      }

      case "reloaded":
        // Configuration reloaded successfully
        addMessageToStore({
          id: generateId(),
          role: "system",
          content: "Configuration reloaded successfully",
          timestamp: new Date(),
        });
        break;
    }
  }, [addMessageToStore, updateLastMessageInStore, setSessionId, setStatus]);

  // Connect to the dev console
  const connect = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      return; // Already connected
    }

    // Don't attempt connection without a service name
    if (!service) {
      return;
    }

    setStatus("connecting");

    // Build WebSocket URL with workspace/namespace context
    // If service is provided (from ArenaDevSession), use it for dynamic routing
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.hostname;
    const params = new URLSearchParams();
    params.set("agent", "dev-console");
    if (workspace) params.set("workspace", workspace);
    if (namespace) params.set("namespace", namespace);
    if (service) params.set("service", service);
    const wsUrl = `${protocol}//${host}:${WS_PROXY_PORT}/api/dev-console?${params.toString()}`;

    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      if (!mountedRef.current) return;
      setStatus("connected");
    };

    ws.onmessage = (event) => {
      if (!mountedRef.current) return;
      try {
        const message = JSON.parse(event.data) as ServerMessage;
        handleMessage(message);
      } catch {
        console.error("[DevConsole] Failed to parse message:", event.data);
      }
    };

    ws.onclose = () => {
      if (!mountedRef.current) return;
      if (statusRef.current === "connected") {
        addMessageToStore({
          id: generateId(),
          role: "system",
          content: "Disconnected from dev console",
          timestamp: new Date(),
        });
      }
      setStatus("disconnected");
    };

    ws.onerror = (event) => {
      // Don't log errors during unmount (React Strict Mode double-invokes effects)
      if (!mountedRef.current) return;
      console.error("[DevConsole] WebSocket error:", event);
      setStatus("error", "Connection error");
    };

    wsRef.current = ws;
  }, [handleMessage, setStatus, addMessageToStore, workspace, namespace, service]);

  // Disconnect from the dev console
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, []);

  // Send a message
  const sendMessage = useCallback((content: string, attachments?: FileAttachment[]) => {
    if (!content.trim() && (!attachments || attachments.length === 0)) return;

    const userMessage: ConsoleMessage = {
      id: generateId(),
      role: "user",
      content: content.trim(),
      timestamp: new Date(),
      attachments,
    };

    addMessageToStore(userMessage);

    const parts = attachments?.length ? convertAttachmentsToParts(attachments) : undefined;

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: "chat",
        content: content.trim(),
        parts,
        timestamp: new Date().toISOString(),
      }));
    } else {
      setStatus("error", "Not connected to dev console");
    }
  }, [addMessageToStore, setStatus]);

  // Reload configuration
  const reload = useCallback((configPath: string) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: "chat",
        content: configPath,
        metadata: { reload: "true" },
        timestamp: new Date().toISOString(),
      }));

      addMessageToStore({
        id: generateId(),
        role: "system",
        content: `Reloading configuration from ${configPath}...`,
        timestamp: new Date(),
      });
    }
  }, [addMessageToStore]);

  // Reset conversation history
  const resetConversation = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: "chat",
        content: "",
        metadata: { reset: "true" },
        timestamp: new Date().toISOString(),
      }));
    }
    storeClearMessages(tabId);
  }, [storeClearMessages, tabId]);

  // Set active provider
  const setProvider = useCallback((providerId: string) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: "chat",
        content: "",
        metadata: { provider: providerId },
        timestamp: new Date().toISOString(),
      }));

      addMessageToStore({
        id: generateId(),
        role: "system",
        content: `Switched to provider: ${providerId}`,
        timestamp: new Date(),
      });
    }
  }, [addMessageToStore]);

  // Clear messages
  const clearMessages = useCallback(() => {
    storeClearMessages(tabId);
  }, [storeClearMessages, tabId]);

  // Track mount state and cleanup on unmount
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (wsRef.current) {
        // Clear event handlers before closing to prevent error logging during cleanup
        wsRef.current.onopen = null;
        wsRef.current.onclose = null;
        wsRef.current.onerror = null;
        wsRef.current.onmessage = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, []);

  return {
    sessionId,
    status,
    messages,
    error,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
    reload,
    resetConversation,
    setProvider,
  };
}
