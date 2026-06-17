/**
 * agent-topology-model — pure builder that turns an agent's static config into
 * a React Flow node/edge graph for the Overview architecture diagram.
 *
 * Kept separate from the rendering component (mirrors workload-graph/to-flow)
 * so the structure — facade(s) in front of a runtime that nests PromptPack,
 * Session and Memory — is unit-testable without mounting React Flow.
 *
 * Layout is deterministic (fixed positions): the graph is small and fixed, so
 * pixel-exact manual placement reads cleanly and avoids async-layout jitter.
 */

import type { Node, Edge } from "@xyflow/react";

export interface AgentTopologyFacade {
  type: string;
  port?: number;
}

export interface AgentTopologyInput {
  facades: AgentTopologyFacade[];
  framework?: { type?: string; version?: string };
  promptPack?: { name?: string; version?: string };
  session?: { type?: string; ttl?: string };
  memoryEnabled?: boolean;
  /** Used to deep-link the memory node to per-agent analytics. */
  agentName?: string;
}

export interface AgentTopologyGraph {
  nodes: Node[];
  edges: Edge[];
}

// Layout constants — a fixed, deterministic placement.
const FACADE_W = 130;
const FACADE_H = 60;
const FACADE_GAP = 18;
const RUNTIME_X = 180;
const RUNTIME_W = 330;
const RUNTIME_HEADER = 44;
const CHILD_W = 140;
const CHILD_H = 56;
const MEMORY_H = 44;
const PAD = 14;
const RUNTIME_H = RUNTIME_HEADER + CHILD_H + PAD + MEMORY_H + PAD;

// Non-interactive nodes: not draggable and not selectable. React Flow gives a
// node `pointer-events: none` (no pointer cursor, not focusable) only when it is
// neither selectable nor draggable — so these read as static diagram boxes. The
// Memory node opts out (it carries a real link) so its anchor stays clickable.
const STATIC = { draggable: false, selectable: false } as const;

export function buildAgentTopologyGraph(input: AgentTopologyInput): AgentTopologyGraph {
  const facades = input.facades.length > 0 ? input.facades : [{ type: "websocket" }];

  // Vertically center the facade stack against the runtime box.
  const stackHeight = facades.length * FACADE_H + (facades.length - 1) * FACADE_GAP;
  const stackTop = Math.max(0, (RUNTIME_H - stackHeight) / 2);

  const facadeNodes: Node[] = facades.map((f, i) => ({
    id: `facade-${i}`,
    type: "agentFacade",
    position: { x: 0, y: stackTop + i * (FACADE_H + FACADE_GAP) },
    data: { facadeType: f.type, port: f.port },
    style: { width: FACADE_W, height: FACADE_H },
    ...STATIC,
  }));

  const runtimeNode: Node = {
    id: "runtime",
    type: "agentRuntime",
    position: { x: RUNTIME_X, y: 0 },
    data: {
      frameworkType: input.framework?.type ?? "promptkit",
      frameworkVersion: input.framework?.version,
    },
    style: { width: RUNTIME_W, height: RUNTIME_H },
    ...STATIC,
  };

  const promptPackNode: Node = {
    id: "promptpack",
    type: "agentPromptPack",
    parentId: "runtime",
    extent: "parent",
    position: { x: PAD, y: RUNTIME_HEADER },
    data: {
      name: input.promptPack?.name,
      version: input.promptPack?.version,
    },
    style: { width: CHILD_W, height: CHILD_H },
    ...STATIC,
  };

  const sessionNode: Node = {
    id: "session",
    type: "agentSession",
    parentId: "runtime",
    extent: "parent",
    position: { x: PAD * 2 + CHILD_W, y: RUNTIME_HEADER },
    data: { sessionType: input.session?.type, ttl: input.session?.ttl },
    style: { width: CHILD_W, height: CHILD_H },
    ...STATIC,
  };

  const memoryNode: Node = {
    id: "memory",
    type: "agentMemory",
    parentId: "runtime",
    extent: "parent",
    position: { x: PAD, y: RUNTIME_HEADER + CHILD_H + PAD },
    data: { enabled: Boolean(input.memoryEnabled), agentName: input.agentName },
    style: { width: CHILD_W, height: MEMORY_H },
    draggable: false,
  };

  const edges: Edge[] = facades.map((_, i) => ({
    id: `facade-${i}->runtime`,
    source: `facade-${i}`,
    target: "runtime",
    type: "smoothstep",
  }));

  return {
    nodes: [...facadeNodes, runtimeNode, promptPackNode, sessionNode, memoryNode],
    edges,
  };
}
