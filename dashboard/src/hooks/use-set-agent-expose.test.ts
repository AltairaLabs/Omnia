import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSetAgentExpose } from "./use-set-agent-expose";

describe("useSetAgentExpose (#1611)", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("PATCHes the expose route with trimmed host and returns true on success", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useSetAgentExpose("ws1", "a1"));
    let ok: boolean | undefined;
    await act(async () => {
      ok = await result.current.save(true, "  agent.example.com  ");
    });

    expect(ok).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces/ws1/agents/a1/expose",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: true, host: "agent.example.com" }),
      })
    );
    expect(result.current.error).toBeNull();
  });

  it("surfaces the server error message and returns false on failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, json: async () => ({ error: "editor access required" }) })
    );

    const { result } = renderHook(() => useSetAgentExpose("ws1", "a1"));
    let ok: boolean | undefined;
    await act(async () => {
      ok = await result.current.save(false, "");
    });

    expect(ok).toBe(false);
    expect(result.current.error).toBe("editor access required");
  });
});
