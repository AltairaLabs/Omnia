import { describe, it, expect, afterEach } from "vitest";
import { NextResponse } from "next/server";
import { applySecurityHeaders } from "./security-headers";

describe("applySecurityHeaders", () => {
  afterEach(() => {
    delete process.env.OMNIA_CSP_POLICY;
  });

  it("self-hosts scripts and allows Monaco blob workers (no third-party CDN)", () => {
    const res = applySecurityHeaders(NextResponse.next());
    const csp = res.headers.get("Content-Security-Policy") ?? "";

    expect(csp).toContain("script-src 'self'");
    // Monaco is self-hosted from /monaco/vs and spawns workers from blob URLs.
    expect(csp).toContain("worker-src 'self' blob:");
    // The editor must not depend on a CDN the CSP would otherwise have to allow.
    expect(csp).not.toContain("jsdelivr");
  });

  it("allows brand webfonts from Google Fonts (white-label branding)", () => {
    const res = applySecurityHeaders(NextResponse.next());
    const csp = res.headers.get("Content-Security-Policy") ?? "";
    // Brand fonts.url stylesheets + their font files must be loadable.
    expect(csp).toContain("https://fonts.googleapis.com");
    expect(csp).toContain("https://fonts.gstatic.com");
  });

  it("honours the OMNIA_CSP_POLICY override", () => {
    process.env.OMNIA_CSP_POLICY = "default-src 'none'";
    const res = applySecurityHeaders(NextResponse.next());
    expect(res.headers.get("Content-Security-Policy")).toBe("default-src 'none'");
  });

  it("applies the security-header baseline", () => {
    const res = applySecurityHeaders(NextResponse.next());
    expect(res.headers.get("X-Frame-Options")).toBe("DENY");
    expect(res.headers.get("X-Content-Type-Options")).toBe("nosniff");
    expect(res.headers.get("Strict-Transport-Security")).toContain("max-age=");
  });

  it("allows microphone for self (duplex voice console)", () => {
    const res = applySecurityHeaders(NextResponse.next());
    expect(res.headers.get("Permissions-Policy")).toContain("microphone=(self)");
  });

  it("keeps camera disabled", () => {
    const res = applySecurityHeaders(NextResponse.next());
    expect(res.headers.get("Permissions-Policy")).toContain("camera=()");
  });
});
