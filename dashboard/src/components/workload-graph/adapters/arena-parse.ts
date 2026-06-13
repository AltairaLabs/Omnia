import yaml from "js-yaml";
import type {
  PromptPackContent,
  PromptDefinition,
  WorkflowConfig,
  AgentsConfig,
} from "@/lib/data/types";

export interface ArenaParsedProvider {
  id: string;
  model?: string;
  providerType?: string;
  group?: string;
  pricing?: { inputPer1kTokens?: number; outputPer1kTokens?: number };
  resolved: boolean;
}
export interface ArenaParsedScenario {
  id: string;
  description?: string;
  turnCount?: number;
  tags?: string[];
  assertions?: string[];
}
export interface ArenaParsedJudge {
  id: string;
  provider?: string;
}
export interface ArenaParsedPersona {
  id: string;
  role?: string;
  provider?: string;
}
export interface ArenaParsed {
  content: PromptPackContent;
  providers: ArenaParsedProvider[];
  scenarios: ArenaParsedScenario[];
  judges: ArenaParsedJudge[];
  persona?: ArenaParsedPersona;
}

interface FileRef {
  file?: string;
}
interface ProviderEntry {
  file?: string;
  group?: string;
}
interface ArenaConfigShape {
  spec?: {
    prompt_configs?: Array<{ id?: string; file?: string }>;
    providers?: ProviderEntry[] | Record<string, ProviderEntry>;
    scenarios?: FileRef[];
    judges?: Array<{ name?: string; provider?: string }>;
    judge_specs?: Record<string, { provider?: string }>;
    workflow?: WorkflowConfig;
    agents?: AgentsConfig;
    self_play?: {
      enabled?: boolean;
      personas?: FileRef[];
      roles?: Array<{ id?: string; provider?: string }>;
    };
  };
}

function dirOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? "" : path.slice(0, i);
}
function join(dir: string, ref: string): string {
  return dir ? `${dir}/${ref}` : ref;
}
function loadYaml<T>(content: string): T | null {
  try {
    return (yaml.load(content) as T) ?? null;
  } catch {
    return null;
  }
}
// Like loadYaml but reports whether parsing threw, so the caller can tell a
// malformed document (genuine error) apart from valid-but-empty YAML.
function tryLoadYaml<T>(content: string): { value: T | null; ok: boolean } {
  try {
    return { value: (yaml.load(content) as T) ?? null, ok: true };
  } catch {
    return { value: null, ok: false };
  }
}
function providerEntries(providers: NonNullable<ArenaConfigShape["spec"]>["providers"]): ProviderEntry[] {
  if (Array.isArray(providers)) return providers;
  if (providers) return Object.values(providers);
  return [];
}

/** Every project-relative file path the config refers to. [] if the config can't parse. */
export function referencedFiles(configPath: string, configContent: string): string[] {
  const cfg = loadYaml<ArenaConfigShape>(configContent);
  if (!cfg?.spec) return [];
  const dir = dirOf(configPath);
  const out: string[] = [];
  for (const p of cfg.spec.prompt_configs ?? []) if (p.file) out.push(join(dir, p.file));
  for (const p of providerEntries(cfg.spec.providers)) if (p.file) out.push(join(dir, p.file));
  for (const s of cfg.spec.scenarios ?? []) if (s.file) out.push(join(dir, s.file));
  for (const p of cfg.spec.self_play?.personas ?? []) if (p.file) out.push(join(dir, p.file));
  return out;
}

interface PromptFileShape {
  metadata?: { name?: string };
  spec?: {
    id?: string;
    name?: string;
    description?: string;
    system_template?: string;
    variables?: PromptDefinition["variables"];
    allowed_tools?: string[];
    tools?: string[];
  };
}
interface ProviderFileShape {
  metadata?: { name?: string };
  spec?: {
    id?: string;
    type?: string;
    model?: string;
    pricing?: { input_per_1k_tokens?: number; output_per_1k_tokens?: number };
  };
}
interface ScenarioFileShape {
  metadata?: { name?: string };
  spec?: {
    id?: string;
    description?: string;
    tags?: string[];
    assertions?: string[];
    turns?: unknown[];
  };
}
interface PersonaFileShape {
  metadata?: { name?: string };
  spec?: { id?: string };
}

function baseName(path: string): string {
  const f = path.split("/").pop() ?? path;
  return f.replace(/\.(prompt|provider|scenario|persona)?\.ya?ml$/i, "");
}

type ReadFile = (projectRelativePath: string) => string | undefined;

function buildPrompts(cfg: ArenaConfigShape, dir: string, readFile: ReadFile): Record<string, PromptDefinition> {
  const prompts: Record<string, PromptDefinition> = {};
  for (const ref of cfg.spec?.prompt_configs ?? []) {
    if (!ref.id) continue;
    const raw = ref.file ? readFile(join(dir, ref.file)) : undefined;
    const pf = raw ? loadYaml<PromptFileShape>(raw) : null;
    prompts[ref.id] = {
      id: ref.id,
      name: pf?.spec?.name || pf?.metadata?.name || ref.id,
      description: pf?.spec?.description,
      system_template: pf?.spec?.system_template,
      variables: pf?.spec?.variables,
      tools: pf?.spec?.allowed_tools ?? pf?.spec?.tools,
    };
  }
  return prompts;
}

function buildProviders(cfg: ArenaConfigShape, dir: string, readFile: ReadFile): ArenaParsedProvider[] {
  return providerEntries(cfg.spec?.providers).map((entry) => {
    const path = entry.file ? join(dir, entry.file) : undefined;
    const raw = path ? readFile(path) : undefined;
    const pf = raw ? loadYaml<ProviderFileShape>(raw) : null;
    const id = pf?.spec?.id || pf?.metadata?.name || (path ? baseName(path) : "provider");
    const pricing = pf?.spec?.pricing
      ? {
          inputPer1kTokens: pf.spec.pricing.input_per_1k_tokens,
          outputPer1kTokens: pf.spec.pricing.output_per_1k_tokens,
        }
      : undefined;
    return {
      id,
      model: pf?.spec?.model,
      providerType: pf?.spec?.type,
      group: entry.group,
      pricing,
      resolved: pf != null,
    };
  });
}

function buildScenarios(cfg: ArenaConfigShape, dir: string, readFile: ReadFile): ArenaParsedScenario[] {
  return (cfg.spec?.scenarios ?? []).map((ref) => {
    const path = ref.file ? join(dir, ref.file) : undefined;
    const raw = path ? readFile(path) : undefined;
    const sf = raw ? loadYaml<ScenarioFileShape>(raw) : null;
    return {
      id: sf?.spec?.id || sf?.metadata?.name || (path ? baseName(path) : "scenario"),
      description: sf?.spec?.description,
      turnCount: Array.isArray(sf?.spec?.turns) ? sf!.spec!.turns!.length : undefined,
      tags: sf?.spec?.tags,
      assertions: sf?.spec?.assertions,
    };
  });
}

function buildJudges(cfg: ArenaConfigShape): ArenaParsedJudge[] {
  const out: ArenaParsedJudge[] = [];
  for (const j of cfg.spec?.judges ?? []) {
    out.push({ id: j.name || "judge", provider: j.provider });
  }
  for (const [name, spec] of Object.entries(cfg.spec?.judge_specs ?? {})) {
    out.push({ id: name, provider: spec.provider });
  }
  return out;
}

function buildPersona(cfg: ArenaConfigShape, dir: string, readFile: ReadFile): ArenaParsedPersona | undefined {
  const sp = cfg.spec?.self_play;
  if (!sp?.enabled) return undefined;
  const ref = sp.personas?.[0];
  const role = sp.roles?.[0];
  const path = ref?.file ? join(dir, ref.file) : undefined;
  const raw = path ? readFile(path) : undefined;
  const pf = raw ? loadYaml<PersonaFileShape>(raw) : null;
  const id = pf?.spec?.id || pf?.metadata?.name || role?.id || (path ? baseName(path) : "persona");
  return { id, role: role?.id, provider: role?.provider };
}

export function parseArenaProject(input: {
  configPath: string;
  configContent: string;
  readFile: ReadFile;
}): { parsed: ArenaParsed | null; error: string | null } {
  const { configPath, configContent, readFile } = input;
  const { value, ok } = tryLoadYaml<ArenaConfigShape>(configContent);
  // Only malformed YAML is an error. Valid YAML with no `spec` (e.g. a brand-new
  // project's metadata-only config) is just an empty workload.
  if (!ok) return { parsed: null, error: "Could not parse arena config" };
  const cfg = value ?? {};
  const dir = dirOf(configPath);

  const content: PromptPackContent = {
    prompts: buildPrompts(cfg, dir, readFile),
    workflow: cfg.spec?.workflow,
    agents: cfg.spec?.agents,
  };
  return {
    parsed: {
      content,
      providers: buildProviders(cfg, dir, readFile),
      scenarios: buildScenarios(cfg, dir, readFile),
      judges: buildJudges(cfg),
      persona: buildPersona(cfg, dir, readFile),
    },
    error: null,
  };
}
