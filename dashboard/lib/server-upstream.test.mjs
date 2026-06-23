/**
 * Unit tests for resolveUpstreamTarget — the WS proxy helper that picks
 * a parked pod IP (via Redis route hint) or falls back to the Service.
 */

import { describe, it, expect } from "vitest";
import { resolveUpstreamTarget } from "./server-upstream.js";

const serviceTarget = { host: "agent.ns.svc.cluster.local", port: 8080 };

describe("resolveUpstreamTarget", () => {
  it("dials pod IP on route hit", async () => {
    /* eslint-disable sonarjs/no-hardcoded-ip */
    const podRoute = "10.0.0.5:8080";
    const expectedPod = { host: "10.0.0.5", port: 8080 };
    /* eslint-enable sonarjs/no-hardcoded-ip */
    const redis = { get: async () => podRoute };
    const t = await resolveUpstreamTarget(
      { resume: "sid", service: serviceTarget },
      redis,
      { timeoutMs: 200 },
    );
    expect(t).toEqual(expectedPod);
  });

  it("falls back to Service on miss", async () => {
    const redis = { get: async () => null };
    const t = await resolveUpstreamTarget(
      { resume: "sid", service: serviceTarget },
      redis,
      { timeoutMs: 200 },
    );
    expect(t).toEqual(serviceTarget);
  });

  it("falls back to Service on redis error (fail-open)", async () => {
    const redis = {
      get: async () => {
        throw new Error("down");
      },
    };
    const t = await resolveUpstreamTarget(
      { resume: "sid", service: serviceTarget },
      redis,
      { timeoutMs: 200 },
    );
    expect(t).toEqual(serviceTarget);
  });

  it("uses Service when no resume param", async () => {
    const t = await resolveUpstreamTarget(
      { resume: null, service: serviceTarget },
      null,
      { timeoutMs: 200 },
    );
    expect(t).toEqual(serviceTarget);
  });

  it("falls back to Service on timeout", async () => {
    const redis = { get: () => new Promise(() => {}) }; // never resolves
    const t = await resolveUpstreamTarget({ resume: "sid", service: serviceTarget }, redis, { timeoutMs: 10 });
    expect(t).toEqual(serviceTarget);
  });
});
