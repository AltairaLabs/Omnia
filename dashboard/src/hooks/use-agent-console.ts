"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type {
  ClientMessage,
  ConsoleMessage,
  ConsoleState,
  ServerMessage,
  ToolCallWithResult,
} from "@/types/websocket";

interface UseAgentConsoleOptions {
  agentName: string;
  namespace: string;
  /** Enable mock mode for demos without a real agent */
  mockMode?: boolean;
  /** WebSocket URL override (defaults to agent service URL) */
  wsUrl?: string;
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

// Mock responses for demo mode
const MOCK_RESPONSES = [
  {
    content: "I'd be happy to help you with that! Let me look into it.",
    toolCalls: [
      {
        name: "search_database",
        arguments: { query: "user request" },
        result: { found: true, records: 3 },
      },
    ],
  },
  {
    content: "Based on my analysis, here's what I found:\n\n1. Your account is in good standing\n2. No recent issues detected\n3. All services are operational\n\nIs there anything specific you'd like me to help you with?",
    toolCalls: [],
  },
  {
    content: "Let me check that for you using our tools.",
    toolCalls: [
      {
        name: "get_user_info",
        arguments: { user_id: "demo-user" },
        result: { name: "Demo User", plan: "premium", created: "2024-01-15" },
      },
      {
        name: "check_permissions",
        arguments: { user_id: "demo-user", resource: "settings" },
        result: { allowed: true, roles: ["admin", "user"] },
      },
    ],
  },
];

export function useAgentConsole({
  agentName,
  namespace,
  mockMode = false,
  wsUrl,
}: UseAgentConsoleOptions): UseAgentConsoleReturn {
  const [state, setState] = useState<ConsoleState>({
    sessionId: null,
    status: "disconnected",
    messages: [],
    error: null,
  });

  const wsRef = useRef<WebSocket | null>(null);
  const mockIndexRef = useRef(0);

  // Build WebSocket URL
  const getWebSocketUrl = useCallback(() => {
    if (wsUrl) return wsUrl;
    // In a real deployment, this would be the agent's service URL
    // For now, use a relative URL that would be proxied
    const protocol = typeof window !== "undefined" && window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}/api/agents/${namespace}/${agentName}/ws`;
  }, [agentName, namespace, wsUrl]);

  // Handle incoming WebSocket messages
  const handleMessage = useCallback((event: MessageEvent) => {
    try {
      const message: ServerMessage = JSON.parse(event.data);

      switch (message.type) {
        case "connected":
          setState((prev) => ({
            ...prev,
            sessionId: message.session_id || null,
            status: "connected",
          }));
          break;

        case "chunk":
          // Append chunk to current streaming message
          setState((prev) => {
            const messages = [...prev.messages];
            const lastMessage = messages[messages.length - 1];

            if (lastMessage?.isStreaming && lastMessage.role === "assistant") {
              lastMessage.content += message.content || "";
            } else {
              // Create new streaming message
              messages.push({
                id: generateId(),
                role: "assistant",
                content: message.content || "",
                timestamp: new Date(message.timestamp),
                isStreaming: true,
                toolCalls: [],
              });
            }

            return { ...prev, messages };
          });
          break;

        case "done":
          // Mark message as complete
          setState((prev) => {
            const messages = [...prev.messages];
            const lastMessage = messages[messages.length - 1];

            if (lastMessage?.isStreaming) {
              lastMessage.isStreaming = false;
              if (message.content) {
                lastMessage.content = message.content;
              }
            }

            return { ...prev, messages };
          });
          break;

        case "tool_call":
          // Add tool call to current message
          if (message.tool_call) {
            setState((prev) => {
              const messages = [...prev.messages];
              const lastMessage = messages[messages.length - 1];

              if (lastMessage?.role === "assistant") {
                const toolCall: ToolCallWithResult = {
                  id: message.tool_call!.id,
                  name: message.tool_call!.name,
                  arguments: message.tool_call!.arguments,
                  status: "pending",
                };
                lastMessage.toolCalls = [...(lastMessage.toolCalls || []), toolCall];
              }

              return { ...prev, messages };
            });
          }
          break;

        case "tool_result":
          // Update tool call with result
          if (message.tool_result) {
            setState((prev) => {
              const messages = [...prev.messages];
              const lastMessage = messages[messages.length - 1];

              if (lastMessage?.toolCalls) {
                const toolCall = lastMessage.toolCalls.find(
                  (tc) => tc.id === message.tool_result!.id
                );
                if (toolCall) {
                  toolCall.result = message.tool_result!.result;
                  toolCall.error = message.tool_result!.error;
                  toolCall.status = message.tool_result!.error ? "error" : "success";
                }
              }

              return { ...prev, messages };
            });
          }
          break;

        case "error":
          setState((prev) => ({
            ...prev,
            error: message.error?.message || "Unknown error",
            status: "error",
          }));
          break;
      }
    } catch {
      console.error("Failed to parse WebSocket message:", event.data);
    }
  }, []);

  // Mock message simulation
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const simulateMockResponse = useCallback((_userMessage: string) => {
    const mockResponse = MOCK_RESPONSES[mockIndexRef.current % MOCK_RESPONSES.length];
    mockIndexRef.current++;

    // Simulate connection delay
    setTimeout(() => {
      setState((prev) => ({
        ...prev,
        sessionId: prev.sessionId || `mock-session-${generateId()}`,
        status: "connected",
      }));
    }, 100);

    // Simulate streaming response
    const words = mockResponse.content.split(" ");
    let currentContent = "";
    const messageId = generateId();

    // Add initial empty assistant message
    setTimeout(() => {
      setState((prev) => ({
        ...prev,
        messages: [
          ...prev.messages,
          {
            id: messageId,
            role: "assistant",
            content: "",
            timestamp: new Date(),
            isStreaming: true,
            toolCalls: [],
          },
        ],
      }));
    }, 300);

    // Stream words one by one
    words.forEach((word, index) => {
      setTimeout(() => {
        currentContent += (index > 0 ? " " : "") + word;
        setState((prev) => ({
          ...prev,
          messages: prev.messages.map((msg) =>
            msg.id === messageId ? { ...msg, content: currentContent } : msg
          ),
        }));
      }, 400 + index * 50);
    });

    // Add tool calls after content
    const toolDelay = 400 + words.length * 50 + 200;
    mockResponse.toolCalls.forEach((tc, index) => {
      const toolId = `tool-${generateId()}`;

      // Show tool call (pending)
      setTimeout(() => {
        setState((prev) => {
          return {
            ...prev,
            messages: prev.messages.map((msg) => {
              if (msg.id !== messageId) return msg;
              // Check if tool already exists (prevent duplicates)
              if (msg.toolCalls?.some((t) => t.id === toolId)) return msg;
              const toolCall: ToolCallWithResult = {
                id: toolId,
                name: tc.name,
                arguments: tc.arguments,
                status: "pending",
              };
              return {
                ...msg,
                toolCalls: [...(msg.toolCalls || []), toolCall],
              };
            }),
          };
        });
      }, toolDelay + index * 700);

      // Show tool result (success) - update same tool call by ID
      setTimeout(() => {
        setState((prev) => {
          return {
            ...prev,
            messages: prev.messages.map((msg) => {
              if (msg.id !== messageId || !msg.toolCalls) return msg;
              return {
                ...msg,
                toolCalls: msg.toolCalls.map((t) =>
                  t.id === toolId
                    ? { ...t, result: tc.result, status: "success" as const }
                    : t
                ),
              };
            }),
          };
        });
      }, toolDelay + index * 700 + 500); // 500ms after pending appears
    });

    // Mark as done
    const doneDelay = toolDelay + mockResponse.toolCalls.length * 700 + 600;
    setTimeout(() => {
      setState((prev) => ({
        ...prev,
        messages: prev.messages.map((msg) =>
          msg.id === messageId ? { ...msg, isStreaming: false } : msg
        ),
      }));
    }, doneDelay);
  }, []);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (mockMode) {
      setState((prev) => ({
        ...prev,
        status: "connected",
        sessionId: `mock-session-${generateId()}`,
        error: null,
      }));
      return;
    }

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    setState((prev) => ({ ...prev, status: "connecting", error: null }));

    try {
      const ws = new WebSocket(getWebSocketUrl());

      ws.onopen = () => {
        setState((prev) => ({ ...prev, status: "connected" }));
      };

      ws.onmessage = handleMessage;

      ws.onerror = () => {
        setState((prev) => ({
          ...prev,
          status: "error",
          error: "WebSocket connection error",
        }));
      };

      ws.onclose = () => {
        setState((prev) => ({ ...prev, status: "disconnected" }));
        wsRef.current = null;
      };

      wsRef.current = ws;
    } catch (err) {
      setState((prev) => ({
        ...prev,
        status: "error",
        error: err instanceof Error ? err.message : "Failed to connect",
      }));
    }
  }, [mockMode, getWebSocketUrl, handleMessage]);

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setState((prev) => ({ ...prev, status: "disconnected" }));
  }, []);

  // Send a message
  const sendMessage = useCallback(
    (content: string) => {
      if (!content.trim()) return;

      // Add user message to state
      const userMessage: ConsoleMessage = {
        id: generateId(),
        role: "user",
        content: content.trim(),
        timestamp: new Date(),
      };

      setState((prev) => ({
        ...prev,
        messages: [...prev.messages, userMessage],
      }));

      if (mockMode) {
        simulateMockResponse(content);
        return;
      }

      if (wsRef.current?.readyState !== WebSocket.OPEN) {
        setState((prev) => ({
          ...prev,
          error: "Not connected to agent",
        }));
        return;
      }

      const clientMessage: ClientMessage = {
        type: "message",
        session_id: state.sessionId || undefined,
        content: content.trim(),
      };

      wsRef.current.send(JSON.stringify(clientMessage));
    },
    [mockMode, state.sessionId, simulateMockResponse]
  );

  // Clear messages
  const clearMessages = useCallback(() => {
    setState((prev) => ({
      ...prev,
      messages: [],
      sessionId: mockMode ? `mock-session-${generateId()}` : null,
    }));
    mockIndexRef.current = 0;
  }, [mockMode]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, []);

  return {
    ...state,
    sendMessage,
    connect,
    disconnect,
    clearMessages,
  };
}
