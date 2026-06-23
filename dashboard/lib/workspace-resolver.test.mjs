/**
 * Unit tests for the WS-proxy workspace-name resolver.
 *
 * The facade validates the mgmt-plane JWT's `workspace` claim against the
 * value it computes via Go `ResolveWorkspaceName`: the AgentRuntime's
 * `omnia.altairalabs.ai/workspace` label, falling back to the namespace's
 * label. The dashboard proxy only knows the K8s namespace + agent name from
 * the WS path, so it must perform the same resolution before minting — these
 * tests pin that resolution logic (the in-cluster API lookups are injected).
 */

import { describe, it, expect, vi } from "vitest";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { resolveWorkspaceName, WORKSPACE_LABEL } = require("./workspace-resolver.js");

// Namespace != workspace name — the whole point of the resolver (#1552).
const NS = "omnia-demo";
const AGENT = "rag-hero";
const WS_NAME = "demo";

describe("resolveWorkspaceName", () => {
  it("uses the AgentRuntime's workspace label when present", async () => {
    const namespaceLabel = vi.fn();
    const ws = await resolveWorkspaceName(NS, AGENT, {
      agentRuntimeLabel: async () => WS_NAME,
      namespaceLabel,
    });
    expect(ws).toBe(WS_NAME);
    // The namespace fallback must not be consulted once the resource resolves.
    expect(namespaceLabel).not.toHaveBeenCalled();
  });

  it("falls back to the namespace label when the AgentRuntime has no label", async () => {
    const ws = await resolveWorkspaceName(NS, AGENT, {
      agentRuntimeLabel: async () => "",
      namespaceLabel: async (ns) => (ns === NS ? WS_NAME : ""),
    });
    expect(ws).toBe(WS_NAME);
  });

  it("returns empty string when neither the resource nor the namespace is labelled", async () => {
    const ws = await resolveWorkspaceName(NS, AGENT, {
      agentRuntimeLabel: async () => "",
      namespaceLabel: async () => "",
    });
    expect(ws).toBe("");
  });

  it("propagates lookup errors so the caller can decide the fallback", async () => {
    await expect(
      resolveWorkspaceName(NS, AGENT, {
        agentRuntimeLabel: async () => {
          throw new Error("apiserver down");
        },
        namespaceLabel: async () => WS_NAME,
      }),
    ).rejects.toThrow("apiserver down");
  });

  it("exports the canonical workspace label constant", () => {
    expect(WORKSPACE_LABEL).toBe("omnia.altairalabs.ai/workspace");
  });

  it("defaults to the in-cluster lookups when none are injected", async () => {
    // No opts → the real in-cluster lookups are selected. Outside a cluster
    // (no KUBERNETES_SERVICE_HOST) they reject, which proves the defaults are
    // wired without depending on a live apiserver.
    const saved = process.env.KUBERNETES_SERVICE_HOST;
    delete process.env.KUBERNETES_SERVICE_HOST;
    try {
      await expect(resolveWorkspaceName(NS, AGENT)).rejects.toThrow(
        /in-cluster API server/,
      );
    } finally {
      if (saved !== undefined) {
        process.env.KUBERNETES_SERVICE_HOST = saved;
      }
    }
  });
});
