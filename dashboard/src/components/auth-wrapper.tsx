import { getUser } from "@/lib/auth";
import { AuthProvider } from "@/hooks/auth";
import { SessionWatcher } from "@/components/auth/session-watcher";

interface AuthWrapperProps {
  children: React.ReactNode;
}

/**
 * Server component that fetches the current user and provides it to client components.
 * Also mounts the SessionWatcher so it can detect session expiry and redirect to login.
 */
export async function AuthWrapper({ children }: Readonly<AuthWrapperProps>) {
  const user = await getUser();

  return (
    <AuthProvider user={user}>
      <SessionWatcher />
      {children}
    </AuthProvider>
  );
}
