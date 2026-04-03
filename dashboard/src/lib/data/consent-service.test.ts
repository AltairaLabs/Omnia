import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ConsentService } from "./consent-service";

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("ConsentService", () => {
  let service: ConsentService;

  beforeEach(() => {
    service = new ConsentService();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("getConsent", () => {
    it("fetches consent for a user and returns parsed response", async () => {
      const expected = {
        grants: ["analytics"],
        defaults: ["essential"],
        denied: ["marketing"],
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(expected),
      });

      const result = await service.getConsent("my-workspace", "user-123");

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("/api/workspaces/my-workspace/privacy/consent");
      expect(url).toContain("userId=user-123");
      expect(result).toEqual(expected);
    });

    it("encodes workspace name in the URL", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ grants: [], defaults: [], denied: [] }),
      });

      await service.getConsent("my workspace", "u1");

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("my%20workspace");
    });

    it("returns empty defaults on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.getConsent("ws", "u1");
      expect(result).toEqual({ grants: [], defaults: [], denied: [] });
    });

    it("returns empty defaults on 401", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 401, statusText: "Unauthorized" });

      const result = await service.getConsent("ws", "u1");
      expect(result).toEqual({ grants: [], defaults: [], denied: [] });
    });

    it("returns empty defaults on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      const result = await service.getConsent("ws", "u1");
      expect(result).toEqual({ grants: [], defaults: [], denied: [] });
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Internal Server Error" });

      await expect(service.getConsent("ws", "u1")).rejects.toThrow("Failed to fetch consent");
    });
  });

  describe("updateConsent", () => {
    it("sends PUT with JSON body and returns updated response", async () => {
      const expected = {
        grants: ["analytics", "personalization"],
        defaults: ["essential"],
        denied: ["marketing"],
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(expected),
      });

      const result = await service.updateConsent("my-workspace", "user-123", {
        grants: ["analytics", "personalization"],
      });

      const [url, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
      expect(url).toContain("/api/workspaces/my-workspace/privacy/consent");
      expect(url).toContain("userId=user-123");
      expect(opts.method).toBe("PUT");
      expect(opts.headers).toMatchObject({ "Content-Type": "application/json" });
      expect(JSON.parse(opts.body as string)).toEqual({ grants: ["analytics", "personalization"] });
      expect(result).toEqual(expected);
    });

    it("sends PUT with revocations in body", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ grants: [], defaults: ["essential"], denied: ["analytics"] }),
      });

      await service.updateConsent("ws", "u1", { revocations: ["analytics"] });

      const [, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
      expect(JSON.parse(opts.body as string)).toEqual({ revocations: ["analytics"] });
    });

    it("throws on 400 error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 400, statusText: "Bad Request" });

      await expect(
        service.updateConsent("ws", "u1", { grants: ["unknown-category"] })
      ).rejects.toThrow("Failed to update consent");
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Internal Server Error" });

      await expect(
        service.updateConsent("ws", "u1", { grants: ["analytics"] })
      ).rejects.toThrow("Failed to update consent");
    });
  });
});
