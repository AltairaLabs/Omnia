import { getUser } from "@/lib/auth";
import { AuthProvider } from "@/hooks/use-auth";

interface AuthWrapperProps {
  children: React.ReactNode;
}

/**
 * Server component that fetches the current user and provides it to client components.
 */
export async function AuthWrapper({ children }: Readonly<AuthWrapperProps>) {
  const user = await getUser();

  return <AuthProvider user={user}>{children}</AuthProvider>;
}
