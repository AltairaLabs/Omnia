/**
 * Pathname classification for the app shell. "Chromeless" routes render
 * without sidebar/banners and don't require an authenticated session — so
 * providers above the shell should also skip workspace/auth-dependent
 * fetches when the current pathname matches.
 */

export const CHROMELESS_PATH_PREFIXES: readonly string[] = ["/login"];

export function isChromelessPath(pathname: string): boolean {
  for (const prefix of CHROMELESS_PATH_PREFIXES) {
    if (pathname === prefix || pathname.startsWith(`${prefix}/`)) return true;
  }
  return false;
}
