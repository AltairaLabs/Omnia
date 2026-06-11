import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useOpenAPIToolPreview } from "./use-openapi-tool-preview";

beforeEach(() => vi.restoreAllMocks());

describe("useOpenAPIToolPreview", () => {
  it("loads tools for an openapi handler", async () => {
    vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ tools: [{ name: "getPet", description: "d" }], specURL: "u" }), { status: 200 })
    );
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", "petstore"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tools).toHaveLength(1);
    expect(result.current.error).toBeNull();
  });

  it("surfaces a discovery error payload", async () => {
    vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ tools: [], error: "failed to fetch OpenAPI spec", specURL: "u" }), { status: 422 })
    );
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", "petstore"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tools).toHaveLength(0);
    expect(result.current.error).toMatch(/failed to fetch/i);
  });

  it("does not fetch when handler name is empty", async () => {
    const fetchMock = vi.spyOn(global, "fetch");
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", ""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("surfaces a network error from fetch rejection", async () => {
    vi.spyOn(global, "fetch").mockRejectedValue(new Error("network failure"));
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", "petstore"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tools).toHaveLength(0);
    expect(result.current.error).toMatch(/network failure/i);
  });

  it("falls back to generic message for non-Error rejections", async () => {
    vi.spyOn(global, "fetch").mockRejectedValue("raw string error");
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", "petstore"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("Failed to load tools");
  });

  it("handles a response with missing optional fields", async () => {
    vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ tools: [] }), { status: 200 })
    );
    const { result } = renderHook(() => useOpenAPIToolPreview("ws", "reg", "petstore"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.tools).toHaveLength(0);
    expect(result.current.specURL).toBeNull();
    expect(result.current.error).toBeNull();
  });
});
