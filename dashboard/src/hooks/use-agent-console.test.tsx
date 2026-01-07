import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useAgentConsole } from "./use-agent-console";
import { DataServiceProvider } from "@/lib/data";
import { MockAgentConnection } from "@/lib/data/mock-service";
import type { DataService, AgentConnection } from "@/lib/data/types";
import type { ReactNode } from "react";

// Create a mock DataService that uses MockAgentConnection
function createMockDataService(): DataService {
  return {
    name: "TestMockDataService",
    isDemo: true,
    getAgents: vi.fn().mockResolvedValue([]),
    getAgent: vi.fn().mockResolvedValue(undefined),
    createAgent: vi.fn().mockResolvedValue({}),
    scaleAgent: vi.fn().mockResolvedValue({}),
    getAgentLogs: vi.fn().mockResolvedValue([]),
    getAgentEvents: vi.fn().mockResolvedValue([]),
    getPromptPacks: vi.fn().mockResolvedValue([]),
    getPromptPack: vi.fn().mockResolvedValue(undefined),
    getToolRegistries: vi.fn().mockResolvedValue([]),
    getToolRegistry: vi.fn().mockResolvedValue(undefined),
    getProviders: vi.fn().mockResolvedValue([]),
    getStats: vi.fn().mockResolvedValue({ agents: {}, promptPacks: {}, tools: {} }),
    getNamespaces: vi.fn().mockResolvedValue([]),
    getCosts: vi.fn().mockResolvedValue({ available: false, summary: {}, byAgent: [], byProvider: [], byModel: [], timeSeries: [] }),
    createAgentConnection: (namespace: string, name: string): AgentConnection => {
      return new MockAgentConnection(namespace, name);
    },
  };
}

// Wrapper component to provide DataService context
function createWrapper(service: DataService) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <DataServiceProvider initialService={service}>
        {children}
      </DataServiceProvider>
    );
  };
}

describe("useAgentConsole", () => {
  let mockService: DataService;

  beforeEach(() => {
    vi.useFakeTimers();
    mockService = createMockDataService();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("with DataService (mock connection)", () => {
    it("should initialize with disconnected status", () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      expect(result.current.status).toBe("disconnected");
      expect(result.current.messages).toHaveLength(0);
      expect(result.current.sessionId).toBeNull();
    });

    it("should connect and set session ID", async () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      // MockAgentConnection has a 200ms delay for connection
      act(() => {
        vi.advanceTimersByTime(300);
      });

      expect(result.current.status).toBe("connected");
      expect(result.current.sessionId).toMatch(/^mock-session-/);
    });

    it("should add user message when sending", () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
      });

      act(() => {
        result.current.sendMessage("Hello, agent!");
      });

      expect(result.current.messages).toHaveLength(1);
      expect(result.current.messages[0].role).toBe("user");
      expect(result.current.messages[0].content).toBe("Hello, agent!");
    });

    it("should simulate streaming assistant response", async () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
      });

      act(() => {
        result.current.sendMessage("Hello!");
      });

      // Fast-forward to receive streaming chunks
      act(() => {
        vi.advanceTimersByTime(500);
      });

      expect(result.current.messages.length).toBeGreaterThanOrEqual(2);
      const assistantMessage = result.current.messages.find(
        (m) => m.role === "assistant"
      );
      expect(assistantMessage).toBeDefined();
      expect(assistantMessage?.isStreaming).toBe(true);
    });

    it("should add tool calls during response", async () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
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
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
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
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
      });

      expect(result.current.status).toBe("connected");

      act(() => {
        result.current.disconnect();
      });

      expect(result.current.status).toBe("disconnected");
    });

    it("should not send empty messages", () => {
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
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
      const { result } = renderHook(
        () => useAgentConsole({ agentName: "test-agent", namespace: "default" }),
        { wrapper: createWrapper(mockService) }
      );

      act(() => {
        result.current.connect();
      });

      act(() => {
        vi.advanceTimersByTime(300);
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
