import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { NodeSummaryCard, type SelectedNode } from "./node-summary-card";
import type { AgentRuntime, PromptPack, ToolRegistry, Provider } from "@/types";

// Mock hooks
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: { name: "demo" } })),
}));
vi.mock("@/hooks/agents", () => ({
  useAgentCost: vi.fn(() => ({ data: null })),
}));
vi.mock("@/hooks/resources", () => ({
  useProvider: vi.fn(() => ({ data: null })),
  useProviderMetrics: vi.fn(() => ({ data: null })),
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

// Helper to create mock agent
function createMockAgent(name: string, namespace: string): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, namespace },
    spec: {
      promptPackRef: { name: "test-pack" },
      facades: [{ type: "websocket", port: 8080 }],
      runtime: { replicas: 1 },
    },
    status: { phase: "Running" },
  } as AgentRuntime;
}

// Helper to create mock provider
function createMockProvider(name: string, namespace: string): Provider {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: { name, namespace },
    spec: {
      type: "claude",
      model: "claude-sonnet-4-20250514",
      credential: { secretRef: { name: "test-secret" } },
    },
    status: { phase: "Ready" },
  };
}

// Helper to create mock prompt pack. `metadataName` mirrors the real
// deterministic `pp-<hash>` object name (#1837); `packName` is the logical
// name the UI should display, which may differ from `metadataName`.
function createMockPromptPack(metadataName: string, namespace: string, packName: string = metadataName): PromptPack {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name: metadataName, namespace },
    spec: {
      packName,
      source: { type: "configmap", configMapRef: { name: "test" } },
      version: "1.0.0",
    },
    status: { phase: "Active", activeVersion: "1.0.0" },
  } as PromptPack;
}

// Helper to create mock tool registry
function createMockToolRegistry(name: string, namespace: string): ToolRegistry {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: { name, namespace },
    spec: { handlers: [] },
    status: { phase: "Ready", discoveredToolsCount: 5 },
  } as ToolRegistry;
}

describe("NodeSummaryCard", () => {
  const mockOnClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("agent cards", () => {
    it("renders agent summary card when agent is selected", () => {
      const agent = createMockAgent("test-agent", "default");
      const selectedNode: SelectedNode = {
        type: "agent",
        name: "test-agent",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[agent]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("test-agent")).toBeInTheDocument();
      expect(screen.getByText("default")).toBeInTheDocument();
      expect(screen.getByText("Agent")).toBeInTheDocument();
    });

    it("calls onClose when close button is clicked", () => {
      const agent = createMockAgent("test-agent", "default");
      const selectedNode: SelectedNode = {
        type: "agent",
        name: "test-agent",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[agent]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      // Find the close button by its accessible role and the X icon
      const closeButtons = screen.getAllByRole("button");
      // The close button is the first one (in the header)
      const closeButton = closeButtons[0];
      fireEvent.click(closeButton);
      expect(mockOnClose).toHaveBeenCalledTimes(1);
    });

    it("renders view details link for agent", () => {
      const agent = createMockAgent("test-agent", "ns1");
      const selectedNode: SelectedNode = {
        type: "agent",
        name: "test-agent",
        namespace: "ns1",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[agent]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      const link = screen.getByRole("link", { name: /view details/i });
      expect(link).toHaveAttribute("href", "/agents/test-agent?namespace=ns1");
    });
  });

  describe("provider cards", () => {
    it("renders provider summary card when provider is selected", () => {
      const provider = createMockProvider("test-provider", "default");
      const selectedNode: SelectedNode = {
        type: "provider",
        name: "test-provider",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[provider]}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("test-provider")).toBeInTheDocument();
      expect(screen.getByText("Provider")).toBeInTheDocument();
    });

    it("renders view details link for provider", () => {
      const provider = createMockProvider("my-provider", "ns2");
      const selectedNode: SelectedNode = {
        type: "provider",
        name: "my-provider",
        namespace: "ns2",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[provider]}
          onClose={mockOnClose}
        />
      );

      const link = screen.getByRole("link", { name: /view details/i });
      expect(link).toHaveAttribute("href", "/providers/my-provider?namespace=ns2");
    });
  });

  describe("prompt pack cards", () => {
    it("renders prompt pack summary card showing the logical packName, not the hash object name", () => {
      // metadata.name is the deterministic pp-<hash> object name; the card
      // must display spec.packName instead (#1860).
      const promptPack = createMockPromptPack("pp-abc123hash", "default", "test-pack");
      const selectedNode: SelectedNode = {
        type: "promptpack",
        name: "pp-abc123hash",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[promptPack]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("test-pack")).toBeInTheDocument();
      expect(screen.queryByText("pp-abc123hash")).not.toBeInTheDocument();
      expect(screen.getByText("PromptPack")).toBeInTheDocument();
    });

    it("renders view details link for prompt pack using the logical packName", () => {
      const promptPack = createMockPromptPack("pp-def456hash", "ns3", "my-pack");
      const selectedNode: SelectedNode = {
        type: "promptpack",
        name: "pp-def456hash",
        namespace: "ns3",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[promptPack]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      const link = screen.getByRole("link", { name: /view details/i });
      expect(link).toHaveAttribute("href", "/promptpacks/my-pack?namespace=ns3");
    });
  });

  describe("tool registry cards", () => {
    it("renders tool registry summary card when tool registry is selected", () => {
      const toolRegistry = createMockToolRegistry("test-tools", "default");
      const selectedNode: SelectedNode = {
        type: "tools",
        name: "test-tools",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[]}
          toolRegistries={[toolRegistry]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("test-tools")).toBeInTheDocument();
      expect(screen.getByText("ToolRegistry")).toBeInTheDocument();
    });

    it("shows tool count in tool registry card", () => {
      const toolRegistry = createMockToolRegistry("test-tools", "default");
      const selectedNode: SelectedNode = {
        type: "tools",
        name: "test-tools",
        namespace: "default",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[]}
          toolRegistries={[toolRegistry]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("5")).toBeInTheDocument(); // discoveredToolsCount
    });
  });

  describe("edge cases", () => {
    it("returns null when resource is not found", () => {
      const selectedNode: SelectedNode = {
        type: "agent",
        name: "nonexistent",
        namespace: "default",
      };

      const { container } = render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      expect(container.firstChild).toBeNull();
    });

    it("matches resource by both name and namespace", () => {
      const agent1 = createMockAgent("test-agent", "ns1");
      const agent2 = createMockAgent("test-agent", "ns2");
      const selectedNode: SelectedNode = {
        type: "agent",
        name: "test-agent",
        namespace: "ns2",
      };

      render(
        <NodeSummaryCard
          selectedNode={selectedNode}
          agents={[agent1, agent2]}
          promptPacks={[]}
          toolRegistries={[]}
          providers={[]}
          onClose={mockOnClose}
        />
      );

      // Should find ns2, not ns1
      expect(screen.getByText("ns2")).toBeInTheDocument();
    });
  });
});
