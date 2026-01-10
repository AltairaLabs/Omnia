import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useAgentConsole } from "./use-agent-console";
import { clearConsoleState } from "./use-console-store";

// Mock connection
const mockConnect = vi.fn();
const mockDisconnect = vi.fn();
const mockSend = vi.fn();
const mockOnMessage = vi.fn();
const mockOnStatusChange = vi.fn();

const mockConnection = {
  connect: mockConnect,
  disconnect: mockDisconnect,
  send: mockSend,
  onMessage: mockOnMessage,
  onStatusChange: mockOnStatusChange,
};

// Mock useDataService
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    createAgentConnection: vi.fn().mockReturnValue(mockConnection),
  }),
}));

// Mock crypto.randomUUID
vi.stubGlobal("crypto", {
  randomUUID: () => "test-uuid-1234",
});

describe("useAgentConsole", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearConsoleState("production/my-agent");
    clearConsoleState("test-session-id");
  });

  it("should return initial state", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    expect(result.current.sessionId).toBeNull();
    expect(result.current.status).toBe("disconnected");
    expect(result.current.messages).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("should have required methods", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    expect(typeof result.current.sendMessage).toBe("function");
    expect(typeof result.current.connect).toBe("function");
    expect(typeof result.current.disconnect).toBe("function");
    expect(typeof result.current.clearMessages).toBe("function");
  });

  it("should connect when connect is called", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    expect(mockConnect).toHaveBeenCalled();
    expect(mockOnMessage).toHaveBeenCalled();
    expect(mockOnStatusChange).toHaveBeenCalled();
  });

  it("should disconnect when disconnect is called", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.disconnect();
    });

    expect(mockDisconnect).toHaveBeenCalled();
  });

  it("should add user message when sending", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("Hello, agent!");
    });

    expect(result.current.messages).toHaveLength(1);
    expect(result.current.messages[0].role).toBe("user");
    expect(result.current.messages[0].content).toBe("Hello, agent!");
  });

  it("should send message through connection", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("Hello, agent!");
    });

    expect(mockSend).toHaveBeenCalledWith("Hello, agent!");
  });

  it("should not send empty messages", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("");
    });

    expect(mockSend).not.toHaveBeenCalled();
    expect(result.current.messages).toHaveLength(0);
  });

  it("should not send whitespace-only messages", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("   ");
    });

    expect(mockSend).not.toHaveBeenCalled();
    expect(result.current.messages).toHaveLength(0);
  });

  it("should clear messages", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("Hello!");
    });

    expect(result.current.messages).toHaveLength(1);

    act(() => {
      result.current.clearMessages();
    });

    expect(result.current.messages).toHaveLength(0);
  });

  it("should handle custom sessionId for multi-tab support", () => {
    const { result } = renderHook(() =>
      useAgentConsole({
        agentName: "my-agent",
        namespace: "production",
        sessionId: "test-session-id",
      })
    );

    expect(result.current.status).toBe("disconnected");
  });

  it("should set error status when sending without connection", () => {
    // Create hook but don't connect
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    // Try to send without connecting
    act(() => {
      result.current.sendMessage("Hello!");
    });

    // Should add user message but set error
    expect(result.current.messages).toHaveLength(1);
    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("Not connected to agent");
  });

  it("should disconnect on unmount", () => {
    const { result, unmount } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    unmount();

    // Connection should be cleaned up
    expect(mockDisconnect).toHaveBeenCalled();
  });

  it("should trim message content", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "my-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    act(() => {
      result.current.sendMessage("  Hello with spaces  ");
    });

    expect(result.current.messages[0].content).toBe("Hello with spaces");
    expect(mockSend).toHaveBeenCalledWith("Hello with spaces");
  });
});

describe("useAgentConsole message handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearConsoleState("production/test-agent");
  });

  it("should handle connected message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    // Get the onMessage callback
    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    act(() => {
      onMessageCallback({
        type: "connected",
        session_id: "sess-123",
        timestamp: new Date().toISOString(),
      });
    });

    expect(result.current.sessionId).toBe("sess-123");
  });

  it("should handle connected message without session_id", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    act(() => {
      onMessageCallback({
        type: "connected",
        timestamp: new Date().toISOString(),
      });
    });

    expect(result.current.sessionId).toBeNull();
  });

  it("should handle chunk message for new streaming message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Hello",
        timestamp: new Date().toISOString(),
      });
    });

    // Should create new streaming message
    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage).toBeDefined();
    expect(assistantMessage?.content).toBe("Hello");
    expect(assistantMessage?.isStreaming).toBe(true);
  });

  it("should append chunk to existing streaming message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // First chunk
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Hello ",
        timestamp: new Date().toISOString(),
      });
    });

    // Second chunk - should append
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "World",
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.content).toBe("Hello World");
    expect(assistantMessage?.isStreaming).toBe(true);
  });

  it("should handle done message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // First create a streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Streaming content",
        timestamp: new Date().toISOString(),
      });
    });

    // Then mark as done
    act(() => {
      onMessageCallback({
        type: "done",
        content: "Final content",
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.isStreaming).toBe(false);
    expect(assistantMessage?.content).toBe("Final content");
  });

  it("should handle tool_call message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // First create a streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Let me search...",
        timestamp: new Date().toISOString(),
      });
    });

    // Add tool call
    act(() => {
      onMessageCallback({
        type: "tool_call",
        tool_call: {
          id: "tool-1",
          name: "search",
          arguments: '{"query": "test"}',
        },
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.toolCalls).toHaveLength(1);
    expect(assistantMessage?.toolCalls?.[0].name).toBe("search");
    expect(assistantMessage?.toolCalls?.[0].status).toBe("pending");
  });

  it("should handle tool_result message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Searching...",
        timestamp: new Date().toISOString(),
      });
    });

    // Add tool call
    act(() => {
      onMessageCallback({
        type: "tool_call",
        tool_call: {
          id: "tool-1",
          name: "search",
          arguments: '{"query": "test"}',
        },
        timestamp: new Date().toISOString(),
      });
    });

    // Add tool result
    act(() => {
      onMessageCallback({
        type: "tool_result",
        tool_result: {
          id: "tool-1",
          result: '{"results": []}',
        },
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.toolCalls?.[0].status).toBe("success");
    expect(assistantMessage?.toolCalls?.[0].result).toBe('{"results": []}');
  });

  it("should handle tool_result with error", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Searching...",
        timestamp: new Date().toISOString(),
      });
    });

    // Add tool call
    act(() => {
      onMessageCallback({
        type: "tool_call",
        tool_call: {
          id: "tool-1",
          name: "search",
          arguments: '{"query": "test"}',
        },
        timestamp: new Date().toISOString(),
      });
    });

    // Add tool result with error
    act(() => {
      onMessageCallback({
        type: "tool_result",
        tool_result: {
          id: "tool-1",
          error: "Search failed",
        },
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.toolCalls?.[0].status).toBe("error");
    expect(assistantMessage?.toolCalls?.[0].error).toBe("Search failed");
  });

  it("should handle error message", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    act(() => {
      onMessageCallback({
        type: "error",
        error: { code: "CONN_ERROR", message: "Connection lost" },
        timestamp: new Date().toISOString(),
      });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("Connection lost");

    consoleSpy.mockRestore();
  });

  it("should handle error message without details", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "test-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    act(() => {
      onMessageCallback({
        type: "error",
        timestamp: new Date().toISOString(),
      });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("Unknown error");

    consoleSpy.mockRestore();
  });
});

describe("useAgentConsole status change handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearConsoleState("production/status-agent");
  });

  it("should add system message on connect", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "status-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onStatusChange = mockOnStatusChange.mock.calls[0][0];

    act(() => {
      onStatusChange("connected");
    });

    const systemMessages = result.current.messages.filter(m => m.role === "system");
    expect(systemMessages.some(m => m.content === "Connected to agent")).toBe(true);
  });

  it("should add system message on disconnect", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "status-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onStatusChange = mockOnStatusChange.mock.calls[0][0];

    // First connect
    act(() => {
      onStatusChange("connected");
    });

    // Then disconnect
    act(() => {
      onStatusChange("disconnected");
    });

    const systemMessages = result.current.messages.filter(m => m.role === "system");
    expect(systemMessages.some(m => m.content === "Disconnected from agent")).toBe(true);
  });

  it("should add system message on error", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "status-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onStatusChange = mockOnStatusChange.mock.calls[0][0];

    act(() => {
      onStatusChange("error", "Connection failed");
    });

    const systemMessages = result.current.messages.filter(m => m.role === "system");
    expect(systemMessages.some(m => m.content === "Connection failed")).toBe(true);
  });
});
