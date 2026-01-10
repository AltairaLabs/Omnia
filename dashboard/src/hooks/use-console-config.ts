"use client";

import { useMemo } from "react";
import { useAgent } from "./use-agents";
import {
  buildAttachmentConfig,
  type AttachmentConfig,
} from "@/components/console/attachment-utils";

/**
 * Hook to get console configuration from an agent's spec.
 * Falls back to default values when agent has no console config.
 */
export function useConsoleConfig(namespace: string, agentName: string) {
  const { data: agent, isLoading, error } = useAgent(agentName, namespace);

  const attachmentConfig: AttachmentConfig = useMemo(() => {
    return buildAttachmentConfig(agent?.spec?.console);
  }, [agent?.spec?.console]);

  return {
    /** Resolved attachment configuration with defaults applied */
    config: attachmentConfig,
    /** Whether the agent config is still loading */
    isLoading,
    /** Error if agent fetch failed */
    error,
    /** Raw console config from agent spec (for debugging) */
    rawConfig: agent?.spec?.console,
  };
}
