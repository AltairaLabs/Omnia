"use client";

/**
 * AgentTopology — the agent Overview architecture diagram.
 *
 * Renders facade(s) sitting in front of the runtime, with PromptPack, Session
 * and Memory nested inside the runtime. Built on @xyflow/react (same stack as
 * topology/ and workload-graph/) but locked down to a static diagram: no drag,
 * no pan, no zoom, fitView only — so it reads as a diagram, not a canvas.
 *
 * Graph structure/layout lives in agent-topology-model.
 */

import { useMemo } from "react";
import { ReactFlow } from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { agentTopologyNodeTypes } from "./agent-topology-nodes";
import { buildAgentTopologyGraph, type AgentTopologyFacade } from "./agent-topology-model";

interface AgentTopologyProps {
  agentName: string;
  facades: AgentTopologyFacade[];
  framework?: { type?: string; version?: string };
  promptPack?: { name?: string; version?: string };
  session?: { type?: string; ttl?: string };
  memoryEnabled?: boolean;
}

export function AgentTopology({
  agentName,
  facades,
  framework,
  promptPack,
  session,
  memoryEnabled,
}: Readonly<AgentTopologyProps>) {
  const { nodes, edges } = useMemo(
    () =>
      buildAgentTopologyGraph({
        agentName,
        facades,
        framework,
        promptPack,
        session,
        memoryEnabled,
      }),
    [agentName, facades, framework, promptPack, session, memoryEnabled],
  );

  return (
    // Inline height is required: React Flow measures this container and renders
    // nothing if it computes to 0 (error#004). A Tailwind arbitrary class is not
    // reliable here — match the working topology-graph and set it inline.
    <div style={{ width: "100%", height: 200 }} className="rounded-lg border bg-card/40">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={agentTopologyNodeTypes}
        fitView
        fitViewOptions={{ padding: 0.06 }}
        maxZoom={1}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable={false}
        elementsSelectable
        panOnDrag={false}
        panOnScroll={false}
        zoomOnScroll={false}
        zoomOnPinch={false}
        zoomOnDoubleClick={false}
        proOptions={{ hideAttribution: true }}
      />
    </div>
  );
}
