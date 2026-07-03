"use client";

import { useState } from "react";

/**
 * Tri-state result of the ServiceUnreadyBanner's own `/services` check:
 * `undefined` = pending (banner mounted, fetch in flight), `true` = a
 * culprit service was identified, `false` = services are healthy (the
 * caller's generic error, if any, is the accurate one).
 */
export type BannerCulprit = boolean | undefined;

export interface ServiceBannerCulpritResult {
  bannerCulprit: BannerCulprit;
  setBannerCulprit: (value: boolean) => void;
  /**
   * Render ServiceUnreadyBanner now — while the query is loading OR
   * errored. A hung backend never surfaces an error on its own (that's the
   * whole problem this fixes), so callers must check proactively rather
   * than waiting for `error` to land.
   */
  showBanner: boolean;
}

interface ResolvedFor {
  error: unknown;
  workspaceName: string | undefined;
  wasLoading: boolean;
}

const INITIAL_RESOLVED_FOR: ResolvedFor = {
  error: undefined,
  workspaceName: undefined,
  wasLoading: false,
};

/**
 * Tracks the culprit banner's tri-state across a query's loading/error
 * lifecycle for a page that embeds <ServiceUnreadyBanner>.
 *
 * Resets to "pending" (undefined) at the start of each new cycle — a
 * changed error identity, a changed workspace, or a fresh loading cycle
 * (isLoading flipping false -> true) — so a stale result from a previous
 * cycle doesn't leak into the new one (e.g. "culprit found" lingering
 * after the backend has recovered).
 *
 * This must run at render time (a plain conditional `setState` call during
 * render), not inside a useEffect: effects fire child-first, so a parent
 * effect resetting this state would always run one tick after the banner's
 * own effect has already reported its result, clobbering it.
 * See https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
 */
export function useServiceBannerCulprit(
  workspaceName: string | undefined,
  error: unknown,
  isLoading: boolean
): ServiceBannerCulpritResult {
  const [bannerCulprit, setBannerCulprit] = useState<BannerCulprit>(undefined);
  const [resolvedFor, setResolvedFor] = useState<ResolvedFor>(INITIAL_RESOLVED_FOR);

  const startedNewLoad = isLoading && !resolvedFor.wasLoading;
  const changed =
    error !== resolvedFor.error || workspaceName !== resolvedFor.workspaceName || startedNewLoad;

  if (changed) {
    setResolvedFor({ error, workspaceName, wasLoading: isLoading });
    setBannerCulprit(workspaceName ? undefined : false);
  } else if (isLoading !== resolvedFor.wasLoading) {
    setResolvedFor((prev) => ({ ...prev, wasLoading: isLoading }));
  }

  return {
    bannerCulprit,
    setBannerCulprit,
    showBanner: isLoading || Boolean(error),
  };
}
