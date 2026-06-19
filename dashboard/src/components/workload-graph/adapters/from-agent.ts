import type { PromptPackContent } from "@/lib/data/types";
import type { SkillRef } from "@/types/prompt-pack";
import type { SkillSource } from "@/types/skill-source";
import type {
  WorkloadModel,
  WorkloadNode,
  WorkloadToolDetail,
  ResolutionStatus,
} from "../types";
import { deriveWorkloadTier } from "../derive-tier";
import { attachSkills } from "./skills";

export interface ResolvedProvider {
  name: string; // Provider CRD name (display label)
  group?: string; // service-group / slot name (e.g. "default")
  type?: string;
  model?: string;
  baseURL?: string;
  role?: string;
}

export interface DiscoveredTool {
  name: string;
  handlerName?: string;
  description?: string;
  endpoint?: string;
  status?: string; // ToolRegistry status: Available | Unavailable | Unknown
}

export interface AgentWorkloadInputs {
  content: PromptPackContent | undefined;
  providers: ResolvedProvider[];
  discoveredTools: DiscoveredTool[];
  toolRegistryName?: string;
  skillRefs?: SkillRef[];
  skillSources?: SkillSource[];
}

function resolveTool(
  tool: WorkloadToolDetail,
  byName: Map<string, DiscoveredTool>,
  registryEmpty: boolean,
): WorkloadToolDetail {
  const match = byName.get(tool.name);
  if (match) {
    const status: ResolutionStatus = match.status === "Unavailable" ? "unavailable" : "resolved";
    return {
      ...tool,
      endpoint: match.endpoint,
      handlerType: match.handlerName,
      description: tool.description ?? match.description,
      status,
    };
  }
  return { ...tool, status: registryEmpty ? "unresolved" : "unavailable" };
}

function providerNode(p: ResolvedProvider): WorkloadNode {
  return {
    // Key by the (unique) service-group slot so two slots pointing at the same
    // Provider CRD don't collide; the label still shows the provider.
    id: `provider:${p.group ?? p.name}`,
    kind: "provider",
    label: p.name,
    badges: p.role ? [{ label: p.role }] : [],
    detail: { group: p.group, model: p.model, providerType: p.type, baseURL: p.baseURL, role: p.role },
  };
}

// A single node for the agent's ToolRegistry, carrying the discovered tools in
// its detail so the drawer lists them. Wired to the entry node so the graph
// shows the registry is bound to the agent.
function toolRegistryNode(name: string, discovered: DiscoveredTool[]): WorkloadNode {
  const tools: WorkloadToolDetail[] = discovered.map((t) => ({
    name: t.name,
    description: t.description,
    endpoint: t.endpoint,
    handlerType: t.handlerName,
    status: t.status === "Unavailable" ? "unavailable" : "resolved",
  }));
  return {
    id: "toolregistry",
    kind: "tool",
    label: name,
    badges: [{ icon: "tool", label: String(tools.length) }],
    detail: { tools },
  };
}

export function agentRuntimeToWorkload(inputs: AgentWorkloadInputs): WorkloadModel {
  const base = deriveWorkloadTier(inputs.content ?? {});
  const byName = new Map(inputs.discoveredTools.map((t) => [t.name, t]));
  const registryEmpty = inputs.discoveredTools.length === 0;

  const nodes: WorkloadNode[] = base.nodes.map((n) => ({
    ...n,
    detail: {
      ...n.detail,
      tools: n.detail.tools?.map((t) => resolveTool(t, byName, registryEmpty)),
    },
  }));

  const edges = [...base.edges];
  // The agent's entry node anchors the bindings (providers + tool registry) so
  // the graph shows what the agent calls out to.
  const entry = base.nodes.find((n) => n.isEntry) ?? base.nodes[0];
  const wireToEntry = (targetId: string) => {
    if (entry) {
      edges.push({ id: `${entry.id}->${targetId}`, source: entry.id, target: targetId, style: "provides" });
    }
  };

  // Providers wired to the entry so the graph shows which LLMs the agent uses.
  for (const p of inputs.providers) {
    const pn = providerNode(p);
    nodes.push(pn);
    wireToEntry(pn.id);
  }

  // A single ToolRegistry node wired to the agent's entry, when one is bound.
  if (inputs.toolRegistryName) {
    const registry = toolRegistryNode(inputs.toolRegistryName, inputs.discoveredTools);
    nodes.push(registry);
    wireToEntry(registry.id);
  }

  const deployed: WorkloadModel = {
    ...base,
    altitude: "deployment",
    nodes,
    edges,
    meta: {
      ...base.meta,
      binding: {
        providers: inputs.providers.map((p) => ({ name: p.name, model: p.model, role: p.role })),
        toolRegistry: inputs.toolRegistryName,
      },
    },
  };
  return attachSkills(deployed, inputs.skillRefs, inputs.skillSources);
}
