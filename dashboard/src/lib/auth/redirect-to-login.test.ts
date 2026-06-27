import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Reset module state between tests so the redirect guard is fresh each time.
// vi.resetModules() inside beforeEach() + dynamic imports give each test its
// own module instance with its own `redirecting` variable.
beforeEach(() => {
  vi.resetModules();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("redirectToLogin", () => {
  it("navigates to /login with returnTo from current location when no arg given", async () => {
    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      writable: true,
      value: { pathname: "/agents", search: "?ns=default", assign },
    });

    const { redirectToLogin } = await import("./redirect-to-login");
    redirectToLogin();
    expect(assign).toHaveBeenCalledWith(
      "/login?returnTo=%2Fagents%3Fns%3Ddefault",
    );
  });

  it("uses the provided returnTo path when supplied", async () => {
    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      writable: true,
      value: { pathname: "/", search: "", assign },
    });

    const { redirectToLogin } = await import("./redirect-to-login");
    redirectToLogin("/some/path");
    expect(assign).toHaveBeenCalledWith("/login?returnTo=%2Fsome%2Fpath");
  });

  it("fires only once when called multiple times (in-flight guard)", async () => {
    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      writable: true,
      value: { pathname: "/dashboard", search: "", assign },
    });

    const { redirectToLogin } = await import("./redirect-to-login");
    redirectToLogin();
    redirectToLogin();
    redirectToLogin();
    expect(assign).toHaveBeenCalledTimes(1);
  });

  it("does nothing in an SSR context where window is undefined", async () => {
    const originalWindow = globalThis.window;
    // @ts-expect-error – simulate SSR
    delete globalThis.window;

    const { redirectToLogin } = await import("./redirect-to-login");
    // Should not throw
    expect(() => redirectToLogin()).not.toThrow();

    globalThis.window = originalWindow;
  });

  it("_resetRedirectGuard allows a second redirect after reset", async () => {
    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      writable: true,
      value: { pathname: "/x", search: "", assign },
    });

    const { redirectToLogin, _resetRedirectGuard } = await import(
      "./redirect-to-login"
    );
    redirectToLogin();
    expect(assign).toHaveBeenCalledTimes(1);

    _resetRedirectGuard();
    redirectToLogin();
    expect(assign).toHaveBeenCalledTimes(2);
  });
});
