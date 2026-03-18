/**
 * Pure utility functions for the job wizard.
 * Extracted for testability — these have no React dependencies.
 */

import type {
  ArenaProviderEntry,
  ArenaProviderGroup,
  ArenaJobSpec,
} from "@/types/arena";

export interface ProviderGroupEntry {
  type: "provider" | "agent";
  name: string;
  namespace?: string;
}

export interface JobWizardFormState {
  name: string;
  sourceRef: string;
  rootPath: string;
  arenaFileName: string;
  providerGroups: Record<string, ProviderGroupEntry[]>;
  providerMappings: Record<string, Record<string, ProviderGroupEntry | null>>;
  selectedToolRegistries: string[];
  workers: string;
  verbose: boolean;
}

export function getInitialFormState(
  preselectedSource?: string,
  defaultName?: string
): JobWizardFormState {
  return {
    name: defaultName || "",
    sourceRef: preselectedSource || "",
    rootPath: "",
    arenaFileName: "config.arena.yaml",
    providerGroups: {},
    providerMappings: {},
    selectedToolRegistries: [],
    workers: "1",
    verbose: false,
  };
}

export function validateProviderMappings(
  mappings: Record<string, Record<string, ProviderGroupEntry | null>>
): string | null {
  for (const [group, mapping] of Object.entries(mappings)) {
    for (const [configID, entry] of Object.entries(mapping)) {
      if (!entry) {
        return `Provider mapping "${configID}" in group "${group}" requires a selection`;
      }
    }
  }
  return null;
}

export function validateProviderGroups(
  form: JobWizardFormState,
  requiredGroups: string[]
): string | null {
  for (const group of requiredGroups) {
    if (group in form.providerMappings) continue;
    const entries = form.providerGroups[group] || [];
    if (entries.length === 0) {
      return `Provider group "${group}" is required by the arena config but has no entries`;
    }
  }
  return validateProviderMappings(form.providerMappings);
}

export function validateForm(
  form: JobWizardFormState,
  maxWorkerReplicas: number,
  requiredGroups: string[]
): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(form.name)) {
    return "Name must be lowercase alphanumeric and may contain hyphens";
  }
  if (!form.sourceRef) {
    return "Source is required";
  }

  const providerError = validateProviderGroups(form, requiredGroups);
  if (providerError) return providerError;

  const workers = Number.parseInt(form.workers, 10);
  if (Number.isNaN(workers) || workers < 1) {
    return "Workers must be a positive integer";
  }

  if (maxWorkerReplicas > 0 && workers > maxWorkerReplicas) {
    return `Open Core is limited to ${maxWorkerReplicas} worker(s)`;
  }

  return null;
}

export function buildArenaFilePath(rootPath: string, fileName: string): string | undefined {
  if (!rootPath && !fileName) return undefined;
  if (!rootPath) return fileName;
  if (!fileName) return `${rootPath}/config.arena.yaml`;
  return `${rootPath}/${fileName}`;
}

export function toProviderEntry(e: ProviderGroupEntry): ArenaProviderEntry {
  return e.type === "agent"
    ? { agentRef: { name: e.name } }
    : { providerRef: { name: e.name, namespace: e.namespace } };
}

export function buildSpec(form: JobWizardFormState): ArenaJobSpec {
  const arenaFile = buildArenaFilePath(form.rootPath, form.arenaFileName);

  const providers: Record<string, ArenaProviderGroup> = {};

  // Array-mode groups
  for (const [group, entries] of Object.entries(form.providerGroups)) {
    if (entries.length > 0) {
      providers[group] = entries.map(toProviderEntry);
    }
  }

  // Map-mode groups
  for (const [group, mapping] of Object.entries(form.providerMappings)) {
    const mapEntries: Record<string, ArenaProviderEntry> = {};
    let hasEntries = false;
    for (const [configID, entry] of Object.entries(mapping)) {
      if (entry) {
        mapEntries[configID] = toProviderEntry(entry);
        hasEntries = true;
      }
    }
    if (hasEntries) {
      providers[group] = mapEntries;
    }
  }

  const toolRegistries = form.selectedToolRegistries.map((name) => ({ name }));

  return {
    sourceRef: { name: form.sourceRef },
    arenaFile: arenaFile || undefined,
    type: "evaluation",
    providers: Object.keys(providers).length > 0 ? providers : undefined,
    toolRegistries: toolRegistries.length > 0 ? toolRegistries : undefined,
    workers: {
      replicas: Number.parseInt(form.workers, 10),
    },
    verbose: form.verbose || undefined,
    evaluation: {
      outputFormats: ["json", "junit"],
    },
  };
}

/**
 * Count provider/agent entries that participate in the scenario × provider matrix.
 * Only array-mode groups (test provider pools) are counted — map-mode groups
 * (judges, self-play) are 1:1 config references that don't multiply with scenarios.
 */
export function countTotalEntries(
  groups: Record<string, ProviderGroupEntry[]>,
): number {
  return Object.values(groups).reduce((sum, entries) => sum + entries.length, 0);
}

export function getStepIndicatorClassName(stepIndex: number, currentStep: number): string {
  if (stepIndex < currentStep) return "bg-primary text-primary-foreground";
  if (stepIndex === currentStep) return "border-2 border-primary";
  return "border border-muted-foreground/30";
}

/** Build a summary string for a provider group. */
export function groupSummary(entries: ProviderGroupEntry[]): string {
  const providers = entries.filter((e) => e.type === "provider").length;
  const agents = entries.filter((e) => e.type === "agent").length;
  const parts: string[] = [];
  if (providers > 0) parts.push(`${providers} provider${providers === 1 ? "" : "s"}`);
  if (agents > 0) parts.push(`${agents} agent${agents === 1 ? "" : "s"}`);
  return parts.join(", ");
}
