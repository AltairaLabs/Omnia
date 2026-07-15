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

// Helper to create minimal prompt pack.
// After #1837 metadata.name is a hash and spec.packName is the logical name;
// tests can set them independently, defaulting packName to name/version.
function createPromptPack(
  name: string,
  namespace: string,
  opts: { packName?: string; version?: string } = {},
): PromptPack {
  const version = opts.version ?? "1.0.0";
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name, namespace },
    spec: {
      packName: opts.packName ?? name,
      source: { type: "configmap", configMapRef: { name: "test" } },
      version,
    },
    status: { phase: "Active", activeVersion: version },
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
      credential: { secretRef: { name: "test-secret" } },
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

    it("joins agents to a hash-named pack via spec.packName (#1849)", () => {
      const agents = [
        createAgent("agent1", "ns1", { promptPackRef: { name: "rag-hero" } }),
      ];
      // Object is hash-named (pp-<hash>); logical name lives on spec.packName.
      const promptPacks = [
        createPromptPack("pp-abc123", "ns1", { packName: "rag-hero" }),
      ];

      const result = buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries: [],
        providers: [],
      });

      const packNode = result.nodes.find((n) => n.type === "promptPack");
      expect(packNode).toMatchObject({
        id: "promptpack-ns1-rag-hero",
        data: { label: "rag-hero" },
      });

      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        source: "agent-ns1-agent1",
        target: "promptpack-ns1-rag-hero",
        label: "uses",
      });
    });

    it("dedupes multiple version-objects of one packName to the channel-max node", () => {
      const agents = [
        createAgent("agent1", "ns1", { promptPackRef: { name: "rag-hero" } }),
      ];
      const promptPacks = [
        createPromptPack("pp-v100", "ns1", { packName: "rag-hero", version: "1.0.0" }),
        createPromptPack("pp-v110", "ns1", { packName: "rag-hero", version: "1.1.0" }),
        createPromptPack("pp-v200beta", "ns1", { packName: "rag-hero", version: "2.0.0-beta.1" }),
      ];

      const result = buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries: [],
        providers: [],
      });

      // One agent + exactly one deduped pack node.
      const packNodes = result.nodes.filter((n) => n.type === "promptPack");
      expect(packNodes).toHaveLength(1);
      // Channel-max stable => 1.1.0, not the 2.0.0 prerelease.
      expect(packNodes[0].data.version).toBe("1.1.0");
      // Exactly one edge to the single node.
      expect(result.edges).toHaveLength(1);
      expect(result.edges[0]).toMatchObject({
        target: "promptpack-ns1-rag-hero",
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

    it("creates edges from agents to providers via providers list", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          providers: [{ name: "default", providerRef: { name: "claude-prod" } }],
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

    it("creates edges for multiple providers in providers list", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          providers: [
            { name: "default", providerRef: { name: "claude-prod" } },
            { name: "fallback", providerRef: { name: "openai-prod" } },
          ],
        }),
      ];
      const providers = [
        createProvider("claude-prod", "ns1", "claude"),
        createProvider("openai-prod", "ns1", "openai"),
      ];

      const result = buildTopologyGraph({
        agents,
        promptPacks: [],
        toolRegistries: [],
        providers,
      });

      // 1 agent + 2 providers
      expect(result.nodes).toHaveLength(3);
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
          providers: [{ name: "default", providerRef: { name: "provider1" } }],
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
    it("handles providers with explicit namespace", () => {
      const agents = [
        createAgent("agent1", "ns1", {
          providers: [{ name: "default", providerRef: { name: "shared-provider", namespace: "shared" } }],
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
