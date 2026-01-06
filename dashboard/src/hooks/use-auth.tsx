"use client";

/**
 * Auth context and hooks for client components.
 *
 * Usage:
 *   import { useAuth } from "@/hooks/use-auth";
 *
 *   function MyComponent() {
 *     const { user, isAuthenticated, canWrite, logout } = useAuth();
 *     ...
 *   }
 */

import {
  createContext,
  useContext,
  useCallback,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import type { User, UserRole } from "@/lib/auth/types";
import { logout as serverLogout } from "@/lib/auth/actions";

interface AuthContextValue {
  /** Current user */
  user: User;
  /** Whether user is authenticated (not anonymous) */
  isAuthenticated: boolean;
  /** User's role */
  role: UserRole;
  /** Check if user has at least the required role */
  hasRole: (role: UserRole) => boolean;
  /** Check if user can perform write operations */
  canWrite: boolean;
  /** Check if user can perform admin operations */
  canAdmin: boolean;
  /** Logout and redirect */
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
  /** User fetched on the server */
  user: User;
}

/**
 * Auth provider - wrap your app with this to provide auth context.
 * The user should be fetched on the server and passed as a prop.
 */
export function AuthProvider({ children, user }: AuthProviderProps) {
  const router = useRouter();

  const hasRole = useCallback(
    (requiredRole: UserRole) => {
      const roleHierarchy: Record<UserRole, number> = {
        admin: 3,
        editor: 2,
        viewer: 1,
      };
      return roleHierarchy[user.role] >= roleHierarchy[requiredRole];
    },
    [user.role]
  );

  const logout = useCallback(async () => {
    await serverLogout();
    router.refresh();
  }, [router]);

  const value: AuthContextValue = {
    user,
    isAuthenticated: user.provider !== "anonymous",
    role: user.role,
    hasRole,
    canWrite: hasRole("editor"),
    canAdmin: hasRole("admin"),
    logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/**
 * Hook to access auth context.
 */
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
