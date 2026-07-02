"use client";

import { ReactFlow, Background, type Node, type Edge } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useFlowColorMode } from "@/lib/flow/use-color-mode";

// Nodes tinted with category tokens (var()) so a brand-preset switch re-themes
// them. Loaded via next/dynamic { ssr: false } from the page — React Flow uses
// browser-only APIs (ResizeObserver / getBoundingClientRect) and does not
// server-render, so it must be mounted client-side only.
const FLOW_NODES: Node[] = [
  { id: "a", position: { x: 0, y: 20 }, data: { label: "Agent" }, style: { background: "var(--category-1)", color: "#fff", border: "none" } },
  { id: "b", position: { x: 180, y: 0 }, data: { label: "Skill" }, style: { background: "var(--category-2)", color: "#fff", border: "none" } },
  { id: "c", position: { x: 180, y: 80 }, data: { label: "Tool" }, style: { background: "var(--category-4)", color: "#fff", border: "none" } },
];
const FLOW_EDGES: Edge[] = [
  { id: "a-b", source: "a", target: "b" },
  { id: "a-c", source: "a", target: "c" },
];

export default function ThemeFlow() {
  const colorMode = useFlowColorMode();
  return (
    <ReactFlow
      nodes={FLOW_NODES}
      edges={FLOW_EDGES}
      colorMode={colorMode}
      fitView
      proOptions={{ hideAttribution: true }}
    >
      <Background />
    </ReactFlow>
  );
}
