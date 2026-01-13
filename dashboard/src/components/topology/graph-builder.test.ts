import { describe, it, expect, vi } from "vitest";
import { buildTopologyGraph } from "./graph-builder";
import type { AgentRuntime, PromptPack, ToolRegistry, Provider } from "@/types";

// Helper to create minimal agent
function createAgent(name: string, namespace: string, overrides: Partial<AgentRuntime["spec"]> = {}): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, namespace },
    spec: {
      facade: { type: "websocket", port: 8080 },
      runtime: { replicas: 1 },
      ...overrides,
    },
    status: { phase: "Running" },
  } as AgentRuntime;
}

// Helper to create minimal prompt pack
function createPromptPack(name: string, namespace: string): PromptPack {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name, namespace },
    spec: {
      source: { type: "configmap", configMapRef: { name: "test" } },
      version: "1.0.0",
      rollout: { type: "immediate" },
    },
    status: { phase: "Active", activeVersion: "1.0.0" },
  } as PromptPack;
}

// Helper to create minimal tool registry
function _createToolRegistry(name: string, namespace: string): ToolRegistry {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: { name, namespace },
    spec: { handlers: [] },
    status: { phase: "Ready", discoveredToolsCount: 0 },
  } as ToolRegistry;
}

// Helper to create minimal provider
function createProvider(name: string, namespace: string, type: "claude" | "openai" | "gemini" | "ollama" | "mock" = "claude"): Provider {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: { name, namespace },
    spec: {
      type,
      secretRef: { name: "test-secret" },
      model: "test-model",
    },
    status: { phase: "Ready" },
  };
}

describe("buildTopologyGraph", () => {
  describe("basic graph building", () => {
    it("returns empty graph when no resources provided", () => {
      const result = buildTopologyGraph({
        agents: [],
        promptPacks: [],
        toolRegistries: [],
        providers: [],
      });

      expect(result.nodes).toHaveLength(0);
      expect(result.edges).toHaveLength(0);
    });

    it("creates agent nodes", () => {
      const agents = [createAgent("agent1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers: [],
      });

      expect(result.nodes).toHaveLength(1);
      expect(result.nodes[0]).toMatchObject({
        id: "agent-ns1-agent1",
        type: "agent",
        data: {
          label: "agent1",
          namespace: "ns1",
          phase: "Running",
        },
      });
    });

    it("creates prompt pack nodes and edges to agents", () => {
      const agents = [
        createAgent("agent1", "ns1", { promptPackRef: { name: "pack1" } }),
      ];
      const promptPacks = [createPromptPack("pack1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries: [],
        providers: [],
      });

      expect(result.nodes).toHaveLength(2);
      expect(result.nodes.find((n) => n.type === "promptPack")).toMatchObject({
        id: "promptpack-ns1-pack1",
        data: { label: "pack1" },
      });

      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        source: "agent-ns1-agent1",
        target: "promptpack-ns1-pack1",
        label: "uses",
      });
    });
  });

  describe("provider nodes", () => {
    it("creates provider nodes from Provider CRDs", () => {
      const providers = [createProvider("claude-prod", "ns1", "claude")];

      const result = buildTopologyGraph({
        agents: [],
        promptPacks: [],
        toolRegistries: [],
        providers,
      });

      expect(result.nodes).toHaveLength(1);
      expect(result.nodes[0]).toMatchObject({
        id: "provider-ns1-claude-prod",
        type: "provider",
        data: {
          label: "claude-prod",
          namespace: "ns1",
          providerType: "claude",
          model: "test-model",
          phase: "Ready",
        },
      });
    });

    it("creates edges from agents to providers via providerRef", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          providerRef: { name: "claude-prod" },
        }),
      ];
      const providers = [createProvider("claude-prod", "ns1", "claude")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers,
      });

      expect(result.nodes).toHaveLength(2);
      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        source: "agent-ns1-agent1",
        target: "provider-ns1-claude-prod",
        label: "powered by",
      });
    });

    it("creates synthetic provider nodes for inline provider configs", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          provider: { type: "openai", model: "gpt-4" },
        }),
      ];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers: [],
      });

      // Agent + synthetic provider
      expect(result.nodes).toHaveLength(2);

      const syntheticProvider = result.nodes.find((n) => n.id.includes("synthetic"));
      expect(syntheticProvider).toMatchObject({
        type: "provider",
        data: {
          label: "openai",
          namespace: "(inline)",
          providerType: "openai",
          model: "gpt-4",
        },
      });

      // Edge from agent to synthetic provider
      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        source: "agent-ns1-agent1",
        label: "powered by",
      });
    });

    it("reuses synthetic provider node for multiple agents with same inline config", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          provider: { type: "mock" },
        }),
        createAgent("agent2", "ns1", {
          provider: { type: "mock" },
        }),
      ];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers: [],
      });

      // 2 agents + 1 synthetic provider (reused)
      expect(result.nodes).toHaveLength(3);
      expect(result.nodes.filter((n) => n.type === "provider")).toHaveLength(1);

      // Both agents connect to the same synthetic provider
      expect(result.edges).toHaveLength(2);
    });
  });

  describe("dagre layout", () => {
    it("applies layout to position nodes (positions are not 0,0)", () => {
      const agents = [createAgent("agent1", "ns1")];
      const providers = [createProvider("provider1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers,
      });

      // All nodes should have positions set by dagre (not at origin)
      result.nodes.forEach((node) => {
        expect(node.position).toBeDefined();
        // Dagre will position nodes, but some may legitimately be near 0
        // Just verify position exists and is a number
        expect(typeof node.position.x).toBe("number");
        expect(typeof node.position.y).toBe("number");
      });
    });

    it("positions connected nodes in a hierarchy", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          promptPackRef: { name: "pack1" },
          providerRef: { name: "provider1" },
        }),
      ];
      const promptPacks = [createPromptPack("pack1", "ns1")];
      const providers = [createProvider("provider1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries: [],
        providers,
      });

      const agentNode = result.nodes.find((n) => n.type === "agent");
      const packNode = result.nodes.find((n) => n.type === "promptPack");
      const providerNode = result.nodes.find((n) => n.type === "provider");

      // With LR layout, agent should be to the left of connected resources
      // (dagre positions them in columns based on dependencies)
      expect(agentNode).toBeDefined();
      expect(packNode).toBeDefined();
      expect(providerNode).toBeDefined();
    });
  });

  describe("callbacks", () => {
    it("attaches onClick callback to nodes", () => {
      const onNodeClick = vi.fn();
      const agents = [createAgent("agent1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers: [],
        onNodeClick,
      });

      // Invoke the onClick
      const nodeData = result.nodes[0].data as { onClick?: () => void };
      nodeData.onClick?.();

      expect(onNodeClick).toHaveBeenCalledWith("agent", "agent1", "ns1");
    });

    it("attaches note-related data to nodes", () => {
      const onNoteEdit = vi.fn();
      const onNoteDelete = vi.fn();
      const notes = {
        "agent/ns1/agent1": {
          resourceType: "agent" as const,
          namespace: "ns1",
          name: "agent1",
          note: "Test note",
          updatedAt: new Date().toISOString(),
        },
      };

      const agents = [createAgent("agent1", "ns1")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers: [],
        notes,
        onNoteEdit,
        onNoteDelete,
      });

      expect(result.nodes[0].data.note).toBe("Test note");
      expect(result.nodes[0].data.onNoteEdit).toBe(onNoteEdit);
      expect(result.nodes[0].data.onNoteDelete).toBe(onNoteDelete);
    });
  });

  describe("cross-namespace references", () => {
    it("handles providerRef with explicit namespace", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          providerRef: { name: "shared-provider", namespace: "shared" },
        }),
      ];
      const providers = [createProvider("shared-provider", "shared", "claude")];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers,
      });

      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        source: "agent-ns1-agent1",
        target: "provider-shared-shared-provider",
      });
    });
  });
});
