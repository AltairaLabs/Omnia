/**
 * MemoryGraph — force-directed graph visualization of memories using @xyflow/react and elkjs.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import ELK from "elkjs/lib/elk.bundled.js";
import { MemoryNode, type MemoryNodeData } from "./memory-node";
import type { MemoryEntity } from "@/lib/data/types";

const elk = new ELK();

const nodeTypes = { memory: MemoryNode };

const ELK_OPTIONS = {
  "elk.algorithm": "org.eclipse.elk.force",
  "elk.force.iterations": "100",
  "elk.spacing.nodeNode": "60",
};

export function memoryToNode(memory: MemoryEntity, index: number): Node<MemoryNodeData> {
  const confidence = memory.confidence ?? 0.5;
  const size = 30 + confidence * 50;
  const category = memory.metadata?.consent_category as string | undefined;
  const rawLabel = memory.content.length > 20 ? memory.content.slice(0, 17) + "..." : memory.content;

  return {
    id: memory.id || `memory-${index}`,
    type: "memory",
    position: { x: 0, y: 0 },
    data: {
      label: rawLabel,
      category,
      confidence,
      memoryId: memory.id,
    },
    width: size,
    height: size,
  };
}

async function layoutNodes(nodes: Node<MemoryNodeData>[]): Promise<Node<MemoryNodeData>[]> {
  if (nodes.length === 0) return [];

  const elkGraph = {
    id: "root",
    layoutOptions: ELK_OPTIONS,
    children: nodes.map((node) => ({
      id: node.id,
      width: node.width ?? 60,
      height: node.height ?? 60,
    })),
    edges: [] as { id: string; sources: string[]; targets: string[] }[],
  };

  const layout = await elk.layout(elkGraph);

  return nodes.map((node) => {
    const elkNode = layout.children?.find((n) => n.id === node.id);
    return {
      ...node,
      position: {
        x: elkNode?.x ?? 0,
        y: elkNode?.y ?? 0,
      },
    };
  });
}

interface MemoryGraphProps {
  memories: MemoryEntity[];
  onNodeClick: (memory: MemoryEntity) => void;
}

export function MemoryGraph({ memories, onNodeClick }: MemoryGraphProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<MemoryNodeData>>([]);
  const [edges, , onEdgesChange] = useEdgesState<Edge>([]);
  const [isLayouting, setIsLayouting] = useState(true);
  const layoutingRef = useRef(true);

  useEffect(() => {
    let cancelled = false;
    layoutingRef.current = true;

    layoutNodes(memories.map((m, i) => memoryToNode(m, i))).then((positioned) => {
      if (!cancelled) {
        setNodes(positioned);
        setIsLayouting(false);
        layoutingRef.current = false;
      }
    });

    return () => {
      cancelled = true;
    };
  }, [memories, setNodes]);

  const handleNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      const nodeData = node.data as MemoryNodeData;
      const memory = memories.find((m) => m.id === nodeData.memoryId);
      if (memory) onNodeClick(memory);
    },
    [memories, onNodeClick]
  );

  return (
    <div data-testid="memory-graph" className="w-full h-[600px] rounded-lg border bg-background">
      {isLayouting ? (
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Arranging memories...
        </div>
      ) : (
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={handleNodeClick}
          nodeTypes={nodeTypes}
          fitView
          proOptions={{ hideAttribution: true }}
          minZoom={0.3}
          maxZoom={2}
        >
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
      )}
    </div>
  );
}
