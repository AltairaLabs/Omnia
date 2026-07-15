/**
 * Client-side channel-max PromptPack selection.
 *
 * After #1837 a PromptPack's `metadata.name` is a deterministic `pp-<hash>` and
 * multiple version-objects share one logical `spec.packName`. The dashboard
 * resolves a logical pack to a concrete version-object by picking the
 * "channel-max": the highest semver on the requested track.
 *
 * Mirrors the Go `internal/promptpack/packselect.ChannelMax` semantics:
 *   - "stable" (default): highest version with NO prerelease identifier
 *   - "prerelease": highest version overall (prereleases included)
 *
 * A tiny semver compare is used (no `semver` npm dependency in the dashboard).
 */

import type { PromptPack } from "@/types";

export type Track = "stable" | "prerelease";

interface ParsedVersion {
  major: number;
  minor: number;
  patch: number;
  /** Prerelease identifier string ("" when the version is a release). */
  prerelease: string;
}

/** Split off SemVer build metadata (`+...`) and the prerelease (`-...`). */
function splitVersion(version: string): { core: string; prerelease: string } {
  const withoutBuild = version.split("+")[0];
  const dashIdx = withoutBuild.indexOf("-");
  if (dashIdx === -1) {
    return { core: withoutBuild, prerelease: "" };
  }
  return {
    core: withoutBuild.slice(0, dashIdx),
    prerelease: withoutBuild.slice(dashIdx + 1),
  };
}

/** Parse `major.minor.patch[-prerelease][+build]`, tolerating a leading `v`. */
function parseVersion(raw: string): ParsedVersion | undefined {
  const trimmed = raw.trim().replace(/^v/, "");
  if (trimmed === "") return undefined;

  const { core, prerelease } = splitVersion(trimmed);
  const parts = core.split(".");
  if (parts.length !== 3) return undefined;

  const nums = parts.map((p) => Number(p));
  if (nums.some((n) => !Number.isInteger(n) || n < 0)) return undefined;

  return { major: nums[0], minor: nums[1], patch: nums[2], prerelease };
}

/** Compare a single dot-separated prerelease identifier (SemVer rules). */
function compareIdentifier(a: string, b: string): number {
  const aNumeric = /^\d+$/.test(a);
  const bNumeric = /^\d+$/.test(b);
  if (aNumeric && bNumeric) return Number(a) - Number(b);
  if (aNumeric) return -1; // numeric identifiers have lower precedence
  if (bNumeric) return 1;
  return a.localeCompare(b);
}

/** Compare prerelease strings; a release ("") outranks any prerelease. */
function comparePrerelease(a: string, b: string): number {
  if (a === b) return 0;
  if (a === "") return 1;
  if (b === "") return -1;

  const aIds = a.split(".");
  const bIds = b.split(".");
  const len = Math.min(aIds.length, bIds.length);
  for (let i = 0; i < len; i++) {
    const cmp = compareIdentifier(aIds[i], bIds[i]);
    if (cmp !== 0) return cmp;
  }
  return aIds.length - bIds.length;
}

/** Compare two parsed versions; >0 when `a` is greater. */
function compareVersions(a: ParsedVersion, b: ParsedVersion): number {
  if (a.major !== b.major) return a.major - b.major;
  if (a.minor !== b.minor) return a.minor - b.minor;
  if (a.patch !== b.patch) return a.patch - b.patch;
  return comparePrerelease(a.prerelease, b.prerelease);
}

interface Candidate {
  pack: PromptPack;
  version: ParsedVersion;
}

/**
 * Select the channel-max PromptPack from a set sharing one logical packName.
 *
 * @param packs Version-objects to choose from.
 * @param track "stable" (default) excludes prereleases; "prerelease" allows them.
 * @returns The chosen pack, or undefined when no candidate matches the track.
 */
export function selectChannelMax(
  packs: PromptPack[],
  track: Track = "stable",
): PromptPack | undefined {
  const parsed: Candidate[] = [];
  for (const pack of packs) {
    const version = parseVersion(pack.spec?.version ?? "");
    if (version) parsed.push({ pack, version });
  }

  const candidates =
    track === "stable" ? parsed.filter((c) => c.version.prerelease === "") : parsed;

  if (candidates.length === 0) return undefined;

  let best = candidates[0];
  for (const candidate of candidates.slice(1)) {
    if (compareVersions(candidate.version, best.version) > 0) {
      best = candidate;
    }
  }
  return best.pack;
}

/**
 * Group version-objects by `keyFn` and reduce each group to its channel-max
 * (stable, falling back to prerelease, then the first entry). Shared by the
 * PromptPack list page and the topology graph so both show one logical pack
 * per `namespace/packName`.
 */
export function channelMaxByGroup(
  packs: PromptPack[],
  keyFn: (pack: PromptPack) => string,
): Map<string, PromptPack> {
  const groups = new Map<string, PromptPack[]>();
  for (const pack of packs) {
    const key = keyFn(pack);
    const group = groups.get(key);
    if (group) {
      group.push(pack);
    } else {
      groups.set(key, [pack]);
    }
  }

  const result = new Map<string, PromptPack>();
  for (const [key, group] of groups) {
    result.set(key, selectChannelMax(group, "stable") ?? selectChannelMax(group, "prerelease") ?? group[0]);
  }
  return result;
}
