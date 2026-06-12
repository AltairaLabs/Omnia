import type {
  PromptPackContent,
  PromptDefinition,
  ToolDefinition,
} from "@/lib/data/types";
import type {
  WorkloadModel,
  WorkloadNode,
  WorkloadToolDetail,
} from "./types";

function toolDetails(
  names: string[] | undefined,
  packTools: PromptPackContent["tools"],
): WorkloadToolDetail[] {
  const lookup = new Map<string, ToolDefinition>();
  if (Array.isArray(packTools)) {
    for (const t of packTools) lookup.set(t.name, t);
  } else if (packTools) {
    for (const [name, t] of Object.entries(packTools)) lookup.set(name, { ...t, name });
  }
  return (names ?? []).map((name) => ({
    name,
    description: lookup.get(name)?.description,
  }));
}

function previewTemplate(tpl?: string): string | undefined {
  if (!tpl) return undefined;
  return tpl.length > 280 ? `${tpl.slice(0, 280)}…` : tpl;
}

function agentNodeFromPrompt(
  id: string,
  prompt: PromptDefinition,
  content: PromptPackContent,
  isEntry: boolean,
): WorkloadNode {
  const tools = toolDetails(prompt.tools, content.tools);
  const skills = (content.skills ?? []).map((s) => s.name);
  return {
    id,
    kind: "agent",
    label: prompt.name || prompt.id || id,
    isEntry,
    badges: [
      { icon: "tool", label: `${tools.length}` },
      { icon: "skill", label: `${skills.length}` },
    ],
    detail: {
      description: prompt.description,
      systemTemplatePreview: previewTemplate(prompt.system_template),
      tools,
      skills,
      parameters: prompt.parameters,
    },
  };
}

function firstPrompt(
  content: PromptPackContent,
): { id: string; prompt: PromptDefinition } | undefined {
  const entries = Object.entries(content.prompts ?? {});
  if (entries.length === 0) return undefined;
  const [id, prompt] = entries[0];
  return { id, prompt };
}

function sumTools(nodes: WorkloadNode[]): number {
  const set = new Set<string>();
  for (const n of nodes) for (const t of n.detail.tools ?? []) set.add(t.name);
  return set.size;
}

export function deriveWorkloadTier(content: PromptPackContent): WorkloadModel {
  const skillCount = (content.skills ?? []).length;

  // Solo: no workflow, no agents — single agent from the (only) prompt.
  const first = firstPrompt(content);
  const nodes: WorkloadNode[] = first
    ? [agentNodeFromPrompt(first.id, first.prompt, content, true)]
    : [];

  return {
    tier: "solo",
    altitude: "definition",
    nodes,
    edges: [],
    meta: {
      counts: { agents: nodes.length, tools: sumTools(nodes), skills: skillCount, states: 0 },
    },
  };
}
