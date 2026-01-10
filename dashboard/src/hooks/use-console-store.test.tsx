import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useConsoleStore, useConsoleStoreBySession, clearConsoleState } from "./use-console-store";

describe("useConsoleStoreBySession", () => {
  beforeEach(() => {
    // Clear all state before each test
    clearConsoleState("test-session");
    clearConsoleState("session-1");
    clearConsoleState("session-2");
    clearConsoleState("production/my-agent");
  });

  it("should return default state initially", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    expect(result.current.sessionId).toBeNull();
    expect(result.current.status).toBe("disconnected");
    expect(result.current.messages).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("should set messages", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.setMessages([
        { id: "1", role: "user", content: "Hello", timestamp: new Date() },
      ]);
    });

    expect(result.current.messages).toHaveLength(1);
    expect(result.current.messages[0].content).toBe("Hello");
  });

  it("should add a message", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.addMessage({
        id: "1",
        role: "user",
        content: "First message",
        timestamp: new Date(),
      });
    });

    act(() => {
      result.current.addMessage({
        id: "2",
        role: "assistant",
        content: "Response",
        timestamp: new Date(),
      });
    });

    expect(result.current.messages).toHaveLength(2);
    expect(result.current.messages[0].content).toBe("First message");
    expect(result.current.messages[1].content).toBe("Response");
  });

  it("should update last message", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.addMessage({
        id: "1",
        role: "assistant",
        content: "Streaming...",
        timestamp: new Date(),
        isStreaming: true,
      });
    });

    act(() => {
      result.current.updateLastMessage((msg) => ({
        ...msg,
        content: msg.content + " done",
        isStreaming: false,
      }));
    });

    expect(result.current.messages[0].content).toBe("Streaming... done");
    expect(result.current.messages[0].isStreaming).toBe(false);
  });

  it("should not update if no messages", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    // Should not throw when there are no messages
    act(() => {
      result.current.updateLastMessage((msg) => ({
        ...msg,
        content: "Updated",
      }));
    });

    expect(result.current.messages).toHaveLength(0);
  });

  it("should set status", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.setStatus("connected");
    });

    expect(result.current.status).toBe("connected");
    expect(result.current.error).toBeNull();
  });

  it("should set status with error", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.setStatus("error", "Connection failed");
    });

    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe("Connection failed");
  });

  it("should set session ID", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.setSessionId("sess-12345");
    });

    expect(result.current.sessionId).toBe("sess-12345");
  });

  it("should clear messages and reset session ID", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.setSessionId("sess-12345");
      result.current.addMessage({
        id: "1",
        role: "user",
        content: "Hello",
        timestamp: new Date(),
      });
    });

    expect(result.current.messages).toHaveLength(1);
    expect(result.current.sessionId).toBe("sess-12345");

    act(() => {
      result.current.clearMessages();
    });

    expect(result.current.messages).toHaveLength(0);
    expect(result.current.sessionId).toBeNull();
  });

  it("should persist state across component unmounts", () => {
    const { result, unmount } = renderHook(() => useConsoleStoreBySession("session-1"));

    act(() => {
      result.current.addMessage({
        id: "1",
        role: "user",
        content: "Persisted message",
        timestamp: new Date(),
      });
    });

    unmount();

    // Render new hook with same key
    const { result: result2 } = renderHook(() => useConsoleStoreBySession("session-1"));

    expect(result2.current.messages).toHaveLength(1);
    expect(result2.current.messages[0].content).toBe("Persisted message");
  });

  it("should maintain separate state for different sessions", () => {
    const { result: result1 } = renderHook(() => useConsoleStoreBySession("session-1"));
    const { result: result2 } = renderHook(() => useConsoleStoreBySession("session-2"));

    act(() => {
      result1.current.addMessage({
        id: "1",
        role: "user",
        content: "Session 1 message",
        timestamp: new Date(),
      });
    });

    act(() => {
      result2.current.addMessage({
        id: "2",
        role: "user",
        content: "Session 2 message",
        timestamp: new Date(),
      });
    });

    expect(result1.current.messages[0].content).toBe("Session 1 message");
    expect(result2.current.messages[0].content).toBe("Session 2 message");
  });
});

describe("useConsoleStore", () => {
  beforeEach(() => {
    clearConsoleState("production/my-agent");
    clearConsoleState("staging/other-agent");
  });

  it("should use namespace/agentName as key", () => {
    const { result } = renderHook(() => useConsoleStore("production", "my-agent"));

    act(() => {
      result.current.addMessage({
        id: "1",
        role: "user",
        content: "Test",
        timestamp: new Date(),
      });
    });

    // Verify using the same key returns same state
    const { result: result2 } = renderHook(() =>
      useConsoleStoreBySession("production/my-agent")
    );

    expect(result2.current.messages[0].content).toBe("Test");
  });

  it("should maintain separate state for different agents", () => {
    const { result: result1 } = renderHook(() => useConsoleStore("production", "my-agent"));
    const { result: result2 } = renderHook(() => useConsoleStore("staging", "other-agent"));

    act(() => {
      result1.current.setStatus("connected");
    });

    act(() => {
      result2.current.setStatus("error", "Failed");
    });

    expect(result1.current.status).toBe("connected");
    expect(result2.current.status).toBe("error");
  });
});

describe("clearConsoleState", () => {
  it("should clear state for a specific key", () => {
    const { result } = renderHook(() => useConsoleStoreBySession("test-session"));

    act(() => {
      result.current.addMessage({
        id: "1",
        role: "user",
        content: "Message",
        timestamp: new Date(),
      });
      result.current.setStatus("connected");
    });

    expect(result.current.messages).toHaveLength(1);

    act(() => {
      clearConsoleState("test-session");
    });

    // Re-render should get default state
    const { result: result2 } = renderHook(() => useConsoleStoreBySession("test-session"));
    expect(result2.current.messages).toHaveLength(0);
    expect(result2.current.status).toBe("disconnected");
  });
});
