import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { SecretsService, getSecretsService } from "./secrets-service";

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock secret data
const mockSecrets = [
  {
    namespace: "default",
    name: "anthropic-credentials",
    keys: ["ANTHROPIC_API_KEY"],
    annotations: { "omnia.altairalabs.ai/provider": "claude" },
    referencedBy: [{ namespace: "default", name: "claude-prod", type: "claude" }],
    createdAt: "2024-01-15T10:00:00Z",
    modifiedAt: "2024-01-15T10:00:00Z",
  },
];

describe("SecretsService", () => {
  let service: SecretsService;

  beforeEach(() => {
    service = new SecretsService();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe("listSecrets", () => {
    it("should list all secrets", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ secrets: mockSecrets }),
      });

      const result = await service.listSecrets();

      expect(mockFetch).toHaveBeenCalledWith("/api/secrets");
      expect(result).toEqual(mockSecrets);
    });

    it("should list secrets filtered by namespace", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ secrets: mockSecrets }),
      });

      const result = await service.listSecrets("default");

      expect(mockFetch).toHaveBeenCalledWith("/api/secrets?namespace=default");
      expect(result).toEqual(mockSecrets);
    });

    it("should handle API errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: "Internal server error" }),
      });

      await expect(service.listSecrets()).rejects.toThrow("Internal server error");
    });

    it("should handle network errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await expect(service.listSecrets()).rejects.toThrow("Network error");
    });
  });

  describe("getSecret", () => {
    it("should get a single secret", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ secret: mockSecrets[0] }),
      });

      const result = await service.getSecret("default", "anthropic-credentials");

      expect(mockFetch).toHaveBeenCalledWith("/api/secrets/default/anthropic-credentials");
      expect(result).toEqual(mockSecrets[0]);
    });

    it("should return null for 404", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      });

      const result = await service.getSecret("default", "non-existent");

      expect(result).toBeNull();
    });

    it("should handle API errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: "Internal server error" }),
      });

      await expect(service.getSecret("default", "test")).rejects.toThrow(
        "Internal server error"
      );
    });

    it("should encode namespace and name in URL", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ secret: mockSecrets[0] }),
      });

      await service.getSecret("my-namespace", "my-secret");

      expect(mockFetch).toHaveBeenCalledWith("/api/secrets/my-namespace/my-secret");
    });
  });

  describe("createOrUpdateSecret", () => {
    it("should create a secret", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ secret: mockSecrets[0] }),
      });

      const request = {
        namespace: "default",
        name: "anthropic-credentials",
        data: { ANTHROPIC_API_KEY: "test-key" },
        providerType: "claude",
      };

      const result = await service.createOrUpdateSecret(request);

      expect(mockFetch).toHaveBeenCalledWith("/api/secrets", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
      expect(result).toEqual(mockSecrets[0]);
    });

    it("should handle validation errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ error: "Invalid secret name" }),
      });

      const request = {
        namespace: "default",
        name: "INVALID_NAME",
        data: { KEY: "value" },
      };

      await expect(service.createOrUpdateSecret(request)).rejects.toThrow(
        "Invalid secret name"
      );
    });

    it("should handle conflict errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: () =>
          Promise.resolve({ error: "Secret exists but is not a managed credential" }),
      });

      const request = {
        namespace: "default",
        name: "existing-secret",
        data: { KEY: "value" },
      };

      await expect(service.createOrUpdateSecret(request)).rejects.toThrow(
        "Secret exists but is not a managed credential"
      );
    });
  });

  describe("deleteSecret", () => {
    it("should delete a secret", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const result = await service.deleteSecret("default", "anthropic-credentials");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/secrets/default/anthropic-credentials",
        { method: "DELETE" }
      );
      expect(result).toBe(true);
    });

    it("should return false for 404", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      });

      const result = await service.deleteSecret("default", "non-existent");

      expect(result).toBe(false);
    });

    it("should handle permission errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ error: "Insufficient permissions" }),
      });

      await expect(
        service.deleteSecret("default", "anthropic-credentials")
      ).rejects.toThrow("Insufficient permissions");
    });
  });
});

describe("getSecretsService", () => {
  it("should return a singleton instance", () => {
    const service1 = getSecretsService();
    const service2 = getSecretsService();

    expect(service1).toBe(service2);
  });

  it("should return a SecretsService instance", () => {
    const service = getSecretsService();

    expect(service).toBeInstanceOf(SecretsService);
  });
});
