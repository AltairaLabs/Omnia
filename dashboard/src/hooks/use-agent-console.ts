"use client";

import { useCallback, useEffect, useRef } from "react";
import type {
  ConsoleMessage,
  ConsoleState,
  ServerMessage,
  FileAttachment,
  ContentPart,
} from "@/types/websocket";
import { useDataService, type AgentConnection } from "@/lib/data";
import { useConsoleStore, useSession } from "@/stores";

interface UseAgentConsoleOptions {
  agentName: string;
  namespace: string;
  /** Optional session ID for multi-tab support. If provided, uses this as the store key. */
  sessionId?: string;
}

interface UseAgentConsoleReturn extends ConsoleState {
  sendMessage: (content: string, attachments?: FileAttachment[]) => void;
  connect: () => void;
  disconnect: () => void;
  clearMessages: () => void;
}

// Generate unique IDs with counter to guarantee uniqueness
let idCounter = 0;
function generateId(): string {
  idCounter += 1;
  return `${Date.now()}-${idCounter}-${crypto.randomUUID().slice(0, 8)}`;
}

/**
 * Extract text content from content parts.
 * Concatenates all text parts with newlines.
 */
function extractTextFromParts(parts: ContentPart[]): string {
  return parts
    .filter((part) => part.type === "text" && part.text)
    .map((part) => part.text!)
    .join("\n");
}

/**
 * Convert file attachments to content parts for sending to the server.
 * Parses the data URL to extract mime type and base64 data.
 */
function convertAttachmentsToParts(attachments: FileAttachment[]): ContentPart[] {
  return attachments.map((attachment) => {
    // Parse data URL: "data:image/png;base64,..."
    const match = /^data:([^;]+);base64,(.+)$/.exec(attachment.dataUrl);
    const mimeType = match?.[1] || attachment.type;
    const data = match?.[2] || "";

    // Determine content part type from mime type
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
 * Extracts all media parts (image, audio, video, file) and converts them to FileAttachment format.
 */
function extractAttachmentsFromParts(parts: ContentPart[]): FileAttachment[] {
  return parts
    .filter((part) => part.type !== "text" && part.media)
    .map((part) => {
      const media = part.media!;
      // Generate data URL from base64 data
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
 * Hook for managing agent console WebSocket connections.
 *
 * Uses the DataService abstraction to handle both demo mode (mock)
 * and production mode (real WebSocket) connections transparently.
 *
 * State is persisted in a store so it survives component unmounts.
 */
export function useAgentConsole({
  agentName,
  namespace,
  sessionId: customSessionId,
}: UseAgentConsoleOptions): UseAgentConsoleReturn {
  const service = useDataService();
  const connectionRef = useRef<AgentConnection | null>(null);
  const handlersRegistered = useRef(false);

  // Use persistent store for state
  // sessionId is used as the store key (required for multi-tab support)
  const tabId = customSessionId || `${namespace}/${agentName}`;

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

  // Use refs to access current values in callbacks without dependencies
  const statusRef = useRef(status);
  const messagesRef = useRef(messages);

  // Keep refs in sync
  useEffect(() => {
    statusRef.current = status;
  }, [status]);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  // Handle incoming messages from the connection - stable callback
  // eslint-disable-next-line sonarjs/cognitive-complexity -- Switch statement handles multiple message types; extracting handlers would reduce clarity
  const handleMessage = useCallback((message: ServerMessage) => {
    switch (message.type) {
      case "connected": {
        const newSessionId = message.session_id || null;
        setSessionId(newSessionId);
        // Session ID is displayed in the header status bar, no need for a message
        break;
      }

      case "chunk": {
        // Append to existing streaming message or create new
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
        // Mark message as complete, with multi-modal content support
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
        // Add tool call to current message
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
        // Update tool call with result
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
        // Handle error messages from the proxy or agent
        const errorMsg = message.error?.message || "Unknown error";
        // Error is shown to user via setStatus and logged server-side
        setStatus("error", errorMsg);
        break;
      }
    }
  }, [addMessageToStore, updateLastMessageInStore, setSessionId, setStatus]);

  // Handle status changes from the connection - stable callback
  const handleStatusChange = useCallback((newStatus: ConsoleState["status"], statusError?: string) => {
    const currentStatus = statusRef.current;

    // Add system message for important status changes (not connection - shown in header)
    if (newStatus === "disconnected" && currentStatus === "connected") {
      addMessageToStore({
        id: generateId(),
        role: "system",
        content: "Disconnected from agent",
        timestamp: new Date(),
      });
    } else if (newStatus === "error" && currentStatus !== "error") {
      // Only add error message if we weren't already in error state
      addMessageToStore({
        id: generateId(),
        role: "system",
        content: statusError || "Connection error",
        timestamp: new Date(),
      });
    }

    setStatus(newStatus, statusError || null);
  }, [addMessageToStore, setStatus]);

  // Connect to the agent
  const connect = useCallback(() => {
    // Create a new connection if we don't have one
    if (!connectionRef.current) {
      connectionRef.current = service.createAgentConnection(namespace, agentName);
    }

    // Only register handlers once
    if (!handlersRegistered.current) {
      connectionRef.current.onMessage(handleMessage);
      connectionRef.current.onStatusChange(handleStatusChange);
      handlersRegistered.current = true;
    }

    connectionRef.current.connect();
  }, [service, namespace, agentName, handleMessage, handleStatusChange]);

  // Disconnect from the agent
  const disconnect = useCallback(() => {
    if (connectionRef.current) {
      connectionRef.current.disconnect();
    }
  }, []);

  // Send a message to the agent
  const sendMessage = useCallback((content: string, attachments?: FileAttachment[]) => {
    if (!content.trim() && (!attachments || attachments.length === 0)) return;

    // Add user message to state (with attachments for display)
    const userMessage: ConsoleMessage = {
      id: generateId(),
      role: "user",
      content: content.trim(),
      timestamp: new Date(),
      attachments,
    };

    addMessageToStore(userMessage);

    // Convert attachments to content parts for sending
    const parts = attachments?.length ? convertAttachmentsToParts(attachments) : undefined;

    // Send to connection
    if (connectionRef.current) {
      connectionRef.current.send(content.trim(), { parts });
    } else {
      setStatus("error", "Not connected to agent");
    }
  }, [addMessageToStore, setStatus]);

  // Clear messages and reset session
  const clearMessages = useCallback(() => {
    storeClearMessages(tabId);
  }, [storeClearMessages, tabId]);

  // Cleanup connection on unmount (but NOT the messages)
  useEffect(() => {
    return () => {
      // Only cleanup the connection, not the messages
      if (connectionRef.current) {
        connectionRef.current.disconnect();
        connectionRef.current = null;
        handlersRegistered.current = false;
      }
    };
  }, [namespace, agentName]);

  return {
    sessionId,
    status,
    messages,
    error,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
  };
}
