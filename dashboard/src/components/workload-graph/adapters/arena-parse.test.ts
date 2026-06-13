import { describe, it, expect } from "vitest";
import { referencedFiles, parseArenaProject } from "./arena-parse";

const CONFIG = `
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
  providers:
    - file: providers/gpt.provider.yaml
      group: default
  scenarios:
    - file: scenarios/qa.scenario.yaml
  judges:
    - name: relevance
      provider: judge-gpt
  self_play:
    enabled: true
    personas:
      - file: personas/user.persona.yaml
    roles:
      - id: user
        provider: selfplay
`;

const PROMPT = `
kind: PromptConfig
metadata: { name: assistant }
spec:
  description: An assistant
  system_template: "Hello {{topic}}"
  variables:
    - { name: topic, type: string, required: true }
  allowed_tools: [search]
`;

const PROVIDER = `
kind: Provider
metadata: { name: gpt }
spec: { id: gpt, type: openai, model: gpt-4o }
`;

const SCENARIO = `
kind: Scenario
metadata: { name: qa }
spec:
  id: qa
  description: a QA scenario
  tags: [smoke]
  assertions: [is-helpful]
  turns:
    - { role: user, content: hi }
    - { role: assistant, content: hello }
`;

const PERSONA = `
kind: Persona
metadata: { name: user }
spec: { id: user, description: a user }
`;

const files: Record<string, string> = {
  "config.arena.yaml": CONFIG,
  "prompts/assistant.yaml": PROMPT,
  "providers/gpt.provider.yaml": PROVIDER,
  "scenarios/qa.scenario.yaml": SCENARIO,
  "personas/user.persona.yaml": PERSONA,
};

const readFile = (p: string) => files[p];

describe("referencedFiles", () => {
  it("lists every file the config refers to, resolved relative to the config dir", () => {
    const refs = referencedFiles("config.arena.yaml", CONFIG);
    expect(refs.sort()).toEqual(
      [
        "personas/user.persona.yaml",
        "prompts/assistant.yaml",
        "providers/gpt.provider.yaml",
        "scenarios/qa.scenario.yaml",
      ].sort(),
    );
  });

  it("returns [] for unparseable config", () => {
    expect(referencedFiles("config.arena.yaml", "{{ not yaml")).toEqual([]);
  });
});

describe("parseArenaProject", () => {
  it("builds synth content + harness lists from the files", () => {
    const { parsed, error } = parseArenaProject({
      configPath: "config.arena.yaml",
      configContent: CONFIG,
      readFile,
    });
    expect(error).toBeNull();
    expect(parsed!.content.prompts!.assistant.system_template).toBe("Hello {{topic}}");
    expect(parsed!.content.prompts!.assistant.variables![0].name).toBe("topic");
    expect(parsed!.providers).toEqual([
      { id: "gpt", model: "gpt-4o", providerType: "openai", group: "default", pricing: undefined, resolved: true },
    ]);
    expect(parsed!.scenarios).toEqual([
      { id: "qa", description: "a QA scenario", turnCount: 2, tags: ["smoke"], assertions: ["is-helpful"] },
    ]);
    expect(parsed!.judges).toEqual([{ id: "relevance", provider: "judge-gpt" }]);
    expect(parsed!.persona).toEqual({ id: "user", role: "user", provider: "selfplay" });
  });

  it("marks a provider whose file is missing as unresolved", () => {
    const { parsed } = parseArenaProject({
      configPath: "config.arena.yaml",
      configContent: CONFIG,
      readFile: (p) => (p === "providers/gpt.provider.yaml" ? undefined : files[p]),
    });
    expect(parsed!.providers[0].resolved).toBe(false);
  });

  it("returns an error (and null parsed) for unparseable config", () => {
    const { parsed, error } = parseArenaProject({
      configPath: "config.arena.yaml",
      configContent: "{{ not yaml",
      readFile,
    });
    expect(parsed).toBeNull();
    expect(error).toMatch(/config/i);
  });

  it("treats valid YAML with no spec as an empty workload, not an error", () => {
    // A brand-new project writes a metadata-only config.arena.yaml (no spec).
    const { parsed, error } = parseArenaProject({
      configPath: "config.arena.yaml",
      configContent: "name: my-project\ndescription: just metadata\ntags: []\n",
      readFile,
    });
    expect(error).toBeNull();
    expect(parsed).not.toBeNull();
    expect(parsed!.content.prompts).toEqual({});
    expect(parsed!.providers).toEqual([]);
    expect(parsed!.scenarios).toEqual([]);
    expect(parsed!.persona).toBeUndefined();
  });

  it("omits the persona when self_play is disabled", () => {
    const noSelfPlay = CONFIG.replace("enabled: true", "enabled: false");
    const { parsed } = parseArenaProject({ configPath: "config.arena.yaml", configContent: noSelfPlay, readFile });
    expect(parsed!.persona).toBeUndefined();
  });

  it("reads providers in map mode and carries workflow + agents through", () => {
    const mapCfg = `
kind: Arena
spec:
  prompt_configs:
    - id: writer
      file: prompts/writer.yaml
  providers:
    gpt:
      file: providers/gpt.provider.yaml
      group: default
  workflow:
    version: 1
    entry: s
    states:
      s: { prompt_task: writer, terminal: true }
`;
    const { parsed } = parseArenaProject({
      configPath: "config.arena.yaml",
      configContent: mapCfg,
      readFile: (p) => (p === "providers/gpt.provider.yaml" ? PROVIDER : undefined),
    });
    expect(parsed!.providers[0].id).toBe("gpt");
    expect(parsed!.content.workflow!.entry).toBe("s");
  });
});
