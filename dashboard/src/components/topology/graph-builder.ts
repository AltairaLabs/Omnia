import type { Node, Edge } from "@xyflow/react";
import type { AgentRuntime, PromptPack, ToolRegistry } from "@/types";
import type { NotesMap } from "@/lib/notes-storage";

interface GraphData {
  nodes: Node[];
  edges: Edge[];
}

interface BuildGraphOptions {
  agents: AgentRuntime[];
  promptPacks: PromptPack[];
  toolRegistries: ToolRegistry[];
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

// Layout constants
const COLUMN_GAP = 280;
const ROW_GAP = 100;
const INITIAL_X = 50;
const INITIAL_Y = 50;

export function buildTopologyGraph({
  agents,
  promptPacks,
  toolRegistries,
  onNodeClick,
  notes,
  onNoteEdit,
  onNoteDelete,
}: BuildGraphOptions): GraphData {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  // Track positions
  let agentY = INITIAL_Y;
  let toolY = INITIAL_Y;

  // Column positions
  const agentX = INITIAL_X;
  const promptPackX = INITIAL_X + COLUMN_GAP;
  const toolRegistryX = INITIAL_X + COLUMN_GAP;
  const toolX = INITIAL_X + COLUMN_GAP * 2;

  // Create maps for lookups
  const promptPackMap = new Map(promptPacks.map((pp) => [`${pp.metadata.namespace}/${pp.metadata.name}`, pp]));
  const toolRegistryMap = new Map(toolRegistries.map((tr) => [`${tr.metadata.namespace}/${tr.metadata.name}`, tr]));

  // Track which resources are connected
  const connectedPromptPacks = new Set<string>();
  const connectedToolRegistries = new Set<string>();

  // First pass: Create agent nodes and find connections
  agents.forEach((agent) => {
    const agentId = `agent-${agent.metadata.namespace}-${agent.metadata.name}`;

    nodes.push({
      id: agentId,
      type: "agent",
      position: { x: agentX, y: agentY },
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

    agentY += ROW_GAP;
  });

  // Calculate vertical offset for PromptPacks and ToolRegistries
  // Position them in the middle column, stacked vertically
  const middleColumnItems: Array<{ type: "promptPack" | "toolRegistry"; key: string }> = [];

  connectedPromptPacks.forEach((key) => {
    middleColumnItems.push({ type: "promptPack", key });
  });

  connectedToolRegistries.forEach((key) => {
    middleColumnItems.push({ type: "toolRegistry", key });
  });

  // Add unconnected PromptPacks
  promptPacks.forEach((pp) => {
    const key = `${pp.metadata.namespace}/${pp.metadata.name}`;
    if (!connectedPromptPacks.has(key)) {
      middleColumnItems.push({ type: "promptPack", key });
    }
  });

  // Add unconnected ToolRegistries
  toolRegistries.forEach((tr) => {
    const key = `${tr.metadata.namespace}/${tr.metadata.name}`;
    if (!connectedToolRegistries.has(key)) {
      middleColumnItems.push({ type: "toolRegistry", key });
    }
  });

  // Create PromptPack and ToolRegistry nodes
  let middleY = INITIAL_Y;
  middleColumnItems.forEach((item) => {
    if (item.type === "promptPack") {
      const pp = promptPackMap.get(item.key);
      if (!pp) return;

      const nodeId = `promptpack-${pp.metadata.namespace}-${pp.metadata.name}`;
      nodes.push({
        id: nodeId,
        type: "promptPack",
        position: { x: promptPackX, y: middleY },
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

      middleY += ROW_GAP;
    } else {
      const tr = toolRegistryMap.get(item.key);
      if (!tr) return;

      const nodeId = `toolregistry-${tr.metadata.namespace}-${tr.metadata.name}`;
      nodes.push({
        id: nodeId,
        type: "toolRegistry",
        position: { x: toolRegistryX, y: middleY },
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
          position: { x: toolX, y: toolY },
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

        toolY += ROW_GAP * 0.8;
      });

      middleY += ROW_GAP;
    }
  });

  return { nodes, edges };
}
