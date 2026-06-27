"use client";

import { useCallback, useState } from "react";

/**
 * useSetAgentExpose PATCHes expose on the agent's primary facade (#1611/#1576)
 * and tracks
 * the in-flight + error state. The error is the surfaced server message (e.g. a
 * 403 for a non-editor, or a validation message), so callers can show it inline.
 */
export function useSetAgentExpose(workspace: string, agentName: string) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = useCallback(
    async (enabled: boolean, host: string): Promise<boolean> => {
      setSaving(true);
      setError(null);
      try {
        const res = await fetch(
          `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(agentName)}/expose`,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ enabled, host: host.trim() }),
          }
        );
        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new Error(body.error || body.message || "Failed to update exposure");
        }
        return true;
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to update exposure");
        return false;
      } finally {
        setSaving(false);
      }
    },
    [workspace, agentName]
  );

  return { save, saving, error };
}
