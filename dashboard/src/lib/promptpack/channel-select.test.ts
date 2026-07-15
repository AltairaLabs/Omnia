import { describe, it, expect } from "vitest";
import { selectChannelMax } from "./channel-select";
import type { PromptPack } from "@/types";

function pack(version: string, packName = "demo"): PromptPack {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name: `pp-${version}`, namespace: "ns1" },
    spec: {
      packName,
      source: { type: "configmap", configMapRef: { name: "cm" } },
      version,
    },
  } as PromptPack;
}

describe("selectChannelMax", () => {
  it("returns highest stable version, excluding prereleases (default track)", () => {
    const packs = [pack("1.0.0"), pack("1.1.0"), pack("2.0.0-beta.1")];
    expect(selectChannelMax(packs)?.spec.version).toBe("1.1.0");
    expect(selectChannelMax(packs, "stable")?.spec.version).toBe("1.1.0");
  });

  it("returns highest overall version for prerelease track", () => {
    const packs = [pack("1.0.0"), pack("1.1.0"), pack("2.0.0-beta.1")];
    expect(selectChannelMax(packs, "prerelease")?.spec.version).toBe("2.0.0-beta.1");
  });

  it("returns the single pack when only one exists", () => {
    expect(selectChannelMax([pack("1.0.0")])?.spec.version).toBe("1.0.0");
  });

  it("returns undefined for an empty list", () => {
    expect(selectChannelMax([])).toBeUndefined();
  });

  it("returns undefined when stable track has only prereleases", () => {
    expect(selectChannelMax([pack("2.0.0-rc.1")], "stable")).toBeUndefined();
    expect(selectChannelMax([pack("2.0.0-rc.1")], "prerelease")?.spec.version).toBe("2.0.0-rc.1");
  });

  it("tolerates a leading v prefix", () => {
    const packs = [pack("v1.0.0"), pack("v1.2.0")];
    expect(selectChannelMax(packs)?.spec.version).toBe("v1.2.0");
  });

  it("ignores packs with unparseable versions", () => {
    const packs = [pack("not-a-version"), pack("1.5.0")];
    expect(selectChannelMax(packs)?.spec.version).toBe("1.5.0");
  });

  it("orders release above its own prerelease at the same core version", () => {
    const packs = [pack("2.0.0-beta.1"), pack("2.0.0")];
    expect(selectChannelMax(packs, "prerelease")?.spec.version).toBe("2.0.0");
  });

  it("orders numeric prerelease identifiers numerically", () => {
    const packs = [pack("1.0.0-alpha.2"), pack("1.0.0-alpha.10")];
    expect(selectChannelMax(packs, "prerelease")?.spec.version).toBe("1.0.0-alpha.10");
  });

  it("orders numeric prerelease identifiers below alphanumeric ones", () => {
    const packs = [pack("1.0.0-1"), pack("1.0.0-alpha")];
    expect(selectChannelMax(packs, "prerelease")?.spec.version).toBe("1.0.0-alpha");
  });

  it("ranks a longer prerelease above a shorter identical prefix", () => {
    const packs = [pack("1.0.0-alpha"), pack("1.0.0-alpha.1")];
    expect(selectChannelMax(packs, "prerelease")?.spec.version).toBe("1.0.0-alpha.1");
  });

  it("ignores build metadata when comparing", () => {
    const packs = [pack("1.0.0+build.9"), pack("1.0.1")];
    expect(selectChannelMax(packs)?.spec.version).toBe("1.0.1");
  });
});
