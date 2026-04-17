/**
 * Runtime auth-mode guardrail.
 *
 * Pure function — given an env object, decides whether the dashboard
 * should refuse to boot. Called from server.js before Next.js starts.
 *
 * The Helm chart also enforces this at render time
 * (charts/omnia/templates/_helpers.tpl — omnia.validateAuth), but anyone
 * bypassing Helm (raw kubectl apply, kustomize, operator-managed
 * rollouts, forgetting to re-render on a values change) can still land
 * OMNIA_AUTH_MODE=anonymous on a production-shaped pod. This catches
 * that case and makes the pod CrashLoopBackOff instead of silently
 * serving the dashboard unauthenticated.
 *
 * Rules:
 *   - mode != "anonymous"              → allowed.
 *   - mode == "anonymous"
 *        + NODE_ENV != "production"    → allowed (dev/test convenience).
 *   - mode == "anonymous"
 *        + NODE_ENV == "production"
 *        + OMNIA_ALLOW_ANONYMOUS=true  → allowed (explicit opt-in).
 *   - otherwise                        → refuse to boot.
 *
 * Exported as { checkAnonymousAuthGuard } and tested in
 * auth-boot-guard.test.js.
 */

const SEPARATOR = "=".repeat(78);

function checkAnonymousAuthGuard(env = process.env) {
  const mode = (env.OMNIA_AUTH_MODE || "anonymous").trim().toLowerCase();
  if (mode !== "anonymous") return { ok: true };

  const nodeEnv = (env.NODE_ENV || "").trim().toLowerCase();
  const allow = (env.OMNIA_ALLOW_ANONYMOUS || "").trim().toLowerCase() === "true";

  if (nodeEnv === "production" && !allow) {
    return {
      ok: false,
      message: [
        SEPARATOR,
        "REFUSING TO START",
        "",
        "OMNIA_AUTH_MODE=anonymous with NODE_ENV=production disables",
        "authentication entirely. If this is truly intentional (for example a",
        "sandboxed public demo), set OMNIA_ALLOW_ANONYMOUS=true to acknowledge.",
        "",
        "Otherwise, set OMNIA_AUTH_MODE to one of: oauth, builtin, proxy.",
        SEPARATOR,
      ].join("\n"),
    };
  }
  return { ok: true };
}

module.exports = { checkAnonymousAuthGuard };
