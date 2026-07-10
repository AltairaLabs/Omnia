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

  it("returns Client key (Bearer) for clientKeys", () => {
    const auth: ExternalAuth = {
      clientKeys: { defaultRole: "viewer" },
    };
    expect(facadeAuthHint(auth)).toEqual({ label: "Client key (Bearer)" });
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

  it("clientKeys takes precedence over oidc", () => {
    const auth: ExternalAuth = {
      clientKeys: {},
      oidc: { issuer: "https://auth.example.com", audience: "x" },
    };
    const result = facadeAuthHint(auth);
    expect(result.label).toBe("Client key (Bearer)");
  });
});
