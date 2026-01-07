"use client";

import { useCallback, useEffect, useRef } from "react";
import type {
  ConsoleMessage,
  ConsoleState,
  ServerMessage,
  ToolCallWithResult,
} from "@/types/websocket";
import { useDataService, type AgentConnection } from "@/lib/data";
import { useConsoleStore } from "./use-console-store";

interface UseAgentConsoleOptions {
  agentName: string;
  namespace: string;
}

interface UseAgentConsoleReturn extends ConsoleState {
  sendMessage: (content: string) => void;
  connect: () => void;
  disconnect: () => void;
  clearMessages: () => void;
}

// Generate unique IDs with counter to guarantee uniqueness
let idCounter = 0;
function generateId(): string {
  idCounter += 1;
  return `${Date.now()}-${idCounter}-${Math.random().toString(36).slice(2, 7)}`;
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
}: UseAgentConsoleOptions): UseAgentConsoleReturn {
  const service = useDataService();
  const connectionRef = useRef<AgentConnection | null>(null);
  const handlersRegistered = useRef(false);

  // Use persistent store for state
  const store = useConsoleStore(namespace, agentName);
  const {
    sessionId,
    status,
    messages,
    error,
    addMessage,
    updateLastMessage,
    setStatus,
    setSessionId,
    clearMessages: storeClearMessages,
  } = store;

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
  const handleMessage = useCallback((message: ServerMessage) => {
    switch (message.type) {
      case "connected":
        setSessionId(message.session_id || null);
        break;

      case "chunk":
        // Check if we need to append to existing streaming message or create new
        const currentMessages = messagesRef.current;
        const lastMsg = currentMessages[currentMessages.length - 1];
        if (lastMsg?.isStreaming && lastMsg.role === "assistant") {
          updateLastMessage((msg) => ({
            ...msg,
            content: msg.content + (message.content || ""),
          }));
        } else {
          // Create new streaming message
          addMessage({
            id: generateId(),
            role: "assistant",
            content: message.content || "",
            timestamp: new Date(message.timestamp),
            isStreaming: true,
            toolCalls: [],
          });
        }
        break;

      case "done":
        // Mark message as complete
        updateLastMessage((msg) => ({
          ...msg,
          isStreaming: false,
          content: message.content || msg.content,
        }));
        break;

      case "tool_call":
        // Add tool call to current message
        if (message.tool_call) {
          updateLastMessage((msg) => {
            const toolCall: ToolCallWithResult = {
              id: message.tool_call!.id,
              name: message.tool_call!.name,
              arguments: message.tool_call!.arguments,
              status: "pending",
            };
            return {
              ...msg,
              toolCalls: [...(msg.toolCalls || []), toolCall],
            };
          });
        }
        break;

      case "tool_result":
        // Update tool call with result
        if (message.tool_result) {
          updateLastMessage((msg) => {
            const toolCalls = msg.toolCalls?.map((tc) => {
              if (tc.id === message.tool_result!.id) {
                return {
                  ...tc,
                  result: message.tool_result!.result,
                  error: message.tool_result!.error,
                  status: message.tool_result!.error ? "error" as const : "success" as const,
                };
              }
              return tc;
            });
            return { ...msg, toolCalls };
          });
        }
        break;

      case "error":
        // Handle error messages from the proxy or agent
        const errorMsg = message.error?.message || "Unknown error";
        const errorCode = message.error?.code || "UNKNOWN";
        console.error(`[useAgentConsole] Error from server: [${errorCode}] ${errorMsg}`);
        setStatus("error", errorMsg);
        break;
    }
  }, [addMessage, updateLastMessage, setSessionId, setStatus]);

  // Handle status changes from the connection - stable callback
  const handleStatusChange = useCallback((newStatus: ConsoleState["status"], statusError?: string) => {
    const currentStatus = statusRef.current;

    // Add system message for status changes
    if (newStatus === "connected" && currentStatus !== "connected") {
      addMessage({
        id: generateId(),
        role: "system",
        content: "Connected to agent",
        timestamp: new Date(),
      });
    } else if (newStatus === "disconnected" && currentStatus === "connected") {
      addMessage({
        id: generateId(),
        role: "system",
        content: "Disconnected from agent",
        timestamp: new Date(),
      });
    } else if (newStatus === "error" && currentStatus !== "error") {
      // Only add error message if we weren't already in error state
      addMessage({
        id: generateId(),
        role: "system",
        content: statusError || "Connection error",
        timestamp: new Date(),
      });
    }

    setStatus(newStatus, statusError || null);
  }, [addMessage, setStatus]);

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
  const sendMessage = useCallback((content: string) => {
    if (!content.trim()) return;

    // Add user message to state
    const userMessage: ConsoleMessage = {
      id: generateId(),
      role: "user",
      content: content.trim(),
      timestamp: new Date(),
    };

    addMessage(userMessage);

    // Send to connection
    if (connectionRef.current) {
      connectionRef.current.send(content.trim());
    } else {
      setStatus("error", "Not connected to agent");
    }
  }, [addMessage, setStatus]);

  // Clear messages and reset session
  const clearMessages = useCallback(() => {
    storeClearMessages();
  }, [storeClearMessages]);

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
