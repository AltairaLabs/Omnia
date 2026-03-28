/**
 * Pure utility functions for the job wizard.
 * Extracted for testability — these have no React dependencies.
 */

import type {
  ArenaProviderEntry,
  ArenaProviderGroup,
  ArenaJobSpec,
  ArenaJobType,
  LoadThreshold,
  LoadThresholdMetric,
  LoadThresholdOperator,
} from "@/types/arena";

export interface ProviderGroupEntry {
  type: "provider" | "agent";
  name: string;
  namespace?: string;
}

export interface ThresholdRow {
  metric: LoadThresholdMetric | "";
  operator: LoadThresholdOperator | "";
  value: string;
}

export interface JobWizardFormState {
  name: string;
  jobType: ArenaJobType;
  sourceRef: string;
  rootPath: string;
  arenaFileName: string;
  scenarioInclude: string[];
  providerGroups: Record<string, ProviderGroupEntry[]>;
  providerMappings: Record<string, Record<string, ProviderGroupEntry | null>>;
  selectedToolRegistries: string[];
  workers: string;
  verbose: boolean;
  // Load test fields
  trials: string;
  concurrency: string;
  vusPerWorker: string;
  rampUp: string;
  rampDown: string;
  budgetLimit: string;
  thresholds: ThresholdRow[];
}

export function getInitialFormState(
  preselectedSource?: string,
  defaultName?: string
): JobWizardFormState {
  return {
    name: defaultName || "",
    jobType: "evaluation",
    sourceRef: preselectedSource || "",
    rootPath: "",
    arenaFileName: "config.arena.yaml",
    scenarioInclude: [],
    providerGroups: {},
    providerMappings: {},
    selectedToolRegistries: [],
    workers: "1",
    verbose: false,
    trials: "",
    concurrency: "",
    vusPerWorker: "",
    rampUp: "",
    rampDown: "",
    budgetLimit: "",
    thresholds: [],
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

  if (form.jobType === "loadtest") {
    const loadErr = validateLoadTestFields(form);
    if (loadErr) return loadErr;
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

function buildProviders(form: JobWizardFormState): Record<string, ArenaProviderGroup> {
  const providers: Record<string, ArenaProviderGroup> = {};

  for (const [group, entries] of Object.entries(form.providerGroups)) {
    if (entries.length > 0) {
      providers[group] = entries.map(toProviderEntry);
    }
  }

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

  return providers;
}

export function buildSpec(form: JobWizardFormState): ArenaJobSpec {
  const arenaFile = buildArenaFilePath(form.rootPath, form.arenaFileName);
  const providers = buildProviders(form);
  const toolRegistries = form.selectedToolRegistries.map((name) => ({ name }));

  const spec: ArenaJobSpec = {
    sourceRef: { name: form.sourceRef },
    arenaFile: arenaFile || undefined,
    type: form.jobType,
    providers: Object.keys(providers).length > 0 ? providers : undefined,
    toolRegistries: toolRegistries.length > 0 ? toolRegistries : undefined,
    workers: {
      replicas: Number.parseInt(form.workers, 10),
    },
    verbose: form.verbose || undefined,
  };

  // Scenario filter
  if (form.scenarioInclude.length > 0) {
    spec.scenarios = { include: form.scenarioInclude };
  }

  if (form.jobType === "evaluation") {
    spec.evaluation = { outputFormats: ["json", "junit"] };
  } else if (form.jobType === "loadtest") {
    applyLoadTestSpec(spec, form);
  }

  return spec;
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

const DURATION_RE = /^\d+[smh]$/;

function parsePositiveInt(s: string): number {
  const n = Number.parseInt(s, 10);
  return Number.isNaN(n) || n < 0 ? 0 : n;
}

function validateLoadTestFields(form: JobWizardFormState): string | null {
  if (form.concurrency && parsePositiveInt(form.concurrency) < 1) {
    return "Concurrency must be a positive integer";
  }
  if (form.vusPerWorker && parsePositiveInt(form.vusPerWorker) < 1) {
    return "VUs per worker must be a positive integer";
  }
  if (form.rampUp && !DURATION_RE.test(form.rampUp)) {
    return "Ramp up must be a duration (e.g., 30s, 2m)";
  }
  if (form.rampDown && !DURATION_RE.test(form.rampDown)) {
    return "Ramp down must be a duration (e.g., 30s, 2m)";
  }
  if (form.budgetLimit && (Number.isNaN(Number(form.budgetLimit)) || Number(form.budgetLimit) <= 0)) {
    return "Budget limit must be a positive number";
  }
  for (const t of form.thresholds) {
    const partial = Boolean(t.metric) || Boolean(t.operator) || Boolean(t.value);
    if (partial && (!t.metric || !t.operator || !t.value)) {
      return "Each threshold must have a metric, operator, and value";
    }
  }
  return null;
}

function applyLoadTestSpec(spec: ArenaJobSpec, form: JobWizardFormState): void {
  const trials = parsePositiveInt(form.trials);
  if (trials > 0) spec.trials = trials;

  const ramp: Record<string, string> = {};
  if (form.rampUp) ramp.up = form.rampUp;
  if (form.rampDown) ramp.down = form.rampDown;

  spec.loadTest = {
    concurrency: parsePositiveInt(form.concurrency) || 1,
    vusPerWorker: parsePositiveInt(form.vusPerWorker) || 1,
    ramp: Object.keys(ramp).length > 0 ? ramp : undefined,
    budgetLimit: form.budgetLimit || undefined,
    budgetCurrency: form.budgetLimit ? "USD" : undefined,
    thresholds: buildThresholds(form.thresholds),
  };

  // Remove empty thresholds array
  if (spec.loadTest.thresholds?.length === 0) {
    spec.loadTest.thresholds = undefined;
  }
}

function buildThresholds(rows: ThresholdRow[]): LoadThreshold[] {
  return rows
    .filter((r) => r.metric && r.operator && r.value)
    .map((r) => ({
      metric: r.metric as LoadThresholdMetric,
      operator: r.operator as LoadThresholdOperator,
      value: r.value,
    }));
}

export const THRESHOLD_METRICS: { value: LoadThresholdMetric; label: string }[] = [
  { value: "latency_p50", label: "Latency p50" },
  { value: "latency_p90", label: "Latency p90" },
  { value: "latency_p95", label: "Latency p95" },
  { value: "latency_p99", label: "Latency p99" },
  { value: "latency_avg", label: "Latency avg" },
  { value: "ttft_p50", label: "TTFT p50" },
  { value: "ttft_p90", label: "TTFT p90" },
  { value: "ttft_p95", label: "TTFT p95" },
  { value: "ttft_p99", label: "TTFT p99" },
  { value: "ttft_avg", label: "TTFT avg" },
  { value: "error_rate", label: "Error rate" },
  { value: "pass_rate", label: "Pass rate" },
  { value: "total_cost", label: "Total cost" },
  { value: "rate_limit_rate", label: "Rate limit rate" },
];

export const THRESHOLD_OPERATORS: { value: LoadThresholdOperator; label: string }[] = [
  { value: "<", label: "<" },
  { value: "<=", label: "≤" },
  { value: ">", label: ">" },
  { value: ">=", label: "≥" },
];

/** Build a summary string for a provider group. */
export function groupSummary(entries: ProviderGroupEntry[]): string {
  const providers = entries.filter((e) => e.type === "provider").length;
  const agents = entries.filter((e) => e.type === "agent").length;
  const parts: string[] = [];
  if (providers > 0) parts.push(`${providers} provider${providers === 1 ? "" : "s"}`);
  if (agents > 0) parts.push(`${agents} agent${agents === 1 ? "" : "s"}`);
  return parts.join(", ");
}
