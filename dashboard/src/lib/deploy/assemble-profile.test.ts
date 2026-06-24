import { describe, it, expect } from "vitest";
import * as yaml from "js-yaml";
import { assembleDeployConfig } from "./assemble-profile";
import type { DeployProfile } from "@/types/deploy-profile";

const profile: DeployProfile = {
  api_endpoint: "https://omnia.example.com",
  workspace: "team-acme",
  providers: [
    { name: "default", role: "llm", type: "claude", model: "claude-sonnet-4" },
    { name: "embedder", role: "embedding", type: "openai", model: "text-embed-3" },
  ],
  skills: [{ name: "docs-search", type: "git" }],
};

describe("assembleDeployConfig", () => {
  it("builds a config block with connection, provider refs, and skill bindings", () => {
    const { json } = assembleDeployConfig(profile, "omnia_sk_TEST");
    const parsed = JSON.parse(json);
    expect(parsed.config.api_endpoint).toBe("https://omnia.example.com");
    expect(parsed.config.workspace).toBe("team-acme");
    expect(parsed.config.api_token).toBe("omnia_sk_TEST");
    expect(parsed.config.providers).toEqual([
      { name: "default", ref: "default", role: "llm" },
      { name: "embedder", ref: "embedder", role: "embedding" },
    ]);
    // The adapter consumes skills as SkillBinding objects ({source}), not
    // bare names — must match internal/omnia/config.go's schema (#1519).
    expect(parsed.config.skills).toEqual([{ source: "docs-search" }]);
  });

  it("produces valid YAML that round-trips to the same object", () => {
    const { yaml: y, json } = assembleDeployConfig(profile, "omnia_sk_TEST");
    expect(yaml.load(y)).toEqual(JSON.parse(json));
  });

  it("handles an empty workspace (no providers/skills)", () => {
    const empty: DeployProfile = {
      api_endpoint: "https://o",
      workspace: "w",
      providers: [],
      skills: [],
    };
    const { json } = assembleDeployConfig(empty, "omnia_sk_X");
    const parsed = JSON.parse(json);
    expect(parsed.config.providers).toEqual([]);
    expect(parsed.config.skills).toEqual([]);
  });
});
