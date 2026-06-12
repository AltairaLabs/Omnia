import type {
  PromptPackContent,
  PromptDefinition,
  ToolDefinition,
  WorkflowConfig,
} from "@/lib/data/types";
import type {
  WorkloadModel,
  WorkloadNode,
  WorkloadEdge,
  WorkloadBudget,
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

function budgetFromWorkflow(wf?: WorkflowConfig): WorkloadBudget | undefined {
  const b = wf?.engine?.budget;
  if (!b) return undefined;
  return {
    maxTotalVisits: b.max_total_visits,
    maxToolCalls: b.max_tool_calls,
    maxWallTimeSec: b.max_wall_time_sec,
  };
}

function stateNode(
  id: string,
  content: PromptPackContent,
  isEntry: boolean,
): WorkloadNode {
  const wf = content.workflow!;
  const state = wf.states[id];
  const prompt = content.prompts?.[state.prompt_task];
  const tools = toolDetails(prompt?.tools, content.tools);
  const skills = (content.skills ?? []).map((s) => s.name);
  const badges: WorkloadNode["badges"] = [
    { icon: "tool", label: `${tools.length}` },
    { icon: "skill", label: `${skills.length}` },
  ];
  if (state.max_visits != null) badges.push({ icon: "loop", label: `≤${state.max_visits}` });
  return {
    id,
    kind: "state",
    label: prompt?.name || state.prompt_task || id,
    isEntry,
    isTerminal: state.terminal === true,
    badges,
    detail: {
      description: state.description || prompt?.description,
      systemTemplatePreview: previewTemplate(prompt?.system_template),
      tools,
      skills,
      parameters: prompt?.parameters,
    },
  };
}

function flowEdges(content: PromptPackContent): WorkloadEdge[] {
  const wf = content.workflow!;
  const edges: WorkloadEdge[] = [];
  for (const [stateId, state] of Object.entries(wf.states)) {
    for (const [event, target] of Object.entries(state.on_event ?? {})) {
      edges.push({
        id: `${stateId}--${event}-->${target}`,
        source: stateId,
        target,
        label: event,
        style: wf.states[target] ? "normal" : "unresolved",
      });
    }
    if (state.on_max_visits) {
      edges.push({
        id: `${stateId}--maxvisits-->${state.on_max_visits}`,
        source: stateId,
        target: state.on_max_visits,
        label: "max visits",
        style: "loop",
      });
    }
  }
  return edges;
}

function crewAgentNode(
  promptKey: string,
  content: PromptPackContent,
): WorkloadNode {
  const member = content.agents!.members[promptKey];
  const prompt = content.prompts?.[promptKey];
  const tools = toolDetails(prompt?.tools, content.tools);
  const skills = (content.skills ?? []).map((s) => s.name);
  return {
    id: promptKey,
    kind: "agent",
    label: prompt?.name || promptKey,
    isEntry: content.agents!.entry === promptKey,
    badges: [
      { icon: "tool", label: `${tools.length}` },
      { icon: "skill", label: `${skills.length}` },
    ],
    detail: {
      description: member?.description || prompt?.description,
      systemTemplatePreview: previewTemplate(prompt?.system_template),
      tools,
      skills,
      parameters: prompt?.parameters,
      ioModes: { input: member?.input_modes, output: member?.output_modes },
    },
  };
}

export function deriveWorkloadTier(content: PromptPackContent): WorkloadModel {
  const skillCount = (content.skills ?? []).length;
  const wf = content.workflow;

  // Crew: explicit A2A agents as first-class nodes; workflow (if any) overlays hand-offs.
  const agentsCfg = content.agents;
  if (agentsCfg && Object.keys(agentsCfg.members ?? {}).length > 0) {
    const memberKeys = Object.keys(agentsCfg.members);
    const nodes = memberKeys.map((k) => crewAgentNode(k, content));
    const edges = content.workflow ? flowEdges(content) : [];
    const stateCount = Object.keys(content.workflow?.states ?? {}).length;
    return {
      tier: "crew",
      altitude: "definition",
      nodes,
      edges,
      meta: {
        budget: budgetFromWorkflow(content.workflow),
        counts: { agents: memberKeys.length, tools: sumTools(nodes), skills: skillCount, states: stateCount },
      },
    };
  }

  // Flow: a workflow state machine, one implicit agent moving through states.
  if (wf && Object.keys(wf.states ?? {}).length > 0) {
    const stateIds = Object.keys(wf.states);
    const nodes = stateIds.map((id) => stateNode(id, content, id === wf.entry));
    return {
      tier: "flow",
      altitude: "definition",
      nodes,
      edges: flowEdges(content),
      meta: {
        budget: budgetFromWorkflow(wf),
        counts: { agents: 1, tools: sumTools(nodes), skills: skillCount, states: stateIds.length },
      },
    };
  }

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
