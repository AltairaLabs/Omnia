import type { V1Pod, V1ContainerStatus } from "@kubernetes/client-node";

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

function stateFor(phase: string | undefined, ready: boolean, crashlooping: boolean): ServiceState {
  if (crashlooping) return "crashlooping";
  if (ready) return "ready";
  if (phase === "Pending" || !ready) return "pending";
  return "unknown";
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

  const state = stateFor(status?.phase, ready, crashlooping);

  let reason: string | undefined;
  if (crashlooping && crashedContainer) {
    reason = reasonFrom(crashedContainer);
  } else if (state === "pending") {
    const pendingContainer = containerStatuses.find((cs) => !cs.ready);
    reason = pendingContainer ? reasonForPending(pendingContainer) : undefined;
  }

  return { service, url, state, ready, restarts, reason };
}
