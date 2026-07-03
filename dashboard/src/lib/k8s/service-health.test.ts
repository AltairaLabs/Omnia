/* eslint-disable sonarjs/no-clear-text-protocols */
import { describe, it, expect } from "vitest";
import { podHealthFromStatus } from "./service-health";

const ready = { status: { phase: "Running", containerStatuses: [{ ready: true, restartCount: 0, state: { running: {} } }] } } as any;
const crash = { status: { phase: "Running", containerStatuses: [{ ready: false, restartCount: 12,
  state: { waiting: { reason: "CrashLoopBackOff", message: "back-off restarting" } },
  lastState: { terminated: { reason: "Error", message: "ensure embedding schema: consent required" } } }] } } as any;
const pending = { status: { phase: "Pending", containerStatuses: [{ ready: false, restartCount: 0,
  state: { waiting: { reason: "ContainerCreating" } } }] } } as any;
const unknownPhase = { status: { phase: "Unknown" } } as any;
const emptyContainerStatuses = { status: { phase: "Running", containerStatuses: [] } } as any;
const pendingNoWaiting = { status: { phase: "Pending", containerStatuses: [{ ready: false, restartCount: 0, state: {} }] } } as any;
const crashNoDetail = { status: { phase: "Running", containerStatuses: [{ ready: false, restartCount: 1, state: {} }] } } as any;

describe("podHealthFromStatus", () => {
  it("healthy pod → ready", () => {
    expect(podHealthFromStatus([ready], "session-api", "http://s:8080"))
      .toMatchObject({ service: "session-api", state: "ready", ready: true, restarts: 0, url: "http://s:8080" });
  });
  it("crashlooping pod → surfaces reason + restarts", () => {
    const h = podHealthFromStatus([crash], "memory-api");
    expect(h.state).toBe("crashlooping"); expect(h.ready).toBe(false); expect(h.restarts).toBe(12);
    expect(h.reason).toContain("Error"); expect(h.reason).toContain("ensure embedding schema");
  });
  it("no pods → notDeployed", () => {
    expect(podHealthFromStatus([], "privacy-api")).toMatchObject({ state: "notDeployed", ready: false, restarts: 0 });
  });
  it("pending pod (ContainerCreating) → pending", () => {
    const h = podHealthFromStatus([pending], "runtime");
    expect(h.state).toBe("pending");
    expect(h.ready).toBe(false);
  });
  it("Unknown phase pod with no container statuses → unknown", () => {
    const h = podHealthFromStatus([unknownPhase], "facade");
    expect(h.state).toBe("unknown");
    expect(h.ready).toBe(false);
  });
  it("Running phase pod with empty container statuses → unknown", () => {
    const h = podHealthFromStatus([emptyContainerStatuses], "policy-proxy");
    expect(h.state).toBe("unknown");
    expect(h.ready).toBe(false);
  });
  it("pending container with no waiting state → no reason surfaced", () => {
    const h = podHealthFromStatus([pendingNoWaiting], "runtime");
    expect(h.state).toBe("pending");
    expect(h.reason).toBeUndefined();
  });
  it("crashlooping container with no waiting/terminated detail → no reason surfaced", () => {
    const h = podHealthFromStatus([crashNoDetail], "memory-api");
    expect(h.state).toBe("crashlooping");
    expect(h.reason).toBeUndefined();
  });
});
