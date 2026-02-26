/**
 * Shared React Query configuration constants.
 *
 * Centralizes magic numbers used across query hooks to ensure
 * consistent caching behavior and easy tuning.
 */

/** Default stale time for queries (30 seconds). */
export const DEFAULT_STALE_TIME = 30_000;

/** Prometheus fetch timeout (30 seconds). */
export const PROMETHEUS_FETCH_TIMEOUT_MS = 30_000;
