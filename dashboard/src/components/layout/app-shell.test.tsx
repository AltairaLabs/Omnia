import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  usePathname: vi.fn(),
}));

vi.mock("./sidebar", () => ({
  Sidebar: () => <div data-testid="sidebar">sidebar</div>,
}));
vi.mock("./read-only-banner", () => ({ ReadOnlyBanner: () => null }));
vi.mock("./demo-mode-banner", () => ({ DemoModeBanner: () => null }));
vi.mock("./license-expiry-banner", () => ({ LicenseExpiryBanner: () => null }));
vi.mock("./dev-mode-license-banner", () => ({ DevModeLicenseBanner: () => null }));
vi.mock("./workspace-content", () => ({
  WorkspaceContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="workspace-content">{children}</div>
  ),
}));
vi.mock("@/components/error-boundary", () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { usePathname } from "next/navigation";
import { AppShell } from "./app-shell";

describe("AppShell", () => {
  it("renders full chrome (sidebar + workspace) on regular pages", () => {
    vi.mocked(usePathname).mockReturnValue("/sessions");
    render(
      <AppShell>
        <div data-testid="page">page content</div>
      </AppShell>,
    );
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
    expect(screen.getByTestId("workspace-content")).toBeInTheDocument();
    expect(screen.getByTestId("page")).toBeInTheDocument();
  });

  it("hides chrome on /login", () => {
    vi.mocked(usePathname).mockReturnValue("/login");
    render(
      <AppShell>
        <div data-testid="page">login form</div>
      </AppShell>,
    );
    expect(screen.queryByTestId("sidebar")).not.toBeInTheDocument();
    expect(screen.queryByTestId("workspace-content")).not.toBeInTheDocument();
    expect(screen.getByTestId("page")).toBeInTheDocument();
  });

  it("hides chrome on login subpaths (e.g. /login/error)", () => {
    vi.mocked(usePathname).mockReturnValue("/login/error");
    render(
      <AppShell>
        <div data-testid="page">error</div>
      </AppShell>,
    );
    expect(screen.queryByTestId("sidebar")).not.toBeInTheDocument();
    expect(screen.getByTestId("page")).toBeInTheDocument();
  });

  it("does not match /login-like prefixes (e.g. /loginx)", () => {
    vi.mocked(usePathname).mockReturnValue("/loginx");
    render(
      <AppShell>
        <div data-testid="page">other</div>
      </AppShell>,
    );
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
  });

  it("renders chrome on the root path", () => {
    vi.mocked(usePathname).mockReturnValue("/");
    render(
      <AppShell>
        <div data-testid="page">home</div>
      </AppShell>,
    );
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
  });
});
