import type { AgentRuntime } from "@/types";
import type { AggregateRow } from "./types";

/**
 * Build a map from AgentRuntime UID to human-readable name. Agents missing a
 * UID (still being persisted by the API server) are skipped.
 */
export function agentNameByUidMap(
  agents: ReadonlyArray<AgentRuntime>,
): Map<string, string> {
  const map = new Map<string, string>();
  for (const agent of agents) {
    const uid = agent.metadata?.uid;
    if (uid) map.set(uid, agent.metadata.name);
  }
  return map;
}

/**
 * Replace the AggregateRow.key (an AgentRuntime UID from memory_entities.agent_id)
 * with the corresponding agent name. UIDs without a matching agent (deleted,
 * cross-namespace) pass through unchanged so the row remains traceable.
 */
export function resolveAgentRows(
  rows: ReadonlyArray<AggregateRow>,
  nameByUid: ReadonlyMap<string, string>,
): AggregateRow[] {
  return rows.map((r) => ({ ...r, key: nameByUid.get(r.key) ?? r.key }));
}
