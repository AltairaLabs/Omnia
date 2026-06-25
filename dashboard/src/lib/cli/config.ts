/** CLI browser-login configuration. */

const DEFAULT_CLI_TOKEN_TTL = 3600;

/** One-time exchange code lifetime (seconds). */
export const CLI_CODE_TTL_SECONDS = 60;

/** TTL for the minted CLI deploy token. Env override, default 1h. */
export function cliTokenTtlSeconds(): number {
  const raw = process.env.OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS;
  const n = raw ? Number.parseInt(raw, 10) : Number.NaN;
  return Number.isFinite(n) && n > 0 ? n : DEFAULT_CLI_TOKEN_TTL;
}
