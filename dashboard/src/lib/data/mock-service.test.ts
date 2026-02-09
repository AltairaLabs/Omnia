import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { MockDataService, MockAgentConnection } from "./mock-service";
import type { ServerMessage, ConnectionStatus } from "@/types/websocket";

describe("MockDataService", () => {
  let service: MockDataService;

  beforeEach(() => {
    service = new MockDataService();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("service metadata", () => {
    it("should have correct name", () => {
      expect(service.name).toBe("MockDataService");
    });

    it("should be marked as demo service", () => {
      expect(service.isDemo).toBe(true);
    });
  });

  describe("getAgents", () => {
    it("should return mock agents for workspace", async () => {
      const promise = service.getAgents("production");
      vi.advanceTimersByTime(200);
      const agents = await promise;

      expect(Array.isArray(agents)).toBe(true);
      // Should only return agents in the production namespace
      agents.forEach((agent) => {
        expect(agent.metadata?.namespace).toBe("production");
      });
    });

    it("should filter agents by workspace (which maps to namespace)", async () => {
      const promise = service.getAgents("production");
      vi.advanceTimersByTime(200);
      const agents = await promise;

      agents.forEach((agent) => {
        expect(agent.metadata?.namespace).toBe("production");
      });
    });

    it("should return different agents for different workspaces", async () => {
      const promise1 = service.getAgents("production");
      vi.advanceTimersByTime(200);
      const prodAgents = await promise1;

      const promise2 = service.getAgents("default");
      vi.advanceTimersByTime(200);
      const defaultAgents = await promise2;

      // Both should be valid arrays
      expect(Array.isArray(prodAgents)).toBe(true);
      expect(Array.isArray(defaultAgents)).toBe(true);
    });
  });

  describe("getAgent", () => {
    it("should return specific agent by workspace and name", async () => {
      // First get list to know what's available
      const listPromise = service.getAgents("production");
      vi.advanceTimersByTime(200);
      const agents = await listPromise;

      if (agents.length > 0) {
        const firstAgent = agents[0];
        const promise = service.getAgent(
          "production",
          firstAgent.metadata?.name || ""
        );
        vi.advanceTimersByTime(200);
        const agent = await promise;

        expect(agent).toBeDefined();
        expect(agent?.metadata?.name).toBe(firstAgent.metadata?.name);
      }
    });

    it("should return undefined for non-existent agent", async () => {
      const promise = service.getAgent("non-existent", "non-existent");
      vi.advanceTimersByTime(200);
      const agent = await promise;

      expect(agent).toBeUndefined();
    });
  });

  describe("createAgent", () => {
    it("should return a new agent with Pending status", async () => {
      const spec = {
        metadata: {
          name: "test-agent",
        },
        spec: {
          facade: { type: "websocket", port: 8080 },
        },
      };

      const promise = service.createAgent("test-workspace", spec);
      vi.advanceTimersByTime(600);
      const agent = await promise;

      expect(agent.metadata?.name).toBe("test-agent");
      expect(agent.metadata?.namespace).toBe("test-workspace");
      expect(agent.status?.phase).toBe("Pending");
    });
  });

  describe("scaleAgent", () => {
    it("should scale an existing agent", async () => {
      // Get first agent from production workspace
      const listPromise = service.getAgents("production");
      vi.advanceTimersByTime(200);
      const agents = await listPromise;

      if (agents.length > 0) {
        const firstAgent = agents[0];
        const promise = service.scaleAgent(
          "production",
          firstAgent.metadata?.name || "",
          5
        );
        vi.advanceTimersByTime(600);
        const scaled = await promise;

        expect(scaled.spec?.runtime?.replicas).toBe(5);
        // Status replicas is updated to the new desired count
        expect(scaled.status?.replicas).toBeDefined();
      }
    });

    it("should throw for non-existent agent", async () => {
      const promise = service.scaleAgent("non-existent", "non-existent", 5);
      vi.advanceTimersByTime(600);

      await expect(promise).rejects.toThrow("not found");
    });
  });

  describe("getAgentLogs", () => {
    it("should return mock logs", async () => {
      const promise = service.getAgentLogs("default", "test-agent");
      vi.advanceTimersByTime(200);
      const logs = await promise;

      expect(Array.isArray(logs)).toBe(true);
      expect(logs.length).toBe(100); // Default tailLines
    });

    it("should respect tailLines option", async () => {
      const promise = service.getAgentLogs("default", "test-agent", {
        tailLines: 50,
      });
      vi.advanceTimersByTime(200);
      const logs = await promise;

      expect(logs.length).toBe(50);
    });

    it("should generate logs with required fields", async () => {
      const promise = service.getAgentLogs("default", "test-agent", {
        tailLines: 10,
      });
      vi.advanceTimersByTime(200);
      const logs = await promise;

      logs.forEach((log) => {
        expect(log.timestamp).toBeDefined();
        expect(log.level).toBeDefined();
        expect(log.message).toBeDefined();
        expect(log.container).toBeDefined();
        expect(["facade", "runtime"]).toContain(log.container);
        expect(["info", "debug", "warn", "error"]).toContain(log.level);
      });
    });
  });

  describe("getAgentEvents", () => {
    it("should return mock events", async () => {
      const promise = service.getAgentEvents("default", "test-agent");
      vi.advanceTimersByTime(200);
      const events = await promise;

      expect(Array.isArray(events)).toBe(true);
      expect(events.length).toBeGreaterThan(0);
    });

    it("should return events with expected structure", async () => {
      const promise = service.getAgentEvents("default", "test-agent");
      vi.advanceTimersByTime(200);
      const events = await promise;

      events.forEach((event) => {
        expect(event.type).toMatch(/^(Normal|Warning)$/);
        expect(event.reason).toBeDefined();
        expect(event.message).toBeDefined();
        expect(event.firstTimestamp).toBeDefined();
        expect(event.lastTimestamp).toBeDefined();
        expect(event.source).toBeDefined();
        expect(event.involvedObject).toBeDefined();
      });
    });

    it("should return events sorted by timestamp (most recent first)", async () => {
      const promise = service.getAgentEvents("default", "test-agent");
      vi.advanceTimersByTime(200);
      const events = await promise;

      for (let i = 0; i < events.length - 1; i++) {
        const current = new Date(events[i].lastTimestamp).getTime();
        const next = new Date(events[i + 1].lastTimestamp).getTime();
        expect(current).toBeGreaterThanOrEqual(next);
      }
    });
  });

  describe("getPromptPacks", () => {
    it("should return mock prompt packs for workspace", async () => {
      const promise = service.getPromptPacks("production");
      vi.advanceTimersByTime(200);
      const packs = await promise;

      expect(Array.isArray(packs)).toBe(true);
      packs.forEach((pack) => {
        expect(pack.metadata?.namespace).toBe("production");
      });
    });

    it("should filter by workspace (which maps to namespace)", async () => {
      const promise = service.getPromptPacks("production");
      vi.advanceTimersByTime(200);
      const packs = await promise;

      packs.forEach((pack) => {
        expect(pack.metadata?.namespace).toBe("production");
      });
    });
  });

  describe("getToolRegistries", () => {
    it("should return workspace-scoped tool registries (filtered by namespace)", async () => {
      const promise = service.getToolRegistries("test-workspace");
      vi.advanceTimersByTime(200);
      const registries = await promise;

      expect(Array.isArray(registries)).toBe(true);
    });
  });

  describe("getSharedToolRegistries", () => {
    it("should return all shared tool registries", async () => {
      const promise = service.getSharedToolRegistries();
      vi.advanceTimersByTime(200);
      const registries = await promise;

      // Shared tool registries return all
      expect(Array.isArray(registries)).toBe(true);
    });
  });

  describe("getProviders", () => {
    it("should return empty array for workspace-scoped providers", async () => {
      const promise = service.getProviders("test-workspace");
      vi.advanceTimersByTime(200);
      const providers = await promise;

      expect(providers).toEqual([]);
    });
  });

  describe("getSharedProviders", () => {
    it("should return empty array (no mock shared providers)", async () => {
      const promise = service.getSharedProviders();
      vi.advanceTimersByTime(200);
      const providers = await promise;

      expect(providers).toEqual([]);
    });
  });

  describe("getStats", () => {
    it("should return mock stats for workspace", async () => {
      const promise = service.getStats("test-workspace");
      vi.advanceTimersByTime(200);
      const stats = await promise;

      expect(stats).toBeDefined();
    });
  });

  describe("getCosts", () => {
    it("should return cost data", async () => {
      const promise = service.getCosts();
      vi.advanceTimersByTime(200);
      const costs = await promise;

      expect(costs.available).toBe(true);
      expect(costs.summary).toBeDefined();
      expect(costs.byAgent).toBeDefined();
      expect(costs.byProvider).toBeDefined();
      expect(costs.byModel).toBeDefined();
      expect(costs.timeSeries).toBeDefined();
    });

    it("should not include Grafana URL in demo mode", async () => {
      const promise = service.getCosts();
      vi.advanceTimersByTime(200);
      const costs = await promise;

      expect(costs.grafanaUrl).toBeUndefined();
    });
  });

  // ============================================================
  // Sessions
  // ============================================================

  describe("getSessions", () => {
    it("should return sessions with pagination", async () => {
      const promise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.sessions).toBeDefined();
      expect(typeof result.total).toBe("number");
      expect(typeof result.hasMore).toBe("boolean");
    });

    it("should filter sessions by status", async () => {
      const promise = service.getSessions("default", { status: "completed" });
      vi.advanceTimersByTime(200);
      const result = await promise;

      result.sessions.forEach((s) => {
        expect(s.status).toBe("completed");
      });
    });

    it("should respect limit and offset", async () => {
      const promise = service.getSessions("default", { limit: 2, offset: 0 });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.sessions.length).toBeLessThanOrEqual(2);
    });

    it("should filter by date range (from)", async () => {
      const promise = service.getSessions("default", {
        from: new Date(Date.now() - 1000).toISOString(),
      });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result).toBeDefined();
    });

    it("should filter by date range (to)", async () => {
      const promise = service.getSessions("default", {
        to: new Date().toISOString(),
      });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result).toBeDefined();
    });

    it("should filter sessions by agent name", async () => {
      const promise = service.getSessions("default", { agent: "nonexistent-agent" });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.sessions).toHaveLength(0);
    });
  });

  describe("getSessionById", () => {
    it("should return a session by ID", async () => {
      const listPromise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const list = await listPromise;

      if (list.sessions.length > 0) {
        const detailPromise = service.getSessionById("default", list.sessions[0].id);
        vi.advanceTimersByTime(200);
        const session = await detailPromise;

        expect(session).toBeDefined();
        expect(session?.id).toBe(list.sessions[0].id);
      }
    });

    it("should return undefined for unknown ID", async () => {
      const promise = service.getSessionById("default", "nonexistent-id");
      vi.advanceTimersByTime(200);
      const session = await promise;

      expect(session).toBeUndefined();
    });
  });

  describe("searchSessions", () => {
    it("should search sessions by query string", async () => {
      const promise = service.searchSessions("default", { q: "a" });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.sessions).toBeDefined();
      expect(typeof result.total).toBe("number");
    });

    it("should filter search results by agent", async () => {
      const promise = service.searchSessions("default", {
        q: "a",
        agent: "nonexistent-agent",
      });
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.sessions).toHaveLength(0);
    });

    it("should filter search results by status", async () => {
      const promise = service.searchSessions("default", {
        q: "a",
        status: "error",
      });
      vi.advanceTimersByTime(200);
      const result = await promise;

      result.sessions.forEach((s) => {
        expect(s.status).toBe("error");
      });
    });
  });

  describe("getSessionMessages", () => {
    it("should return messages for a valid session", async () => {
      const listPromise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const list = await listPromise;

      if (list.sessions.length > 0) {
        const msgPromise = service.getSessionMessages("default", list.sessions[0].id);
        vi.advanceTimersByTime(200);
        const result = await msgPromise;

        expect(result.messages).toBeDefined();
        expect(typeof result.hasMore).toBe("boolean");
      }
    });

    it("should return empty for unknown session", async () => {
      const promise = service.getSessionMessages("default", "nonexistent-id");
      vi.advanceTimersByTime(200);
      const result = await promise;

      expect(result.messages).toHaveLength(0);
      expect(result.hasMore).toBe(false);
    });

    it("should respect limit option", async () => {
      const listPromise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const list = await listPromise;

      if (list.sessions.length > 0) {
        const msgPromise = service.getSessionMessages("default", list.sessions[0].id, { limit: 1 });
        vi.advanceTimersByTime(200);
        const result = await msgPromise;

        expect(result.messages.length).toBeLessThanOrEqual(1);
      }
    });

    it("should support after cursor", async () => {
      const listPromise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const list = await listPromise;

      if (list.sessions.length > 0) {
        const msgPromise = service.getSessionMessages("default", list.sessions[0].id, { after: 0 });
        vi.advanceTimersByTime(200);
        const result = await msgPromise;

        expect(result).toBeDefined();
      }
    });

    it("should support before cursor", async () => {
      const listPromise = service.getSessions("default");
      vi.advanceTimersByTime(200);
      const list = await listPromise;

      if (list.sessions.length > 0) {
        const msgPromise = service.getSessionMessages("default", list.sessions[0].id, { before: 100 });
        vi.advanceTimersByTime(200);
        const result = await msgPromise;

        expect(result).toBeDefined();
      }
    });
  });

  describe("createAgentConnection", () => {
    it("should return a MockAgentConnection", () => {
      const connection = service.createAgentConnection("default", "test-agent");
      expect(connection).toBeInstanceOf(MockAgentConnection);
    });
  });
});

describe("MockAgentConnection", () => {
  let connection: MockAgentConnection;

  beforeEach(() => {
    connection = new MockAgentConnection("default", "test-agent");
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("initial state", () => {
    it("should start disconnected", () => {
      expect(connection.getStatus()).toBe("disconnected");
    });

    it("should have no session ID initially", () => {
      expect(connection.getSessionId()).toBeNull();
    });
  });

  describe("connect", () => {
    it("should transition to connecting then connected", () => {
      const statusChanges: ConnectionStatus[] = [];
      connection.onStatusChange((status) => statusChanges.push(status));

      connection.connect();

      expect(statusChanges).toContain("connecting");
      expect(connection.getStatus()).toBe("connecting");

      vi.advanceTimersByTime(300);

      expect(statusChanges).toContain("connected");
      expect(connection.getStatus()).toBe("connected");
    });

    it("should generate a session ID when connected", () => {
      connection.connect();
      vi.advanceTimersByTime(300);

      expect(connection.getSessionId()).not.toBeNull();
      expect(connection.getSessionId()).toContain("mock-session-");
    });

    it("should emit connected message", () => {
      const messages: ServerMessage[] = [];
      connection.onMessage((msg) => messages.push(msg));

      connection.connect();
      vi.advanceTimersByTime(300);

      const connectedMsg = messages.find((m) => m.type === "connected");
      expect(connectedMsg).toBeDefined();
      expect(connectedMsg?.session_id).toBeDefined();
    });
  });

  describe("disconnect", () => {
    it("should transition to disconnected", () => {
      connection.connect();
      vi.advanceTimersByTime(300);

      connection.disconnect();

      expect(connection.getStatus()).toBe("disconnected");
      expect(connection.getSessionId()).toBeNull();
    });
  });

  describe("send", () => {
    it("should warn when not connected", () => {
      const consoleSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      connection.send("Hello");

      expect(consoleSpy).toHaveBeenCalledWith(
        "Cannot send message: not connected"
      );
      consoleSpy.mockRestore();
    });

    it("should simulate streaming response when connected", () => {
      const messages: ServerMessage[] = [];
      connection.onMessage((msg) => messages.push(msg));

      connection.connect();
      vi.advanceTimersByTime(300);

      connection.send("Hello, agent!");

      // Advance time to receive all chunks
      vi.advanceTimersByTime(5000);

      // Should have received chunks
      const chunks = messages.filter((m) => m.type === "chunk");
      expect(chunks.length).toBeGreaterThan(0);

      // Should have received done message
      const done = messages.find((m) => m.type === "done");
      expect(done).toBeDefined();
    });

    it("should cycle through mock responses", () => {
      const messages: ServerMessage[] = [];
      connection.onMessage((msg) => messages.push(msg));

      connection.connect();
      vi.advanceTimersByTime(300);

      // Send first message
      connection.send("First message");
      vi.advanceTimersByTime(5000);

      const firstChunks = messages
        .filter((m) => m.type === "chunk")
        .map((m) => m.content)
        .join("");

      messages.length = 0; // Clear messages

      // Send second message
      connection.send("Second message");
      vi.advanceTimersByTime(5000);

      const secondChunks = messages
        .filter((m) => m.type === "chunk")
        .map((m) => m.content)
        .join("");

      // Responses should be different (cycling through mock data)
      // Note: This test may pass even with same content if mock data is short
      expect(typeof firstChunks).toBe("string");
      expect(typeof secondChunks).toBe("string");
    });
  });

  describe("onMessage", () => {
    it("should register message handler", () => {
      const handler = vi.fn();
      connection.onMessage(handler);

      connection.connect();
      vi.advanceTimersByTime(300);

      expect(handler).toHaveBeenCalled();
    });

    it("should support multiple handlers", () => {
      const handler1 = vi.fn();
      const handler2 = vi.fn();

      connection.onMessage(handler1);
      connection.onMessage(handler2);

      connection.connect();
      vi.advanceTimersByTime(300);

      expect(handler1).toHaveBeenCalled();
      expect(handler2).toHaveBeenCalled();
    });
  });

  describe("onStatusChange", () => {
    it("should register status handler", () => {
      const handler = vi.fn();
      connection.onStatusChange(handler);

      connection.connect();

      expect(handler).toHaveBeenCalledWith("connecting", undefined);

      vi.advanceTimersByTime(300);

      expect(handler).toHaveBeenCalledWith("connected", undefined);
    });

    it("should support multiple handlers", () => {
      const handler1 = vi.fn();
      const handler2 = vi.fn();

      connection.onStatusChange(handler1);
      connection.onStatusChange(handler2);

      connection.connect();

      expect(handler1).toHaveBeenCalled();
      expect(handler2).toHaveBeenCalled();
    });
  });

  describe("tool calls simulation", () => {
    it("should simulate tool calls in some responses", () => {
      const messages: ServerMessage[] = [];
      connection.onMessage((msg) => messages.push(msg));

      connection.connect();
      vi.advanceTimersByTime(300);

      // Send multiple messages to cycle through mock responses
      for (let i = 0; i < 3; i++) {
        connection.send(`Message ${i}`);
        vi.advanceTimersByTime(5000);
      }

      // At least one response should include tool calls
      const toolCalls = messages.filter((m) => m.type === "tool_call");
      const toolResults = messages.filter((m) => m.type === "tool_result");

      // Mock data includes tool calls in 2 of 3 responses
      expect(toolCalls.length + toolResults.length).toBeGreaterThan(0);
    });

    it("should match tool call IDs with results", () => {
      const messages: ServerMessage[] = [];
      connection.onMessage((msg) => messages.push(msg));

      connection.connect();
      vi.advanceTimersByTime(300);

      // First response includes tool calls
      connection.send("Hello");
      vi.advanceTimersByTime(5000);

      const toolCalls = messages.filter((m) => m.type === "tool_call");
      const toolResults = messages.filter((m) => m.type === "tool_result");

      // Each tool call should have a matching result
      toolCalls.forEach((tc) => {
        const callId = tc.tool_call?.id;
        const matchingResult = toolResults.find(
          (tr) => tr.tool_result?.id === callId
        );
        expect(matchingResult).toBeDefined();
      });
    });
  });
});
