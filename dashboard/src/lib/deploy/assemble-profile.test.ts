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
  supportedDeployIntentVersions: ["deploy.omnia.altairalabs.ai/v1"],
};

describe("assembleDeployConfig", () => {
  it("builds a config block with connection, provider refs, and skill bindings", () => {
    const { json } = assembleDeployConfig(profile, "omnia_sk_TEST");
    const parsed = JSON.parse(json);
    expect(parsed.config.api_endpoint).toBe("https://omnia.example.com");
    expect(parsed.config.workspace).toBe("team-acme");
    expect(parsed.config.api_token).toBe("omnia_sk_TEST");
    // Only the llm-role provider is emitted; the embedding provider is dropped
    // (non-llm providers aren't deployable into spec.providers — #1596).
    expect(parsed.config.providers).toEqual([
      { name: "default", ref: "default", role: "llm" },
    ]);
    // The adapter consumes skills as SkillBinding objects ({source}), not bare
    // names — must match internal/omnia/config.go's schema (#1519).
    expect(parsed.config.skills).toEqual([{ source: "docs-search" }]);
  });

  it("drops non-llm-role providers (#1596 regression)", () => {
    const { json } = assembleDeployConfig(profile, "t");
    const roles = JSON.parse(json).config.providers.map(
      (p: { role: string }) => p.role
    );
    expect(roles).toEqual(["llm"]);
    expect(roles).not.toContain("embedding");
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
      supportedDeployIntentVersions: ["deploy.omnia.altairalabs.ai/v1"],
    };
    const { json } = assembleDeployConfig(empty, "omnia_sk_X");
    const parsed = JSON.parse(json);
    expect(parsed.config.providers).toEqual([]);
    expect(parsed.config.skills).toEqual([]);
  });

  // The AgentRuntime requires a provider bound under the "default" name (its
  // primary LLM). When none of the discovered providers is literally named
  // "default", one LLM must be promoted, or every deployment breaks. (#1519)
  const noDefault: DeployProfile = {
    api_endpoint: "https://o",
    workspace: "w",
    providers: [
      { name: "rag-hero-baseline", role: "llm", type: "claude" },
      { name: "rag-hero-candidate", role: "llm", type: "claude" },
      { name: "rag-hero-embeddings", role: "embedding", type: "openai" },
    ],
    skills: [],
    supportedDeployIntentVersions: ["deploy.omnia.altairalabs.ai/v1"],
  };

  it("promotes the explicitly chosen LLM to the default binding", () => {
    const { json } = assembleDeployConfig(noDefault, "t", "rag-hero-candidate");
    const parsed = JSON.parse(json);
    expect(parsed.config.providers).toEqual([
      { name: "rag-hero-baseline", ref: "rag-hero-baseline", role: "llm" },
      { name: "default", ref: "rag-hero-candidate", role: "llm" },
    ]); // rag-hero-embeddings (embedding role) dropped (#1596)
  });

  it("falls back to the first LLM when no default is chosen", () => {
    const { json } = assembleDeployConfig(noDefault, "t");
    const parsed = JSON.parse(json);
    const def = parsed.config.providers.find(
      (p: { name: string }) => p.name === "default"
    );
    expect(def).toEqual({ name: "default", ref: "rag-hero-baseline", role: "llm" });
    // exactly one default
    expect(
      parsed.config.providers.filter((p: { name: string }) => p.name === "default")
    ).toHaveLength(1);
  });

  it("ignores a chosen provider that isn't an LLM and uses the first LLM", () => {
    const { json } = assembleDeployConfig(noDefault, "t", "rag-hero-embeddings");
    const parsed = JSON.parse(json);
    const def = parsed.config.providers.find(
      (p: { name: string }) => p.name === "default"
    );
    expect(def.ref).toBe("rag-hero-baseline");
  });

  it("leaves a provider already named default as the default", () => {
    const { json } = assembleDeployConfig(profile, "t");
    const parsed = JSON.parse(json);
    expect(parsed.config.providers[0]).toEqual({
      name: "default",
      ref: "default",
      role: "llm",
    });
  });
});
