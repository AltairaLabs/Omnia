import type { PromptPackContent, PromptVariable } from "@/lib/data/types";
import type { WorkloadNode, WorkloadEdge } from "./types";

export function collectVariables(content: PromptPackContent): PromptVariable[] {
  const seen = new Map<string, PromptVariable>();
  for (const p of Object.values(content.prompts ?? {})) {
    for (const v of p.variables ?? []) {
      if (!seen.has(v.name)) seen.set(v.name, v);
    }
  }
  return [...seen.values()];
}

export function variableNodesAndEdges(
  vars: PromptVariable[],
  targetId: string | undefined,
): { nodes: WorkloadNode[]; edges: WorkloadEdge[] } {
  const nodes: WorkloadNode[] = [];
  const edges: WorkloadEdge[] = [];
  for (const v of vars) {
    const id = `var:${v.name}`;
    nodes.push({
      id,
      kind: "variable",
      label: v.name,
      badges: [],
      detail: {
        varType: v.type,
        required: v.required,
        example: v.example,
        values: v.values,
        description: v.description,
      },
    });
    if (targetId) {
      edges.push({ id: `${id}-->${targetId}`, source: id, target: targetId, style: "data" });
    }
  }
  return { nodes, edges };
}

export function pseudoStateNodesAndEdges(
  content: PromptPackContent,
): { nodes: WorkloadNode[]; edges: WorkloadEdge[] } {
  const wf = content.workflow;
  if (!wf) return { nodes: [], edges: [] };
  const nodes: WorkloadNode[] = [{ id: "initial", kind: "initial", label: "", badges: [], detail: {} }];
  const edges: WorkloadEdge[] = [
    { id: `initial-->${wf.entry}`, source: "initial", target: wf.entry, style: "normal" },
  ];
  const terminals = Object.entries(wf.states)
    .filter(([, s]) => s.terminal === true)
    .map(([id]) => id);
  if (terminals.length > 0) {
    nodes.push({ id: "final", kind: "final", label: "", badges: [], detail: {} });
    for (const t of terminals) {
      edges.push({ id: `${t}-->final`, source: t, target: "final", style: "normal" });
    }
  }
  return { nodes, edges };
}

function escapeRe(s: string): string {
  return s.replaceAll(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function templateRefsArtifact(template: string | undefined, name: string): boolean {
  if (!template) return false;
  return new RegExp(`\\{\\{\\s*artifacts\\.${escapeRe(name)}\\s*\\}\\}`).test(template);
}

export function artifactNodesAndEdges(
  content: PromptPackContent,
): { nodes: WorkloadNode[]; edges: WorkloadEdge[] } {
  const wf = content.workflow;
  if (!wf) return { nodes: [], edges: [] };
  const prompts = content.prompts ?? {};
  const states = Object.entries(wf.states);

  // declarations: name -> { mode, type, producers[] }
  const decls = new Map<string, { mode?: string; type?: string; producers: string[] }>();
  for (const [stateId, state] of states) {
    for (const [name, art] of Object.entries(state.artifacts ?? {})) {
      const e = decls.get(name) ?? { mode: art.mode, type: art.type, producers: [] };
      e.producers.push(stateId);
      decls.set(name, e);
    }
  }

  // also include names referenced in templates but never declared
  const names = new Set(decls.keys());
  for (const [, state] of states) {
    const tpl = prompts[state.prompt_task]?.system_template ?? "";
    for (const g of tpl.matchAll(/\{\{\s*artifacts\.([a-zA-Z0-9_-]+)\s*\}\}/g)) {
      names.add(g[1]);
    }
  }

  const nodes: WorkloadNode[] = [];
  const edges: WorkloadEdge[] = [];
  for (const name of names) {
    const decl = decls.get(name);
    const producers = decl?.producers ?? [];
    const consumers = states
      .filter(([, s]) => templateRefsArtifact(prompts[s.prompt_task]?.system_template, name))
      .map(([id]) => id);
    const id = `artifact:${name}`;
    nodes.push({
      id,
      kind: "artifact",
      label: name,
      badges: [],
      resolution: producers.length === 0 ? "unresolved" : undefined,
      detail: { artifactMode: decl?.mode, artifactType: decl?.type, producers, consumers },
    });
    for (const p of producers) {
      edges.push({ id: `${p}--art-->${id}`, source: p, target: id, style: "data" });
    }
    for (const c of consumers) {
      edges.push({ id: `${id}--art-->${c}`, source: id, target: c, style: "data" });
    }
  }
  return { nodes, edges };
}

export function workflowDataflow(
  content: PromptPackContent,
): { nodes: WorkloadNode[]; edges: WorkloadEdge[] } {
  const wf = content.workflow;
  const vars = variableNodesAndEdges(collectVariables(content), wf?.entry);
  const pseudo = pseudoStateNodesAndEdges(content);
  const arts = artifactNodesAndEdges(content);
  return {
    nodes: [...vars.nodes, ...pseudo.nodes, ...arts.nodes],
    edges: [...vars.edges, ...pseudo.edges, ...arts.edges],
  };
}
