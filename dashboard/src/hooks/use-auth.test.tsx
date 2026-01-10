import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthProvider, useAuth } from "./use-auth";
import type { User } from "@/lib/auth/types";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    refresh: vi.fn(),
  }),
}));

// Mock server actions
const mockServerLogout = vi.fn().mockResolvedValue(undefined);
vi.mock("@/lib/auth/actions", () => ({
  logout: () => mockServerLogout(),
}));

// Test component that uses the auth hook
function TestConsumer() {
  const auth = useAuth();
  return (
    <div>
      <div data-testid="user-name">{auth.user.displayName || auth.user.username}</div>
      <div data-testid="user-role">{auth.role}</div>
      <div data-testid="is-authenticated">{auth.isAuthenticated.toString()}</div>
      <div data-testid="can-write">{auth.canWrite.toString()}</div>
      <div data-testid="can-admin">{auth.canAdmin.toString()}</div>
      <div data-testid="has-role-viewer">{auth.hasRole("viewer").toString()}</div>
      <div data-testid="has-role-editor">{auth.hasRole("editor").toString()}</div>
      <div data-testid="has-role-admin">{auth.hasRole("admin").toString()}</div>
      <button data-testid="logout-btn" onClick={auth.logout}>
        Logout
      </button>
    </div>
  );
}

describe("AuthProvider and useAuth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("with viewer user", () => {
    const viewerUser: User = {
      id: "user-1",
      username: "testviewer",
      displayName: "Test Viewer",
      email: "viewer@example.com",
      groups: [],
      role: "viewer",
      provider: "oauth",
    };

    it("should provide user data", () => {
      render(
        <AuthProvider user={viewerUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("user-name")).toHaveTextContent("Test Viewer");
      expect(screen.getByTestId("user-role")).toHaveTextContent("viewer");
    });

    it("should mark user as authenticated", () => {
      render(
        <AuthProvider user={viewerUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("is-authenticated")).toHaveTextContent("true");
    });

    it("should not have write permissions", () => {
      render(
        <AuthProvider user={viewerUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("can-write")).toHaveTextContent("false");
    });

    it("should not have admin permissions", () => {
      render(
        <AuthProvider user={viewerUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("can-admin")).toHaveTextContent("false");
    });

    it("should have viewer role but not higher", () => {
      render(
        <AuthProvider user={viewerUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("has-role-viewer")).toHaveTextContent("true");
      expect(screen.getByTestId("has-role-editor")).toHaveTextContent("false");
      expect(screen.getByTestId("has-role-admin")).toHaveTextContent("false");
    });
  });

  describe("with editor user", () => {
    const editorUser: User = {
      id: "user-2",
      username: "testeditor",
      displayName: "Test Editor",
      email: "editor@example.com",
      groups: [],
      role: "editor",
      provider: "oauth",
    };

    it("should have write permissions", () => {
      render(
        <AuthProvider user={editorUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("can-write")).toHaveTextContent("true");
    });

    it("should not have admin permissions", () => {
      render(
        <AuthProvider user={editorUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("can-admin")).toHaveTextContent("false");
    });

    it("should have viewer and editor roles", () => {
      render(
        <AuthProvider user={editorUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("has-role-viewer")).toHaveTextContent("true");
      expect(screen.getByTestId("has-role-editor")).toHaveTextContent("true");
      expect(screen.getByTestId("has-role-admin")).toHaveTextContent("false");
    });
  });

  describe("with admin user", () => {
    const adminUser: User = {
      id: "user-3",
      username: "testadmin",
      displayName: "Test Admin",
      email: "admin@example.com",
      groups: [],
      role: "admin",
      provider: "oauth",
    };

    it("should have all permissions", () => {
      render(
        <AuthProvider user={adminUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("can-write")).toHaveTextContent("true");
      expect(screen.getByTestId("can-admin")).toHaveTextContent("true");
    });

    it("should have all roles", () => {
      render(
        <AuthProvider user={adminUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("has-role-viewer")).toHaveTextContent("true");
      expect(screen.getByTestId("has-role-editor")).toHaveTextContent("true");
      expect(screen.getByTestId("has-role-admin")).toHaveTextContent("true");
    });
  });

  describe("with anonymous user", () => {
    const anonymousUser: User = {
      id: "anon",
      username: "anonymous",
      groups: [],
      role: "viewer",
      provider: "anonymous",
    };

    it("should not be authenticated", () => {
      render(
        <AuthProvider user={anonymousUser}>
          <TestConsumer />
        </AuthProvider>
      );

      expect(screen.getByTestId("is-authenticated")).toHaveTextContent("false");
    });
  });

  describe("logout", () => {
    const user: User = {
      id: "user-1",
      username: "testuser",
      displayName: "Test User",
      email: "test@example.com",
      groups: [],
      role: "viewer",
      provider: "oauth",
    };

    it("should call server logout when logout is triggered", async () => {
      render(
        <AuthProvider user={user}>
          <TestConsumer />
        </AuthProvider>
      );

      const logoutBtn = screen.getByTestId("logout-btn");
      await userEvent.click(logoutBtn);

      await waitFor(() => {
        expect(mockServerLogout).toHaveBeenCalled();
      });
    });
  });

  describe("useAuth outside provider", () => {
    it("should throw error when used outside AuthProvider", () => {
      // Suppress console.error for this test
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      expect(() => {
        render(<TestConsumer />);
      }).toThrow("useAuth must be used within an AuthProvider");

      consoleSpy.mockRestore();
    });
  });
});
