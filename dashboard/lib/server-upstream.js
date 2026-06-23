/**
 * resolveUpstreamTarget picks the upstream WebSocket host for a reconnect.
 *
 * When the client sends `resume=<sessionId>` and a Redis route hint exists
 * (published by the facade on park: `rt:route:<sessionId>` → `<podIP>:<port>`),
 * the proxy dials the parked pod directly so in-flight state is restored
 * without a round-trip through the Service load balancer.
 *
 * Fail-open: any miss, Redis error, or timeout returns the Service target so
 * the connection always succeeds (the session-resume protocol then handles
 * state recovery from the session store).
 */

/**
 * @param {{ resume: string|null, service: { host: string, port: number } }} opts
 * @param {{ get(key: string): Promise<string|null> }|null} redis
 * @param {{ timeoutMs: number }} config
 * @returns {Promise<{ host: string, port: number }>}
 */
async function resolveUpstreamTarget({ resume, service }, redis, { timeoutMs }) {
  if (!resume || !redis) {
    return service;
  }
  try {
    const route = await withTimeout(redis.get(`rt:route:${resume}`), timeoutMs);
    if (!route) {
      return service;
    }
    const colonIdx = route.lastIndexOf(":");
    if (colonIdx <= 0) {
      return service;
    }
    const host = route.slice(0, colonIdx);
    const port = Number.parseInt(route.slice(colonIdx + 1), 10);
    if (!host || Number.isNaN(port)) {
      return service;
    }
    return { host, port };
  } catch {
    return service; // fail-open on redis error or timeout
  }
}

function withTimeout(promise, ms) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error("redis timeout")), ms);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

module.exports = { resolveUpstreamTarget };
