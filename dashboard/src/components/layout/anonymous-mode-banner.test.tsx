/**
 * @vitest-environment jsdom
 *
 * Tests for AnonymousModeBanner — renders only when the current user's
 * provider is "anonymous".
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { User } from "@/lib/auth/types";

const useAuthMock = vi.fn<() => { user: User }>();
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => useAuthMock(),
}));

const useDemoModeMock = vi.fn<() => { isDemoMode: boolean; loading: boolean }>();
vi.mock("@/hooks/use-runtime-config", () => ({
  useDemoMode: () => useDemoModeMock(),
}));

import { AnonymousModeBanner } from "./anonymous-mode-banner";

const ANON: User = {
  id: "anonymous",
  username: "anonymous",
  groups: [],
  role: "viewer",
  provider: "anonymous",
};

const OAUTH: User = {
  id: "u1",
  username: "alice",
  groups: [],
  role: "admin",
  provider: "oauth",
};

describe("AnonymousModeBanner", () => {
  beforeEach(() => {
    useAuthMock.mockReset();
    useDemoModeMock.mockReset();
    useDemoModeMock.mockReturnValue({ isDemoMode: false, loading: false });
  });

  it("renders a warning when the current user is anonymous", () => {
    useAuthMock.mockReturnValue({ user: ANON });
    render(<AnonymousModeBanner />);
    const alert = screen.getByRole("alert");
    expect(alert).toBeDefined();
    expect(alert.textContent).toMatch(/Anonymous access/);
    expect(alert.textContent).toMatch(/authentication is disabled/i);
  });

  it.each([OAUTH, { ...OAUTH, provider: "builtin" as const }, { ...OAUTH, provider: "proxy" as const }])(
    "renders nothing for non-anonymous provider %#",
    (user) => {
      useAuthMock.mockReturnValue({ user });
      const { container } = render(<AnonymousModeBanner />);
      expect(container.firstChild).toBeNull();
      expect(screen.queryByRole("alert")).toBeNull();
    },
  );

  it("suppresses the banner in demo mode even when the user is anonymous", () => {
    useAuthMock.mockReturnValue({ user: ANON });
    useDemoModeMock.mockReturnValue({ isDemoMode: true, loading: false });
    const { container } = render(<AnonymousModeBanner />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
