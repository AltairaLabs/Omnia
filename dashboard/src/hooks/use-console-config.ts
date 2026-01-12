"use client";

import { useMemo } from "react";
import { useAgent } from "./use-agents";
import { useProvider } from "./use-provider";
import {
  buildAttachmentConfig,
  type AttachmentConfig,
} from "@/components/console/attachment-utils";
import { getMediaRequirements } from "@/lib/provider-media-defaults";
import type {
  ConsoleConfig,
  MediaRequirements,
  ProviderType,
} from "@/types/agent-runtime";

export interface ConsoleConfigResult {
  /** Resolved attachment configuration with defaults applied */
  config: AttachmentConfig;
  /** Resolved media requirements based on provider */
  mediaRequirements: MediaRequirements;
  /** The resolved provider type (may be undefined if not configured) */
  providerType: ProviderType | undefined;
  /** Whether the agent config is still loading */
  isLoading: boolean;
  /** Error if agent fetch failed */
  error: Error | null;
  /** Raw console config from agent spec (for debugging) */
  rawConfig: ConsoleConfig | undefined;
}

/**
 * Hook to get console configuration from an agent's spec.
 * Falls back to default values when agent has no console config.
 * Also resolves media requirements based on the provider type.
 */
export function useConsoleConfig(
  namespace: string,
  agentName: string
): ConsoleConfigResult {
  const { data: agent, isLoading: agentLoading, error: agentError } = useAgent(agentName, namespace);

  // Resolve provider from ProviderRef if specified
  const providerRefName = agent?.spec?.providerRef?.name;
  const providerRefNamespace = agent?.spec?.providerRef?.namespace ?? namespace;
  const { data: providerCRD, isLoading: providerLoading } = useProvider(
    providerRefName,
    providerRefNamespace
  );

  const attachmentConfig: AttachmentConfig = useMemo(() => {
    return buildAttachmentConfig(agent?.spec?.console);
  }, [agent?.spec?.console]);

  // Resolve provider type: ProviderRef takes precedence over inline provider
  const providerType: ProviderType | undefined = useMemo(() => {
    if (providerCRD?.spec?.type) {
      return providerCRD.spec.type as ProviderType;
    }
    return agent?.spec?.provider?.type;
  }, [providerCRD, agent]);

  // Get media requirements based on provider type and any CRD overrides
  const mediaRequirements: MediaRequirements = useMemo(() => {
    return getMediaRequirements(
      providerType,
      agent?.spec?.console?.mediaRequirements
    );
  }, [providerType, agent?.spec?.console?.mediaRequirements]);

  return {
    config: attachmentConfig,
    mediaRequirements,
    providerType,
    isLoading: agentLoading || providerLoading,
    error: agentError,
    rawConfig: agent?.spec?.console,
  };
}
