/**
 * Tests for the deprecated operator proxy route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// Mock the audit logger before importing the route
vi.mock("@/lib/audit", () => ({
  logProxyUsage: vi.fn(),
  logWarn: vi.fn(),
  logError: vi.fn(),
}));

// Store original env
const originalEnv = { ...process.env };

describe("operator proxy route", () => {
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>;
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.resetModules();
    consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    // Reset env
    delete process.env.OMNIA_PROXY_MODE;
  });

  afterEach(() => {
    consoleWarnSpy.mockRestore();
    consoleErrorSpy.mockRestore();
    process.env = { ...originalEnv };
    vi.resetAllMocks();
  });

  describe("strict mode (default)", () => {
    it("returns 410 Gone for GET requests when proxy is disabled", async () => {
      // Import fresh module with default strict mode
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await GET(request, context);

      expect(response.status).toBe(410);
      const body = await response.json();
      expect(body.error).toBe("Gone");
      expect(body.message).toContain("deprecated");
      expect(body.alternatives).toContain("/api/workspaces/:name/agents");
    });

    it("returns 410 Gone for POST requests when proxy is disabled", async () => {
      const { POST } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents", {
        method: "POST",
        body: JSON.stringify({ name: "test-agent" }),
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await POST(request, context);

      expect(response.status).toBe(410);
    });

    it("returns 410 Gone for PUT requests when proxy is disabled", async () => {
      const { PUT } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents/my-agent", {
        method: "PUT",
        body: JSON.stringify({ spec: {} }),
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents", "my-agent"] }) };

      const response = await PUT(request, context);

      expect(response.status).toBe(410);
    });

    it("returns 410 Gone for PATCH requests when proxy is disabled", async () => {
      const { PATCH } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents/my-agent", {
        method: "PATCH",
        body: JSON.stringify({ spec: {} }),
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents", "my-agent"] }) };

      const response = await PATCH(request, context);

      expect(response.status).toBe(410);
    });

    it("returns 410 Gone for DELETE requests when proxy is disabled", async () => {
      const { DELETE } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents/my-agent", {
        method: "DELETE",
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents", "my-agent"] }) };

      const response = await DELETE(request, context);

      expect(response.status).toBe(410);
    });

    it("logs warning when proxy request is blocked", async () => {
      const { logWarn } = await import("@/lib/audit");
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logWarn).toHaveBeenCalledWith(
        "Blocked proxy request - proxy is disabled",
        "operator-proxy",
        expect.objectContaining({ method: "GET", path: "api/v1/agents" })
      );
    });

    it("includes migration guide URL in 410 response", async () => {
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await GET(request, context);
      const body = await response.json();

      expect(body.migrationGuide).toBe("https://github.com/AltairaLabs/Omnia/issues/278");
    });

    it("logs proxy usage for audit trail even when disabled", async () => {
      const { logProxyUsage } = await import("@/lib/audit");
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents", {
        headers: {
          "x-forwarded-user": "user@example.com",
          "user-agent": "TestAgent/1.0",
        },
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logProxyUsage).toHaveBeenCalledWith(
        "GET",
        "api/v1/agents",
        "user@example.com",
        "TestAgent/1.0"
      );
    });
  });

  describe("compat mode", () => {
    beforeEach(() => {
      process.env.OMNIA_PROXY_MODE = "compat";
    });

    it("forwards GET requests to operator API", async () => {
      const mockResponse = { items: [{ name: "agent1" }] };
      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve(JSON.stringify(mockResponse)),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      // Need to re-import to pick up env change
      vi.resetModules();
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await GET(request, context);

      expect(response.status).toBe(200);
      expect(response.headers.get("X-Deprecated")).toBe("true");
      const body = await response.json();
      expect(body.items).toHaveLength(1);
    });

    it("forwards POST requests with body", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        status: 201,
        statusText: "Created",
        text: () => Promise.resolve(JSON.stringify({ name: "new-agent" })),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      vi.resetModules();
      const { POST } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents", {
        method: "POST",
        body: JSON.stringify({ name: "new-agent" }),
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await POST(request, context);

      expect(response.status).toBe(201);
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/agents"),
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ name: "new-agent" }),
        })
      );
    });

    it("forwards query parameters", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("[]"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      vi.resetModules();
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents?namespace=default&limit=10");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringMatching(/namespace=default.*limit=10|limit=10.*namespace=default/),
        expect.any(Object)
      );
    });

    it("forwards authorization header", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("[]"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      vi.resetModules();
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents", {
        headers: {
          Authorization: "Bearer test-token",
        },
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(global.fetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: "Bearer test-token",
          }),
        })
      );
    });

    it("logs deprecation warning in compat mode", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("[]"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      vi.resetModules();
      const { logWarn } = await import("@/lib/audit");
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logWarn).toHaveBeenCalledWith(
        "Deprecated operator proxy route called",
        "operator-proxy",
        expect.objectContaining({ method: "GET", path: "api/v1/agents" })
      );
    });

    it("includes deprecation headers in response", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("{}"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      vi.resetModules();
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await GET(request, context);

      expect(response.headers.get("X-Deprecated")).toBe("true");
      expect(response.headers.get("X-Deprecation-Notice")).toContain("deprecated");
    });

    it("returns 502 on connection error", async () => {
      global.fetch = vi.fn().mockRejectedValue(new Error("Connection refused"));

      vi.resetModules();
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      const response = await GET(request, context);

      expect(response.status).toBe(502);
      const body = await response.json();
      expect(body.error).toBe("Failed to connect to operator API");
      expect(body.details).toBe("Connection refused");
    });

    it("logs error on connection failure", async () => {
      global.fetch = vi.fn().mockRejectedValue(new Error("Connection refused"));

      vi.resetModules();
      const { logError } = await import("@/lib/audit");
      const { GET } = await import("./route");

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logError).toHaveBeenCalledWith(
        "Failed to connect to operator API",
        expect.any(Error),
        "operator-proxy",
        expect.objectContaining({ method: "GET" })
      );
    });

    it("uses x-user-email header when x-forwarded-user is not present", async () => {
      vi.resetModules();
      const { logProxyUsage } = await import("@/lib/audit");
      const { GET } = await import("./route");

      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("{}"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents", {
        headers: {
          "x-user-email": "alt-user@example.com",
        },
      });
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logProxyUsage).toHaveBeenCalledWith(
        "GET",
        "api/v1/agents",
        "alt-user@example.com",
        undefined
      );
    });

    it("uses 'unknown' when no user headers present", async () => {
      vi.resetModules();
      const { logProxyUsage } = await import("@/lib/audit");
      const { GET } = await import("./route");

      global.fetch = vi.fn().mockResolvedValue({
        status: 200,
        statusText: "OK",
        text: () => Promise.resolve("{}"),
        headers: new Headers({ "Content-Type": "application/json" }),
      });

      const request = new NextRequest("http://localhost/api/operator/api/v1/agents");
      const context = { params: Promise.resolve({ path: ["api", "v1", "agents"] }) };

      await GET(request, context);

      expect(logProxyUsage).toHaveBeenCalledWith(
        "GET",
        "api/v1/agents",
        "unknown",
        undefined
      );
    });
  });
});
