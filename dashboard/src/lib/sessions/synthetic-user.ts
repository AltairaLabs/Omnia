/**
 * Helpers for identifying synthetic (non-human) users in a session, used to
 * badge user turns in the conversation view. The signal comes from the session
 * tags written by the arena worker:
 *   - "source:arena"  → the session is arena-driven (no real user)
 *   - "persona:<id>"  → user turns are self-play (LLM persona-driven)
 *   - source:arena without a persona → user turns are scenario-scripted
 */

export type SyntheticUser = { kind: "self-play" | "scenario"; persona?: string };

const arenaSourceTag = "source:arena";
const personaTagPrefix = "persona:";

/**
 * Returns a SyntheticUser descriptor when the session's tags indicate the user
 * turns are arena-generated, or null for an ordinary (real-user) session.
 */
export function syntheticUserInfo(tags?: string[]): SyntheticUser | null {
  if (!tags?.includes(arenaSourceTag)) return null;
  const personaTag = tags.find((t) => t.startsWith(personaTagPrefix));
  const persona = personaTag ? personaTag.slice(personaTagPrefix.length) : undefined;
  return { kind: persona ? "self-play" : "scenario", persona };
}

/** Short human label for a synthetic user, e.g. "Self-play · sre-user". */
export function syntheticUserLabel(s: SyntheticUser): string {
  if (s.kind === "scenario") return "Scenario";
  return s.persona ? `Self-play · ${s.persona}` : "Self-play";
}
