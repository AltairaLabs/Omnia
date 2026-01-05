/**
 * Hook for read-only mode configuration.
 *
 * When NEXT_PUBLIC_READ_ONLY_MODE=true, the dashboard disables all
 * mutating operations (scaling, deploying, etc.). This is useful for
 * GitOps environments where changes must go through Git.
 *
 * The message shown to users can be customized via NEXT_PUBLIC_READ_ONLY_MESSAGE.
 */

const DEFAULT_MESSAGE = "Dashboard is in read-only mode. Changes must be made through GitOps.";

export interface ReadOnlyConfig {
  /** Whether the dashboard is in read-only mode */
  isReadOnly: boolean;
  /** Message to display when user tries to make changes */
  message: string;
}

/**
 * Returns the read-only configuration for the dashboard.
 * Uses environment variables for configuration.
 */
export function useReadOnly(): ReadOnlyConfig {
  const isReadOnly = process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true";
  const message = process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || DEFAULT_MESSAGE;

  return {
    isReadOnly,
    message,
  };
}
