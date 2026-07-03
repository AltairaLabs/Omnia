import type { V1Pod, V1ContainerStatus } from "@kubernetes/client-node";
import { getWorkspace } from "./workspace-route-helpers";
import { getWorkspaceCoreApi, type WorkspaceClientOptions } from "./workspace-k8s-client-factory";
import type { ServiceGroupStatus } from "@/types/workspace";

export type ServiceState = "ready" | "crashlooping" | "pending" | "notDeployed" | "unknown";

export interface ServiceHealth {
  service: string;
  url?: string;
  state: ServiceState;
  ready: boolean;
  restarts: number;
  reason?: string;
}

export interface ServiceGroupHealth {
  name: string;
  ready: boolean;
  members: ServiceHealth[];
}

export interface WorkspaceServicesHealth {
  workspaceServices: ServiceHealth[];
  groups: ServiceGroupHealth[];
  source: "crd" | "envFallback" | "unknown";
}

const REASON_MAX_LEN = 200;

/**
 * Determines whether a container status indicates a crash loop, per the rule:
 * waiting.reason === "CrashLoopBackOff", or the container has restarted and is
 * not currently ready.
 */
function isCrashlooping(cs: V1ContainerStatus, restarts: number): boolean {
  const waitingReason = cs.state?.waiting?.reason;
  return waitingReason === "CrashLoopBackOff" || (restarts > 0 && !cs.ready);
}

/**
 * Builds a single-line reason string for a crashlooping container from its
 * lastState.terminated info, falling back to the current waiting message.
 * Truncated to keep the surfaced reason readable.
 */
function reasonFrom(cs: V1ContainerStatus): string | undefined {
  const terminated = cs.lastState?.terminated;
  const waiting = cs.state?.waiting;
  const detail = terminated?.message || waiting?.message;
  const reasonPrefix = terminated?.reason || waiting?.reason;

  if (!reasonPrefix && !detail) {
    return undefined;
  }

  const reason = reasonPrefix && detail ? `${reasonPrefix}: ${detail}` : reasonPrefix || detail;
  return reason && reason.length > REASON_MAX_LEN ? `${reason.slice(0, REASON_MAX_LEN)}...` : reason;
}

/**
 * Builds the reason for a pending (not-ready, not-crashlooping) container
 * from its waiting state.
 */
function reasonForPending(cs: V1ContainerStatus): string | undefined {
  const waiting = cs.state?.waiting;
  if (!waiting) {
    return undefined;
  }
  const reason = waiting.reason && waiting.message ? `${waiting.reason}: ${waiting.message}` : waiting.reason || waiting.message;
  return reason && reason.length > REASON_MAX_LEN ? `${reason.slice(0, REASON_MAX_LEN)}...` : reason;
}

function stateFor(phase: string | undefined, ready: boolean, crashlooping: boolean, hasContainerStatuses: boolean): ServiceState {
  if (crashlooping) return "crashlooping";
  if (ready) return "ready";
  if (phase === "Unknown" || !hasContainerStatuses) return "unknown";
  return "pending";
}

/**
 * Maps a Kubernetes pod list for a service to a pure ServiceHealth summary.
 * Only the first pod's status is consulted, matching the mapper's contract.
 */
export function podHealthFromStatus(pods: V1Pod[], service: string, url?: string): ServiceHealth {
  if (pods.length === 0) {
    return { service, url, state: "notDeployed", ready: false, restarts: 0 };
  }

  const status = pods[0].status;
  const containerStatuses = status?.containerStatuses ?? [];
  const restarts = containerStatuses.reduce((sum, cs) => sum + (cs.restartCount ?? 0), 0);
  const ready = containerStatuses.length > 0 && containerStatuses.every((cs) => cs.ready);

  const crashedContainer = containerStatuses.find((cs) => isCrashlooping(cs, cs.restartCount ?? 0));
  const crashlooping = crashedContainer !== undefined;

  const state = stateFor(status?.phase, ready, crashlooping, containerStatuses.length > 0);

  let reason: string | undefined;
  if (crashlooping && crashedContainer) {
    reason = reasonFrom(crashedContainer);
  } else if (state === "pending") {
    const pendingContainer = containerStatuses.find((cs) => !cs.ready);
    reason = pendingContainer ? reasonForPending(pendingContainer) : undefined;
  }

  return { service, url, state, ready, restarts, reason };
}

const SESSION_COMPONENT = "session-api";
const MEMORY_COMPONENT = "memory-api";
const PRIVACY_COMPONENT = "privacy-api";
const DEFAULT_SERVICE_GROUP = "default";
const SERVICE_GROUP_LABEL = "omnia.altairalabs.ai/service-group";

function componentSelector(component: string): string {
  return `app.kubernetes.io/component=${component}`;
}

function groupSelector(component: string, groupName: string): string {
  return `${componentSelector(component)},${SERVICE_GROUP_LABEL}=${groupName}`;
}

/**
 * Lists pods for a workspace by label selector. Errors propagate to the
 * caller — getServiceHealth is the single place that translates a thrown
 * error into an "unknown" health result.
 */
async function listPodsForSelector(options: WorkspaceClientOptions, labelSelector: string): Promise<V1Pod[]> {
  const coreApi = await getWorkspaceCoreApi(options);
  const result = await coreApi.listNamespacedPod({ namespace: options.namespace, labelSelector });
  return result.items ?? [];
}

/**
 * Builds the health of one service group (session-api + memory-api pair).
 */
async function buildGroupHealth(options: WorkspaceClientOptions, group: ServiceGroupStatus): Promise<ServiceGroupHealth> {
  const [sessionPods, memoryPods] = await Promise.all([
    listPodsForSelector(options, groupSelector(SESSION_COMPONENT, group.name)),
    listPodsForSelector(options, groupSelector(MEMORY_COMPONENT, group.name)),
  ]);

  return {
    name: group.name,
    ready: group.ready,
    members: [
      podHealthFromStatus(sessionPods, SESSION_COMPONENT, group.sessionURL),
      podHealthFromStatus(memoryPods, MEMORY_COMPONENT, group.memoryURL),
    ],
  };
}

/**
 * Builds the health of the workspace-level privacy-api. Unlike session-api
 * and memory-api, privacy-api has no service-group label — it's one per
 * workspace.
 */
async function buildPrivacyHealth(options: WorkspaceClientOptions, privacyURL?: string): Promise<ServiceHealth> {
  const pods = await listPodsForSelector(options, componentSelector(PRIVACY_COMPONENT));
  return podHealthFromStatus(pods, PRIVACY_COMPONENT, privacyURL);
}

/**
 * Best-effort service group built from SESSION_API_URL/MEMORY_API_URL env
 * vars, used when the Workspace CRD has no status.services yet (e.g. local
 * dev without the workspace controller). Returns null when either var is
 * unset.
 */
function envFallbackGroups(): ServiceGroupStatus[] | null {
  const sessionURL = process.env.SESSION_API_URL;
  const memoryURL = process.env.MEMORY_API_URL;
  if (!sessionURL || !memoryURL) return null;
  return [{ name: DEFAULT_SERVICE_GROUP, sessionURL, memoryURL, ready: false }];
}

/**
 * Picks which service groups to health-check and what source they came
 * from: CRD status.services when present, else the env fallback, else null
 * (nothing to check).
 */
function resolveActiveGroups(
  groupsStatus: ServiceGroupStatus[] | undefined
): { groups: ServiceGroupStatus[]; source: "crd" | "envFallback" } | null {
  if (groupsStatus && groupsStatus.length > 0) {
    return { groups: groupsStatus, source: "crd" };
  }
  const fallback = envFallbackGroups();
  return fallback ? { groups: fallback, source: "envFallback" } : null;
}

function unknownServiceHealth(service: string): ServiceHealth {
  return { service, state: "unknown", ready: false, restarts: 0 };
}

function unknownGroupHealth(name: string): ServiceGroupHealth {
  return {
    name,
    ready: false,
    members: [unknownServiceHealth(SESSION_COMPONENT), unknownServiceHealth(MEMORY_COMPONENT)],
  };
}

/**
 * Assembles per-service health for a workspace: one entry per service group
 * (session-api + memory-api) plus the workspace-level privacy-api. Reads the
 * Workspace CRD for group config, then lists pods per group via label
 * selectors and maps them with podHealthFromStatus.
 *
 * Never throws — a k8s API failure at any point degrades to "unknown"
 * members with source:"unknown" rather than propagating to the caller.
 */
export async function getServiceHealth(
  options: WorkspaceClientOptions,
  workspaceName: string
): Promise<WorkspaceServicesHealth> {
  let groupsStatus: ServiceGroupStatus[] | undefined;
  try {
    const workspace = await getWorkspace(workspaceName);
    groupsStatus = workspace?.status?.services;

    const active = resolveActiveGroups(groupsStatus);
    if (!active) {
      return { workspaceServices: [], groups: [], source: "unknown" };
    }

    const groups = await Promise.all(active.groups.map((group) => buildGroupHealth(options, group)));
    const privacy = await buildPrivacyHealth(options, workspace?.status?.privacyURL);
    return { workspaceServices: [privacy], groups, source: active.source };
  } catch {
    const groupNames = groupsStatus?.map((group) => group.name) ?? [DEFAULT_SERVICE_GROUP];
    return {
      workspaceServices: [unknownServiceHealth(PRIVACY_COMPONENT)],
      groups: groupNames.map(unknownGroupHealth),
      source: "unknown",
    };
  }
}
