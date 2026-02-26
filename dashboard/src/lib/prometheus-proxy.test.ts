import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";
import { missingParamResponse } from "./prometheus-proxy";

const TEST_PROM_URL = "https://prom.example.com:9090";
const TEST_PROM_URL_TRAILING = "https://prom.example.com:9090/";

// Helper to create NextRequest with search params
function makeRequest(params: Record<string, string> = {}): NextRequest {
  const url = new URL("https://localhost/api/prometheus/query");
  for (const [key, value] of Object.entries(params)) {
    url.searchParams.set(key, value);
  }
  return new NextRequest(url);
}

describe("createPrometheusProxy", () => {
  const originalEnv = process.env.PROMETHEUS_URL;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    if (originalEnv === undefined) {
      delete process.env.PROMETHEUS_URL;
    } else {
      process.env.PROMETHEUS_URL = originalEnv;
    }
  });

  it("returns 503 when PROMETHEUS_URL is not set", async () => {
    delete process.env.PROMETHEUS_URL;

    // Re-import to pick up env change
    vi.resetModules();
    const { createPrometheusProxy: factory } = await import("./prometheus-proxy");

    const handler = factory({
      endpoint: "query",
      extractParams: () => ({ params: { query: "up" } }),
    });

    const response = await handler(makeRequest());
    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.errorType).toBe("configuration");
  });

  it("returns extractParams error when params invalid", async () => {
    process.env.PROMETHEUS_URL = TEST_PROM_URL;
    vi.resetModules();
    const mod = await import("./prometheus-proxy");

    const handler = mod.createPrometheusProxy({
      endpoint: "query",
      extractParams: () => ({
        error: mod.missingParamResponse("Missing required parameter: query"),
      }),
    });

    const response = await handler(makeRequest());
    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toBe("Missing required parameter: query");
  });

  it("proxies to Prometheus and returns response", async () => {
    process.env.PROMETHEUS_URL = TEST_PROM_URL;
    vi.resetModules();
    const { createPrometheusProxy: factory } = await import("./prometheus-proxy");

    const mockData = { status: "success", data: { resultType: "vector", result: [] } };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockData), { status: 200 }),
    );

    const handler = factory({
      endpoint: "query",
      extractParams: () => ({ params: { query: "up" } }),
    });

    const response = await handler(makeRequest({ query: "up" }));
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.status).toBe("success");

    expect(fetchSpy).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/query"),
      expect.objectContaining({ headers: { Accept: "application/json" } }),
    );
    fetchSpy.mockRestore();
  });

  it("returns 504 on timeout (AbortError)", async () => {
    process.env.PROMETHEUS_URL = TEST_PROM_URL;
    vi.resetModules();
    const { createPrometheusProxy: factory } = await import("./prometheus-proxy");

    const abortError = new DOMException("The operation was aborted", "AbortError");
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockRejectedValue(abortError);

    const handler = factory({
      endpoint: "query",
      extractParams: () => ({ params: { query: "up" } }),
    });

    const response = await handler(makeRequest({ query: "up" }));
    expect(response.status).toBe(504);
    const body = await response.json();
    expect(body.errorType).toBe("timeout");
    fetchSpy.mockRestore();
  });

  it("returns 502 on connection error", async () => {
    process.env.PROMETHEUS_URL = TEST_PROM_URL;
    vi.resetModules();
    const { createPrometheusProxy: factory } = await import("./prometheus-proxy");

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("ECONNREFUSED"));
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const handler = factory({
      endpoint: "query",
      extractParams: () => ({ params: { query: "up" } }),
    });

    const response = await handler(makeRequest({ query: "up" }));
    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.errorType).toBe("internal");
    expect(body.details).toBe("ECONNREFUSED");
    fetchSpy.mockRestore();
    consoleSpy.mockRestore();
  });

  it("strips trailing slash from PROMETHEUS_URL", async () => {
    process.env.PROMETHEUS_URL = TEST_PROM_URL_TRAILING;
    vi.resetModules();
    const { createPrometheusProxy: factory } = await import("./prometheus-proxy");

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ status: "success" }), { status: 200 }),
    );

    const handler = factory({
      endpoint: "query_range",
      extractParams: () => ({ params: { query: "up", start: "0", end: "1", step: "1h" } }),
    });

    await handler(makeRequest());
    const calledUrl = fetchSpy.mock.calls[0][0] as string;
    expect(calledUrl).toContain(`${TEST_PROM_URL}/api/v1/query_range`);
    expect(calledUrl).not.toContain("//api");
    fetchSpy.mockRestore();
  });
});

describe("missingParamResponse", () => {
  it("returns 400 with correct error structure", async () => {
    const response = missingParamResponse("Missing required parameter: query");
    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.errorType).toBe("bad_data");
    expect(body.error).toBe("Missing required parameter: query");
  });
});
