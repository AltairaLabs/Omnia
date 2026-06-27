import { describe, it, expect } from "vitest";
import { facadeAuthHint } from "./facade-auth-hint";
import type { ExternalAuth } from "@/types/agent-runtime";

describe("facadeAuthHint", () => {
  it("returns management-plane only when externalAuth is undefined", () => {
    expect(facadeAuthHint(undefined)).toEqual({ label: "Management-plane only" });
  });

  it("returns management-plane only when no auth method is set", () => {
    const auth: ExternalAuth = {};
    expect(facadeAuthHint(auth)).toEqual({ label: "Management-plane only" });
  });

  it("returns Bearer token + secret name for sharedToken", () => {
    const auth: ExternalAuth = {
      sharedToken: { secretRef: { name: "my-agent-token" } },
    };
    expect(facadeAuthHint(auth)).toEqual({
      label: "Bearer token",
      detail: "Secret `my-agent-token`",
    });
  });

  it("returns Bearer token without detail when secretRef.name is undefined", () => {
    const auth: ExternalAuth = {
      sharedToken: { secretRef: {} },
    };
    expect(facadeAuthHint(auth)).toEqual({
      label: "Bearer token",
      detail: undefined,
    });
  });

  it("returns API key (Bearer) for apiKeys", () => {
    const auth: ExternalAuth = {
      apiKeys: { defaultRole: "viewer" },
    };
    expect(facadeAuthHint(auth)).toEqual({ label: "API key (Bearer)" });
  });

  it("returns OIDC + issuer for oidc", () => {
    const auth: ExternalAuth = {
      oidc: { issuer: "https://auth.example.com", audience: "my-agent" },
    };
    expect(facadeAuthHint(auth)).toEqual({
      label: "OIDC",
      detail: "https://auth.example.com",
    });
  });

  it("returns edge-trusted headers for edgeTrust", () => {
    const auth: ExternalAuth = {
      edgeTrust: { headerMapping: { subject: "x-user-id" } },
    };
    expect(facadeAuthHint(auth)).toEqual({ label: "Edge-trusted headers" });
  });

  it("sharedToken takes precedence over apiKeys", () => {
    const auth: ExternalAuth = {
      sharedToken: { secretRef: { name: "token-secret" } },
      apiKeys: { defaultRole: "admin" },
    };
    const result = facadeAuthHint(auth);
    expect(result.label).toBe("Bearer token");
  });

  it("apiKeys takes precedence over oidc", () => {
    const auth: ExternalAuth = {
      apiKeys: {},
      oidc: { issuer: "https://auth.example.com", audience: "x" },
    };
    const result = facadeAuthHint(auth);
    expect(result.label).toBe("API key (Bearer)");
  });
});
