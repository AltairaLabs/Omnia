/**
 * Minimal stub for Node.js perf_hooks module.
 * Used by Turbopack to replace perf_hooks in browser builds.
 */

export const performance = globalThis.performance || {
  timeOrigin: Date.now(),
  now: () => Date.now(),
};

export default { performance };
