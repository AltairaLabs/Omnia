import type { PromptPackContent } from "@/lib/data/types";
import type {
  WorkloadModel,
  WorkloadNode,
  WorkloadToolDetail,
  ResolutionStatus,
} from "../types";
import { deriveWorkloadTier } from "../derive-tier";

export interface ResolvedProvider {
  name: string;
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
    id: `provider:${p.name}`,
    kind: "provider",
    label: p.name,
    badges: p.role ? [{ label: p.role }] : [],
    detail: { model: p.model, providerType: p.type, baseURL: p.baseURL, role: p.role },
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

  for (const p of inputs.providers) nodes.push(providerNode(p));

  return {
    ...base,
    altitude: "deployment",
    nodes,
    meta: {
      ...base.meta,
      binding: {
        providers: inputs.providers.map((p) => ({ name: p.name, model: p.model, role: p.role })),
        toolRegistry: inputs.toolRegistryName,
      },
    },
  };
}
