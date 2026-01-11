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

    expect(mockSend).toHaveBeenCalledWith("Hello, agent!", { parts: undefined });
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
    expect(mockSend).toHaveBeenCalledWith("Hello with spaces", { parts: undefined });
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

describe("useAgentConsole multi-modal content handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearConsoleState("production/multimodal-agent");
  });

  it("should handle done message with text parts", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Processing...",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with parts containing text
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          { type: "text", text: "Here is your response:" },
          { type: "text", text: "Line 2 of response" },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.isStreaming).toBe(false);
    expect(assistantMessage?.content).toBe("Here is your response:\nLine 2 of response");
  });

  it("should handle done message with image parts", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with image part
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          { type: "text", text: "Here is an image:" },
          {
            type: "image",
            media: {
              data: "iVBORw0KGgoAAAANSUhEUg==",
              mime_type: "image/png",
              filename: "test.png",
              size_bytes: 100,
            },
          },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.attachments).toHaveLength(1);
    expect(assistantMessage?.attachments?.[0].type).toBe("image/png");
    expect(assistantMessage?.attachments?.[0].name).toBe("test.png");
    expect(assistantMessage?.attachments?.[0].dataUrl).toBe("data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==");
  });

  it("should handle done message with URL-based media", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with URL-based media
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          {
            type: "video",
            media: {
              url: "https://example.com/video.mp4",
              mime_type: "video/mp4",
              filename: "video.mp4",
            },
          },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.attachments).toHaveLength(1);
    expect(assistantMessage?.attachments?.[0].dataUrl).toBe("https://example.com/video.mp4");
  });

  it("should handle done message with audio parts", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with audio part
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          {
            type: "audio",
            media: {
              data: "SUQzBAAAAAAAI1RTU0UAAAA=",
              mime_type: "audio/mp3",
              filename: "audio.mp3",
              size_bytes: 5000,
            },
          },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.attachments).toHaveLength(1);
    expect(assistantMessage?.attachments?.[0].type).toBe("audio/mp3");
  });

  it("should handle done message with file/document parts", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with file part
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          {
            type: "file",
            media: {
              data: "JVBERi0xLjQKJ",
              mime_type: "application/pdf",
              filename: "document.pdf",
              size_bytes: 10000,
            },
          },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.attachments).toHaveLength(1);
    expect(assistantMessage?.attachments?.[0].type).toBe("application/pdf");
    expect(assistantMessage?.attachments?.[0].name).toBe("document.pdf");
  });

  it("should generate filename when not provided", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with media without filename
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          {
            type: "image",
            media: {
              data: "iVBORw0KGgo=",
              mime_type: "image/jpeg",
              // No filename provided
            },
          },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.attachments?.[0].name).toMatch(/^image-\d+$/);
  });

  it("should handle mixed content parts (text + media)", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with mixed parts
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          { type: "text", text: "Here are your results:" },
          {
            type: "image",
            media: {
              data: "abc123",
              mime_type: "image/png",
              filename: "result.png",
            },
          },
          { type: "text", text: "And some more details." },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.content).toBe("Here are your results:\nAnd some more details.");
    expect(assistantMessage?.attachments).toHaveLength(1);
  });

  it("should handle empty parts array", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message with content
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "Streamed content",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with empty parts - should keep streamed content
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.content).toBe("Streamed content");
    expect(assistantMessage?.isStreaming).toBe(false);
  });

  it("should filter out text parts without text content", () => {
    const { result } = renderHook(() =>
      useAgentConsole({ agentName: "multimodal-agent", namespace: "production" })
    );

    act(() => {
      result.current.connect();
    });

    const onMessageCallback = mockOnMessage.mock.calls[0][0];

    // Create streaming message
    act(() => {
      onMessageCallback({
        type: "chunk",
        content: "",
        timestamp: new Date().toISOString(),
      });
    });

    // Send done with text parts, some without text
    act(() => {
      onMessageCallback({
        type: "done",
        parts: [
          { type: "text", text: "Valid text" },
          { type: "text" }, // No text property
          { type: "text", text: "" }, // Empty text
          { type: "text", text: "More valid text" },
        ],
        timestamp: new Date().toISOString(),
      });
    });

    const assistantMessage = result.current.messages.find(m => m.role === "assistant");
    expect(assistantMessage?.content).toBe("Valid text\nMore valid text");
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
