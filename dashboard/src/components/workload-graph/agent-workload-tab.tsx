"use client";

import { usePromptPack, usePromptPackContent, useProviders, useToolRegistry } from "@/hooks/resources";
import { useSkillSources } from "@/hooks/use-skill-sources";
import type { AgentRuntime } from "@/types";
import { WorkloadGraph } from "./WorkloadGraph";
import { agentRuntimeToWorkload, type ResolvedProvider } from "./adapters/from-agent";

export function AgentWorkloadTab({
  agent,
  workspace,
}: Readonly<{ agent: AgentRuntime; workspace: string }>) {
  const ns = agent.metadata.namespace;
  const packName = agent.spec.promptPackRef?.name ?? "";
  const { data: content } = usePromptPackContent(packName, workspace);
  const { data: promptPack } = usePromptPack(packName, workspace);
  const { sources: skillSources } = useSkillSources();

  const { data: allProviders } = useProviders();

  const trName = agent.spec.toolRegistryRef?.name ?? "";
  const { data: toolRegistry } = useToolRegistry(trName, agent.spec.toolRegistryRef?.namespace ?? ns);

  const providers: ResolvedProvider[] = (agent.spec.providers ?? []).map((ref) => {
    const p = allProviders?.find((x) => x.metadata.name === ref.providerRef.name);
    return {
      name: ref.name ?? ref.providerRef.name,
      type: p?.spec?.type,
      model: p?.spec?.model,
      baseURL: p?.spec?.baseURL,
    };
  });

  const discoveredTools = (toolRegistry?.status?.discoveredTools ?? []).map((t) => ({
    name: t.name,
    handlerName: t.handlerName,
    description: t.description,
    endpoint: t.endpoint,
    status: t.status,
  }));

  const model = agentRuntimeToWorkload({
    content: content ?? undefined,
    providers,
    discoveredTools,
    toolRegistryName: trName || undefined,
    skillRefs: promptPack?.spec?.skills,
    skillSources,
  });
  return <WorkloadGraph model={model} namespace={ns} storageKey={`agent:${ns}:${agent.metadata.name}`} />;
}
