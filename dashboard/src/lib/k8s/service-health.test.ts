/* eslint-disable sonarjs/no-clear-text-protocols */
import { describe, it, expect } from "vitest";
import { podHealthFromStatus } from "./service-health";

const ready = { status: { phase: "Running", containerStatuses: [{ ready: true, restartCount: 0, state: { running: {} } }] } } as any;
const crash = { status: { phase: "Running", containerStatuses: [{ ready: false, restartCount: 12,
  state: { waiting: { reason: "CrashLoopBackOff", message: "back-off restarting" } },
  lastState: { terminated: { reason: "Error", message: "ensure embedding schema: consent required" } } }] } } as any;

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
});
