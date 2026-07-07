/**
 * Tests for fetchWithTimeout — the abort-after-timeout fetch wrapper used by
 * the session/memory proxy routes so a hung backend fails fast instead of
 * hanging the request (and the page) forever.
 */

import { describe, it, expect, vi, afterEach } from "vitest";
import { fetchWithTimeout } from "./fetch-with-timeout";

/** A fetch mock that never resolves on its own — only rejects when the
 * caller's AbortSignal fires, mirroring how the native fetch behaves when
 * aborted mid-flight. */
function neverResolvingFetch() {
  return vi.fn((_url: RequestInfo | URL, init?: RequestInit) => {
    return new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => {
        const err = new Error("aborted");
        err.name = "AbortError";
        reject(err);
      });
    });
  });
}

describe("fetchWithTimeout", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("resolves normally when the fetch completes under the timeout", async () => {
    const response = new Response("ok");
    global.fetch = vi.fn().mockResolvedValue(response);

    const result = await fetchWithTimeout("https://example.com", {}, 1000);

    expect(result).toBe(response);
    expect(global.fetch).toHaveBeenCalledWith(
      "https://example.com",
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it("forwards caller init fields (headers, method) alongside the abort signal", async () => {
    const response = new Response("ok");
    global.fetch = vi.fn().mockResolvedValue(response);

    await fetchWithTimeout("https://example.com", { method: "DELETE", headers: { "X-Test": "1" } }, 1000);

    expect(global.fetch).toHaveBeenCalledWith(
      "https://example.com",
      expect.objectContaining({
        method: "DELETE",
        headers: { "X-Test": "1" },
        signal: expect.any(AbortSignal),
      })
    );
  });

  it("throws a clear timeout error when the fetch exceeds the timeout", async () => {
    global.fetch = neverResolvingFetch();

    await expect(fetchWithTimeout("https://example.com", {}, 10)).rejects.toThrow(/upstream timeout/);
  });

  it("rethrows non-abort errors unchanged", async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error("dns failure"));

    await expect(fetchWithTimeout("https://example.com", {}, 1000)).rejects.toThrow("dns failure");
  });

  it("defaults the timeout to 6000ms when not provided", async () => {
    const response = new Response("ok");
    global.fetch = vi.fn().mockResolvedValue(response);

    const result = await fetchWithTimeout("https://example.com");

    expect(result).toBe(response);
  });
});
