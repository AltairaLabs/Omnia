import type { Node, Edge } from "@xyflow/react";
import ELK from "elkjs/lib/elk.bundled.js";
import type { AgentRuntime, PromptPack, ToolRegistry, Provider, ProviderType } from "@/types";
import type { NotesMap } from "@/lib/notes-storage";
import { getProviderColor } from "./provider-icons";

interface GraphData {
  nodes: Node[];
  edges: Edge[];
}

interface BuildGraphOptions {
  agents: AgentRuntime[];
  promptPacks: PromptPack[];
  toolRegistries: ToolRegistry[];
  providers: Provider[];
  onNodeClick?: (type: string, name: string, namespace: string) => void;
  notes?: NotesMap;
  onNoteEdit?: (type: string, namespace: string, name: string) => void;
  onNoteDelete?: (type: string, namespace: string, name: string) => void;
}

function getNoteForResource(notes: NotesMap | undefined, type: string, namespace: string, name: string): string | undefined {
  if (!notes) return undefined;
  const key = `${type}/${namespace}/${name}`;
  return notes[key]?.note;
}

// Node dimensions for layout
const NODE_WIDTH = 180;
const NODE_HEIGHT = 80;

// ELK instance (reused)
const elk = new ELK();

/**
 * Apply ELK layout to nodes and edges.
 * ELK provides better edge routing and crossing minimization than dagre.
 */
async function applyElkLayout(nodes: Node[], edges: Edge[]): Promise<Node[]> {
  if (nodes.length === 0) return nodes;

  const elkGraph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.spacing.nodeNode": "80",
      "elk.layered.spacing.nodeNodeBetweenLayers": "200",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.layered.mergeEdges": "true",
    },
    children: nodes.map((node) => ({
      id: node.id,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    })),
    edges: edges.map((edge) => ({
      id: edge.id,
      sources: [edge.source],
      targets: [edge.target],
    })),
  };

  const layoutedGraph = await elk.layout(elkGraph);

  return nodes.map((node) => {
    const elkNode = layoutedGraph.children?.find((n) => n.id === node.id);
    return {
      ...node,
      position: {
        x: elkNode?.x ?? 0,
        y: elkNode?.y ?? 0,
      },
    };
  });
}

/**
 * Apply simple column-based layout (fallback for sync use).
 * Groups nodes by type and arranges them in columns.
 */
function applySimpleLayout(nodes: Node[]): Node[] {
  // Group nodes by type
  const groups: Record<string, Node[]> = {};
  const typeOrder = ["agent", "promptPack", "toolRegistry", "tool", "provider"];

  nodes.forEach((node) => {
    const type = node.type || "unknown";
    if (!groups[type]) groups[type] = [];
    groups[type].push(node);
  });

  let xOffset = 50;
  const ySpacing = NODE_HEIGHT + 40;
  const xSpacing = NODE_WIDTH + 150;

  typeOrder.forEach((type) => {
    const typeNodes = groups[type];
    if (!typeNodes || typeNodes.length === 0) return;

    typeNodes.forEach((node, idx) => {
      node.position = {
        x: xOffset,
        y: 50 + idx * ySpacing,
      };
    });
    xOffset += xSpacing;
  });

  return nodes;
}

export function buildTopologyGraph({
  agents,
  promptPacks,
  toolRegistries,
  providers,
  onNodeClick,
  notes,
  onNoteEdit,
  onNoteDelete,
}: BuildGraphOptions): GraphData {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  // Create maps for lookups
  const promptPackMap = new Map(promptPacks.map((pp) => [`${pp.metadata.namespace}/${pp.metadata.name}`, pp]));
  const toolRegistryMap = new Map(toolRegistries.map((tr) => [`${tr.metadata.namespace}/${tr.metadata.name}`, tr]));
  const providerMap = new Map(providers.map((p) => [`${p.metadata.namespace}/${p.metadata.name}`, p]));

  // Track which resources are connected
  const connectedPromptPacks = new Set<string>();
  const connectedToolRegistries = new Set<string>();
  const connectedProviders = new Set<string>();
  // Track synthetic providers (inline provider configs)
  const syntheticProviders = new Map<string, { type: ProviderType; model?: string; baseURL?: string }>();

  // First pass: Create agent nodes and find connections
  agents.forEach((agent) => {
    const agentId = `agent-${agent.metadata.namespace}-${agent.metadata.name}`;

    nodes.push({
      id: agentId,
      type: "agent",
      position: { x: 0, y: 0 }, // Will be set by dagre
      data: {
        label: agent.metadata.name,
        namespace: agent.metadata.namespace,
        phase: agent.status?.phase,
        onClick: () => onNodeClick?.("agent", agent.metadata.name, agent.metadata.namespace || "default"),
        note: getNoteForResource(notes, "agent", agent.metadata.namespace || "default", agent.metadata.name),
        onNoteEdit,
        onNoteDelete,
      },
    });

    // Connect to PromptPack (PromptPackRef doesn't have namespace, uses agent's namespace)
    if (agent.spec.promptPackRef?.name) {
      const ppNamespace = agent.metadata.namespace || "default";
      const ppKey = `${ppNamespace}/${agent.spec.promptPackRef.name}`;
      connectedPromptPacks.add(ppKey);
    }

    // Connect to ToolRegistry
    if (agent.spec.toolRegistryRef?.name) {
      const trNamespace = agent.spec.toolRegistryRef.namespace || agent.metadata.namespace;
      const trKey = `${trNamespace}/${agent.spec.toolRegistryRef.name}`;
      connectedToolRegistries.add(trKey);
    }

    // Connect to Provider (via providerRef or inline provider)
    if (agent.spec.providerRef?.name) {
      const pNamespace = agent.spec.providerRef.namespace || agent.metadata.namespace || "default";
      const pKey = `${pNamespace}/${agent.spec.providerRef.name}`;
      connectedProviders.add(pKey);
    } else if (agent.spec.provider?.type) {
      // Inline provider - create synthetic provider node
      const syntheticKey = `synthetic-${agent.spec.provider.type}-${agent.spec.provider.model || "default"}`;
      syntheticProviders.set(syntheticKey, {
        type: agent.spec.provider.type,
        model: agent.spec.provider.model,
        baseURL: agent.spec.provider.baseURL,
      });
    }
  });

  // Create PromptPack nodes
  const allPromptPackKeys = new Set([
    ...connectedPromptPacks,
    ...promptPacks.map((pp) => `${pp.metadata.namespace}/${pp.metadata.name}`),
  ]);

  allPromptPackKeys.forEach((key) => {
    const pp = promptPackMap.get(key);
    if (!pp) return;

    const nodeId = `promptpack-${pp.metadata.namespace}-${pp.metadata.name}`;
    nodes.push({
      id: nodeId,
      type: "promptPack",
      position: { x: 0, y: 0 },
      data: {
        label: pp.metadata.name,
        namespace: pp.metadata.namespace,
        version: pp.status?.activeVersion || pp.spec.version,
        phase: pp.status?.phase,
        onClick: () => onNodeClick?.("promptpack", pp.metadata.name, pp.metadata.namespace || "default"),
        note: getNoteForResource(notes, "promptpack", pp.metadata.namespace || "default", pp.metadata.name),
        onNoteEdit,
        onNoteDelete,
      },
    });

    // Create edges from agents to this PromptPack
    agents.forEach((agent) => {
      if (
        agent.spec.promptPackRef?.name === pp.metadata.name &&
        (agent.metadata.namespace || "default") === (pp.metadata.namespace || "default")
      ) {
        edges.push({
          id: `edge-agent-${agent.metadata.namespace}-${agent.metadata.name}-to-${nodeId}`,
          source: `agent-${agent.metadata.namespace}-${agent.metadata.name}`,
          target: nodeId,
          type: "smoothstep",
          animated: true,
          style: { stroke: "#8b5cf6" },
          label: "uses",
          labelStyle: { fontSize: 10, fill: "#666" },
          labelBgStyle: { fill: "white", fillOpacity: 0.8 },
        });
      }
    });
  });

  // Create ToolRegistry and Tool nodes
  const allToolRegistryKeys = new Set([
    ...connectedToolRegistries,
    ...toolRegistries.map((tr) => `${tr.metadata.namespace}/${tr.metadata.name}`),
  ]);

  allToolRegistryKeys.forEach((key) => {
    const tr = toolRegistryMap.get(key);
    if (!tr) return;

    const nodeId = `toolregistry-${tr.metadata.namespace}-${tr.metadata.name}`;
    nodes.push({
      id: nodeId,
      type: "toolRegistry",
      position: { x: 0, y: 0 },
      data: {
        label: tr.metadata.name,
        namespace: tr.metadata.namespace,
        toolCount: tr.status?.discoveredToolsCount,
        phase: tr.status?.phase,
        onClick: () => onNodeClick?.("tools", tr.metadata.name, tr.metadata.namespace || "default"),
        note: getNoteForResource(notes, "toolregistry", tr.metadata.namespace || "default", tr.metadata.name),
        onNoteEdit,
        onNoteDelete,
      },
    });

    // Create edges from agents to this ToolRegistry
    agents.forEach((agent) => {
      if (
        agent.spec.toolRegistryRef?.name === tr.metadata.name &&
        (agent.spec.toolRegistryRef.namespace || agent.metadata.namespace) === tr.metadata.namespace
      ) {
        edges.push({
          id: `edge-agent-${agent.metadata.namespace}-${agent.metadata.name}-to-${nodeId}`,
          source: `agent-${agent.metadata.namespace}-${agent.metadata.name}`,
          target: nodeId,
          type: "smoothstep",
          animated: true,
          style: { stroke: "#f97316" },
          label: "uses",
          labelStyle: { fontSize: 10, fill: "#666" },
          labelBgStyle: { fill: "white", fillOpacity: 0.8 },
        });
      }
    });

    // Create tool nodes for this registry
    tr.status?.discoveredTools?.forEach((tool) => {
      const toolNodeId = `tool-${tr.metadata.namespace}-${tr.metadata.name}-${tool.name}`;
      nodes.push({
        id: toolNodeId,
        type: "tool",
        position: { x: 0, y: 0 },
        data: {
          label: tool.name,
          handlerType: tool.handlerName,
          status: tool.status,
          onClick: () => onNodeClick?.("tools", tr.metadata.name, tr.metadata.namespace || "default"),
        },
      });

      // Edge from ToolRegistry to Tool
      edges.push({
        id: `edge-${nodeId}-to-${toolNodeId}`,
        source: nodeId,
        target: toolNodeId,
        type: "smoothstep",
        style: { stroke: "#14b8a6" },
        label: "provides",
        labelStyle: { fontSize: 10, fill: "#666" },
        labelBgStyle: { fill: "white", fillOpacity: 0.8 },
      });
    });
  });

  // Create Provider nodes (from Provider CRDs)
  const allProviderKeys = new Set([
    ...connectedProviders,
    ...providers.map((p) => `${p.metadata.namespace}/${p.metadata.name}`),
  ]);

  allProviderKeys.forEach((key) => {
    const provider = providerMap.get(key);
    if (!provider) return;

    const nodeId = `provider-${provider.metadata.namespace}-${provider.metadata.name}`;
    const providerType = provider.spec.type as ProviderType;

    nodes.push({
      id: nodeId,
      type: "provider",
      position: { x: 0, y: 0 },
      data: {
        label: provider.metadata.name,
        namespace: provider.metadata.namespace,
        providerType,
        model: provider.spec.model,
        baseURL: provider.spec.baseURL,
        phase: provider.status?.phase,
        onClick: () => onNodeClick?.("provider", provider.metadata.name, provider.metadata.namespace || "default"),
      },
    });

    // Create edges from agents to this Provider
    agents.forEach((agent) => {
      if (
        agent.spec.providerRef?.name === provider.metadata.name &&
        (agent.spec.providerRef.namespace || agent.metadata.namespace || "default") === (provider.metadata.namespace || "default")
      ) {
        edges.push({
          id: `edge-agent-${agent.metadata.namespace}-${agent.metadata.name}-to-${nodeId}`,
          source: `agent-${agent.metadata.namespace}-${agent.metadata.name}`,
          target: nodeId,
          type: "smoothstep",
          animated: true,
          style: { stroke: getProviderColor(providerType) },
          label: "powered by",
          labelStyle: { fontSize: 10, fill: "#666" },
          labelBgStyle: { fill: "white", fillOpacity: 0.8 },
        });
      }
    });
  });

  // Create synthetic provider nodes (for inline provider configs)
  syntheticProviders.forEach((config, syntheticKey) => {
    const nodeId = `provider-${syntheticKey}`;

    nodes.push({
      id: nodeId,
      type: "provider",
      position: { x: 0, y: 0 },
      data: {
        label: config.type,
        namespace: "(inline)",
        providerType: config.type,
        model: config.model,
        baseURL: config.baseURL,
        phase: "Ready", // Inline providers don't have status
      },
    });

    // Create edges from agents using this inline provider
    agents.forEach((agent) => {
      if (
        !agent.spec.providerRef &&
        agent.spec.provider?.type === config.type &&
        (agent.spec.provider.model || "default") === (config.model || "default")
      ) {
        edges.push({
          id: `edge-agent-${agent.metadata.namespace}-${agent.metadata.name}-to-${nodeId}`,
          source: `agent-${agent.metadata.namespace}-${agent.metadata.name}`,
          target: nodeId,
          type: "smoothstep",
          animated: true,
          style: { stroke: getProviderColor(config.type) },
          label: "powered by",
          labelStyle: { fontSize: 10, fill: "#666" },
          labelBgStyle: { fill: "white", fillOpacity: 0.8 },
        });
      }
    });
  });

  // Apply simple layout initially (sync) - will be replaced by ELK layout
  const layoutedNodes = applySimpleLayout(nodes);

  return { nodes: layoutedNodes, edges };
}

/**
 * Apply ELK layout to an existing graph.
 * Call this after buildTopologyGraph to get better edge routing.
 */
export async function applyLayoutToGraph(graph: GraphData): Promise<GraphData> {
  const layoutedNodes = await applyElkLayout(graph.nodes, graph.edges);
  return { nodes: layoutedNodes, edges: graph.edges };
}
