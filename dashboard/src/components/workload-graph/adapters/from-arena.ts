import type { PromptPackContent } from "@/lib/data/types";
import type { WorkloadModel, WorkloadNode, WorkloadEdge } from "../types";
import { deriveWorkloadTier } from "../derive-tier";
import type { ArenaParsed } from "./arena-parse";

/** The node the harness attaches to — mirrors deriveWorkloadTier's tier precedence. */
function entryId(content: PromptPackContent): string | undefined {
  if (content.agents && Object.keys(content.agents.members ?? {}).length > 0) {
    return content.agents.entry;
  }
  if (content.workflow && Object.keys(content.workflow.states ?? {}).length > 0) {
    return content.workflow.entry;
  }
  return Object.keys(content.prompts ?? {})[0];
}

export function arenaProjectToWorkload(parsed: ArenaParsed): WorkloadModel {
  const base = deriveWorkloadTier(parsed.content);
  const entry = entryId(parsed.content);
  const nodes: WorkloadNode[] = [...base.nodes];
  const edges: WorkloadEdge[] = [...base.edges];

  for (const p of parsed.providers) {
    const id = `provider:${p.id}`;
    nodes.push({
      id,
      kind: "provider",
      label: p.id,
      badges: [],
      resolution: p.resolved ? "resolved" : "unresolved",
      detail: { model: p.model, providerType: p.providerType, role: p.group, pricing: p.pricing },
    });
    if (entry) edges.push({ id: `${id}-->${entry}`, source: id, target: entry, style: "provides" });
  }

  if (parsed.scenarios.length > 0) {
    const id = "scenarios";
    const n = parsed.scenarios.length;
    nodes.push({
      id,
      kind: "scenario",
      label: `${n} scenario${n === 1 ? "" : "s"}`,
      badges: [],
      detail: { scenarios: parsed.scenarios },
    });
    if (entry) edges.push({ id: `${id}-->${entry}`, source: id, target: entry, style: "data" });
  }

  for (const j of parsed.judges) {
    const id = `judge:${j.id}`;
    nodes.push({ id, kind: "judge", label: j.id, badges: [], detail: { judgeProvider: j.provider } });
    if (entry) edges.push({ id: `${entry}-->${id}`, source: entry, target: id, style: "data" });
  }

  if (parsed.persona && entry) {
    const id = `persona:${parsed.persona.id}`;
    nodes.push({ id, kind: "persona", label: parsed.persona.id, badges: [], detail: { persona: parsed.persona } });
    edges.push({ id: `${id}<-->${entry}`, source: id, target: entry, style: "loop" });
  }

  return {
    ...base,
    altitude: "test",
    nodes,
    edges,
  };
}
