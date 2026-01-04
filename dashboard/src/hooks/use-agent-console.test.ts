import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useAgentConsole } from "./use-agent-console";

describe("useAgentConsole", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("mock mode", () => {
    it("should initialize with disconnected status", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      expect(result.current.status).toBe("disconnected");
      expect(result.current.messages).toHaveLength(0);
      expect(result.current.sessionId).toBeNull();
    });

    it("should connect and set session ID in mock mode", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      expect(result.current.status).toBe("connected");
      expect(result.current.sessionId).toMatch(/^mock-session-/);
    });

    it("should add user message when sending", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
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

    it("should simulate streaming assistant response", async () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        result.current.sendMessage("Hello!");
      });

      // Fast-forward to add assistant message
      act(() => {
        vi.advanceTimersByTime(400);
      });

      expect(result.current.messages.length).toBeGreaterThanOrEqual(2);
      const assistantMessage = result.current.messages.find(
        (m) => m.role === "assistant"
      );
      expect(assistantMessage).toBeDefined();
      expect(assistantMessage?.isStreaming).toBe(true);
    });

    it("should add tool calls during response", async () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        result.current.sendMessage("Test message");
      });

      // Fast-forward through streaming and tool calls
      act(() => {
        vi.advanceTimersByTime(5000);
      });

      const assistantMessage = result.current.messages.find(
        (m) => m.role === "assistant"
      );
      expect(assistantMessage?.toolCalls).toBeDefined();
      expect(assistantMessage?.toolCalls?.length).toBeGreaterThan(0);
    });

    it("should clear messages", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        result.current.sendMessage("Hello!");
      });

      expect(result.current.messages.length).toBeGreaterThan(0);

      act(() => {
        result.current.clearMessages();
      });

      expect(result.current.messages).toHaveLength(0);
    });

    it("should disconnect", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      expect(result.current.status).toBe("connected");

      act(() => {
        result.current.disconnect();
      });

      expect(result.current.status).toBe("disconnected");
    });

    it("should not send empty messages", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        result.current.sendMessage("");
      });

      expect(result.current.messages).toHaveLength(0);

      act(() => {
        result.current.sendMessage("   ");
      });

      expect(result.current.messages).toHaveLength(0);
    });

    it("should generate unique message IDs", () => {
      const { result } = renderHook(() =>
        useAgentConsole({
          agentName: "test-agent",
          namespace: "default",
          mockMode: true,
        })
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        result.current.sendMessage("Message 1");
      });

      act(() => {
        result.current.sendMessage("Message 2");
      });

      const ids = result.current.messages.map((m) => m.id);
      const uniqueIds = new Set(ids);
      expect(uniqueIds.size).toBe(ids.length);
    });
  });
});
