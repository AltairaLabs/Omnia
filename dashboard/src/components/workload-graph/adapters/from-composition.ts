import type { CompositionDef, CompositionStep, CompositionPredicate } from "@/lib/data/types";
import type { CompositionSubgraph, WorkloadNode, WorkloadEdge, WorkloadNodeKind } from "../types";

const STEP_KIND: Record<CompositionStep["kind"], WorkloadNodeKind> = {
  prompt: "stepPrompt",
  agent: "stepAgent",
  tool: "stepTool",
  branch: "stepBranch",
  parallel: "stepParallel",
};

function valueText(v: unknown): string {
  if (Array.isArray(v)) return v.map(valueText).join(", ");
  if (v === null || v === undefined) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

export function predicateText(p: CompositionPredicate): string {
  if (p.all_of) return `(${p.all_of.map(predicateText).join(" AND ")})`;
  if (p.any_of) return `(${p.any_of.map(predicateText).join(" OR ")})`;
  if (p.not) return `NOT (${predicateText(p.not)})`;
  if (p.exists !== undefined) return `${p.path} ${p.exists ? "exists" : "missing"}`;
  return `${p.path} ${p.op} ${valueText(p.value)}`.trim();
}

function terminationText(t?: CompositionStep["termination"]): string | undefined {
  if (!t) return undefined;
  if (t.max_steps != null) return `≤${t.max_steps} steps`;
  if (t.tool_called) return `until ${t.tool_called}`;
  return undefined;
}

function stepNode(containerId: string, parentId: string, step: CompositionStep): WorkloadNode {
  const id = `${containerId}::${step.id}`;
  return {
    id,
    parentId,
    kind: STEP_KIND[step.kind],
    label: step.id,
    isContainer: step.kind === "parallel",
    badges: [],
    detail: {
      description: step.description,
      stepKind: step.kind,
      promptTask: step.prompt_task,
      toolRef: step.tool,
      args: step.args,
      predicateText: step.predicate ? predicateText(step.predicate) : undefined,
      reducer: step.reduce ? `${step.reduce.strategy} → ${step.reduce.into}` : undefined,
      termination: terminationText(step.termination),
      tools: (step.tools ?? []).map((name) => ({ name })),
      evals: step.modifiers?.eval,
    },
  };
}

type AddEdge = (sourceStep: string, targetStep: string, label?: string) => void;

// top-level step nodes (+ parallel branch children).
function buildStepNodes(containerId: string, steps: CompositionStep[]): WorkloadNode[] {
  const nodes: WorkloadNode[] = [];
  for (const step of steps) {
    nodes.push(stepNode(containerId, containerId, step));
    if (step.kind === "parallel") {
      for (const b of step.branches ?? []) nodes.push(stepNode(containerId, `${containerId}::${step.id}`, b));
    }
  }
  return nodes;
}

// branch then/else + depends_on edges, which win over the sequential backbone.
function addExplicitEdges(step: CompositionStep, addEdge: AddEdge): void {
  if (step.kind === "branch") {
    if (step.then) addEdge(step.id, step.then, "then");
    if (step.else) addEdge(step.id, step.else, "else");
  }
  for (const dep of step.depends_on ?? []) addEdge(dep, step.id);
}

function buildStepEdges(containerId: string, steps: CompositionStep[]): WorkloadEdge[] {
  const edges: WorkloadEdge[] = [];
  const seen = new Set<string>();
  const addEdge: AddEdge = (sourceStep, targetStep, label) => {
    const source = `${containerId}::${sourceStep}`;
    const target = `${containerId}::${targetStep}`;
    const key = `${source}->${target}`;
    if (seen.has(key)) return;
    seen.add(key);
    edges.push({ id: `${containerId}::${sourceStep}->${targetStep}`, source, target, label });
  };

  for (const step of steps) addExplicitEdges(step, addEdge);
  // sequential backbone for steps without explicit deps
  for (let i = 1; i < steps.length; i++) {
    if ((steps[i].depends_on ?? []).length === 0) addEdge(steps[i - 1].id, steps[i].id);
  }
  return edges;
}

export function compositionToWorkload(
  containerId: string,
  name: string,
  comp: CompositionDef,
): CompositionSubgraph {
  const steps = comp.steps ?? [];
  return { name, nodes: buildStepNodes(containerId, steps), edges: buildStepEdges(containerId, steps) };
}
