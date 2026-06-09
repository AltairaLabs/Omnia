import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useToolRegistryMutations } from "./use-tool-registry-mutations";
import type { ToolRegistrySpec } from "@/types/tool-registry";

let mockWorkspace: { name: string } | undefined = { name: "ws1" };

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: mockWorkspace }),
}));

const SPEC: ToolRegistrySpec = { handlers: [] };

describe("useToolRegistryMutations", () => {
  beforeEach(() => {
    mockWorkspace = { name: "ws1" };
    vi.restoreAllMocks();
  });

  it("POSTs metadata + spec to the workspace toolregistries route", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ metadata: { name: "r1" } }) });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useToolRegistryMutations());
    await act(async () => {
      await result.current.createToolRegistry("r1", SPEC);
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces/ws1/toolregistries",
      expect.objectContaining({ method: "POST" })
    );
    const body = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(body).toEqual({ metadata: { name: "r1" }, spec: SPEC });
  });

  it("throws with the response text on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, text: async () => "boom" }));
    const { result } = renderHook(() => useToolRegistryMutations());
    await expect(result.current.createToolRegistry("r1", SPEC)).rejects.toThrow("boom");
  });

  it("throws when no workspace is selected", async () => {
    mockWorkspace = undefined;
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useToolRegistryMutations());
    await expect(result.current.createToolRegistry("r1", SPEC)).rejects.toThrow(/workspace/i);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
