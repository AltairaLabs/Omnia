"use client";

import { useCallback, useMemo } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { nodeTypes } from "./nodes";
import { buildTopologyGraph } from "./graph-builder";
import type { AgentRuntime, PromptPack, ToolRegistry } from "@/types";

interface TopologyGraphProps {
  agents: AgentRuntime[];
  promptPacks: PromptPack[];
  toolRegistries: ToolRegistry[];
  onNodeClick?: (type: string, name: string, namespace: string) => void;
  className?: string;
}

export function TopologyGraph({
  agents,
  promptPacks,
  toolRegistries,
  onNodeClick,
  className,
}: TopologyGraphProps) {
  // Build the initial graph
  const initialGraph = useMemo(
    () =>
      buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries,
        onNodeClick,
      }),
    [agents, promptPacks, toolRegistries, onNodeClick]
  );

  const [nodes, , onNodesChange] = useNodesState(initialGraph.nodes);
  const [edges, , onEdgesChange] = useEdgesState(initialGraph.edges);

  // Custom mini-map node color
  const nodeColor = useCallback((node: Node) => {
    switch (node.type) {
      case "agent":
        return "#3b82f6"; // blue
      case "promptPack":
        return "#8b5cf6"; // purple
      case "toolRegistry":
        return "#f97316"; // orange
      case "tool":
        return "#14b8a6"; // teal
      case "prompt":
        return "#a855f7"; // violet
      default:
        return "#6b7280"; // gray
    }
  }, []);

  return (
    <div className={className} style={{ width: "100%", height: "100%" }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={2}
        defaultEdgeOptions={{
          type: "smoothstep",
        }}
      >
        <Controls />
        <MiniMap
          nodeColor={nodeColor}
          nodeStrokeWidth={3}
          zoomable
          pannable
        />
        <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
      </ReactFlow>
    </div>
  );
}
