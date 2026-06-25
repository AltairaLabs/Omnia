import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useToolRegistryMutations, ResourceUpdateError } from "./use-tool-registry-mutations";
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

  it("updateToolRegistry PUTs the body to the item route and returns the saved resource", async () => {
    const saved = { metadata: { name: "gh" }, spec: { handlers: [] } };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => saved,
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useToolRegistryMutations());
    let returned: unknown;
    await act(async () => {
      returned = await result.current.updateToolRegistry("gh", {
        metadata: { resourceVersion: "42" },
        spec: { handlers: [] },
      });
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/workspaces/ws1/toolregistries/gh",
      expect.objectContaining({ method: "PUT" })
    );
    const sentBody = JSON.parse(fetchMock.mock.calls[0][1].body);
    expect(sentBody.metadata.resourceVersion).toBe("42");
    expect(returned).toEqual(saved);
  });

  it("updateToolRegistry throws ResourceUpdateError carrying status and API message", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: async () => ({ error: "Request rejected", message: "the object has been modified" }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useToolRegistryMutations());
    await expect(
      result.current.updateToolRegistry("gh", { metadata: {}, spec: {} })
    ).rejects.toMatchObject({ status: 409, message: expect.stringContaining("modified") });
  });

  it("updateToolRegistry throws when no workspace is selected", async () => {
    mockWorkspace = undefined;
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useToolRegistryMutations());
    await expect(
      result.current.updateToolRegistry("gh", { metadata: {}, spec: {} })
    ).rejects.toThrow(/workspace/i);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("ResourceUpdateError is an Error subclass with a status", () => {
    const err = new ResourceUpdateError(422, "bad");
    expect(err).toBeInstanceOf(Error);
    expect(err.status).toBe(422);
    expect(err.message).toBe("bad");
  });
});
