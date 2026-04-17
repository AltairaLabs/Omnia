/**
 * @vitest-environment jsdom
 *
 * Tests for AppShell — chooses between authenticated chrome (sidebar + banners)
 * and the minimal auth-page shell based on the current pathname.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

const mockUsePathname = vi.fn<() => string>();
vi.mock("next/navigation", () => ({
  usePathname: () => mockUsePathname(),
}));

vi.mock("@/components/layout", () => ({
  Sidebar: () => <div data-testid="sidebar" />,
  ReadOnlyBanner: () => <div data-testid="read-only-banner" />,
  DemoModeBanner: () => <div data-testid="demo-mode-banner" />,
  LicenseExpiryBanner: () => <div data-testid="license-expiry-banner" />,
  DevModeLicenseBanner: () => <div data-testid="dev-mode-license-banner" />,
  WorkspaceContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="workspace-content">{children}</div>
  ),
}));

vi.mock("@/components/error-boundary", () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { AppShell } from "./app-shell";

describe("AppShell", () => {
  beforeEach(() => {
    mockUsePathname.mockReset();
  });

  describe("auth pages (no chrome)", () => {
    it.each([
      "/login",
      "/signup",
      "/forgot-password",
      "/reset-password",
      "/verify-email",
      "/login/",
      "/reset-password/abc123",
    ])("renders plain shell for %s", (pathname) => {
      mockUsePathname.mockReturnValue(pathname);
      render(
        <AppShell>
          <div data-testid="page">page content</div>
        </AppShell>
      );
      expect(screen.getByTestId("page")).toBeDefined();
      expect(screen.queryByTestId("sidebar")).toBeNull();
      expect(screen.queryByTestId("demo-mode-banner")).toBeNull();
      expect(screen.queryByTestId("read-only-banner")).toBeNull();
      expect(screen.queryByTestId("license-expiry-banner")).toBeNull();
      expect(screen.queryByTestId("dev-mode-license-banner")).toBeNull();
      expect(screen.queryByTestId("workspace-content")).toBeNull();
    });
  });

  describe("authenticated pages (full chrome)", () => {
    it.each(["/", "/agents", "/sessions", "/loginner-not-really-login"])(
      "renders sidebar + banners for %s",
      (pathname) => {
        mockUsePathname.mockReturnValue(pathname);
        render(
          <AppShell>
            <div data-testid="page">page content</div>
          </AppShell>
        );
        expect(screen.getByTestId("sidebar")).toBeDefined();
        expect(screen.getByTestId("demo-mode-banner")).toBeDefined();
        expect(screen.getByTestId("read-only-banner")).toBeDefined();
        expect(screen.getByTestId("license-expiry-banner")).toBeDefined();
        expect(screen.getByTestId("dev-mode-license-banner")).toBeDefined();
        expect(screen.getByTestId("workspace-content")).toBeDefined();
        expect(screen.getByTestId("page")).toBeDefined();
      }
    );
  });
});
