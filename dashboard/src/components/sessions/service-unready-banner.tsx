"use client";

/**
 * Culprit banner shared by the sessions and memory views.
 *
 * The sessions/memory proxy routes return a generic 503/502/504 whenever
 * the workspace's service group isn't ready — even when the service backing
 * *this* view is perfectly healthy and some other member of the group (e.g.
 * a crashlooping memory-api dragging down a sessions page, or vice versa)
 * is what's actually at fault. That misleads operators into debugging the
 * wrong service. Because the backend proxy fetches now time out (see
 * fetchWithTimeout) rather than hanging forever, callers can also render
 * this banner proactively — while the view is still loading, not just once
 * it has errored — so a hung backend surfaces the culprit immediately
 * instead of an endless spinner.
 *
 * This banner independently checks GET /api/workspaces/:name/services (the
 * per-service health endpoint, which reads k8s pod status directly and
 * stays fast even when the backend service itself is unresponsive) and —
 * if it can identify a genuinely unready member of the `default` service
 * group — names it and links to the workspace settings Services tab. If the
 * services check comes back healthy (a different, unrelated error), it
 * renders nothing so the caller's own generic error message is the only
 * thing shown. `resourceLabel` customizes the "Can't load {resourceLabel}"
 * copy per caller (sessions, memories, etc.).
 */

import { useEffect, useState } from "react";
import Link from "next/link";
import { AlertCircle } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import type { ServiceGroupHealth, WorkspaceServicesHealth } from "@/lib/k8s/service-health";

const DEFAULT_GROUP = "default";

interface ServiceUnreadyBannerProps {
  workspaceName: string;
  /**
   * What to call the thing that failed to load, e.g. "sessions" or
   * "memories" — interpolated into "Can't load {resourceLabel}". Defaults
   * to "sessions" since that's the banner's original caller.
   */
  resourceLabel?: string;
  /**
   * Invoked once the `/services` fetch resolves — `true` when a culprit
   * service was identified, `false` otherwise. Lets the caller decide
   * whether to fall back to a generic error message instead of stacking
   * one on top of this banner.
   */
  onResult?: (hasCulprit: boolean) => void;
}

interface Culprit {
  groupName: string;
  service: string;
}

/** Picks the group the sessions view queried: `default`, else the first group. */
function selectGroup(groups: ServiceGroupHealth[]): ServiceGroupHealth | undefined {
  return groups.find((group) => group.name === DEFAULT_GROUP) ?? groups[0];
}

/** Finds the first not-ready member of the relevant group, if any. */
function findCulprit(health: WorkspaceServicesHealth): Culprit | null {
  const group = selectGroup(health.groups);
  if (!group) return null;

  const unready = group.members.find((member) => !member.ready);
  if (!unready) return null;

  return { groupName: group.name, service: unready.service };
}

async function fetchCulprit(workspaceName: string): Promise<Culprit | null> {
  try {
    const response = await fetch(`/api/workspaces/${workspaceName}/services`);
    if (!response.ok) return null;
    const health: WorkspaceServicesHealth = await response.json();
    return findCulprit(health);
  } catch {
    return null;
  }
}

/**
 * Renders a banner naming the unready service behind a failed sessions
 * load, or nothing when the service group is actually healthy.
 */
export function ServiceUnreadyBanner({
  workspaceName,
  resourceLabel = "sessions",
  onResult,
}: Readonly<ServiceUnreadyBannerProps>) {
  const [culprit, setCulprit] = useState<Culprit | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchCulprit(workspaceName).then((found) => {
      if (cancelled) return;
      setCulprit(found);
      onResult?.(found !== null);
    });
    return () => {
      cancelled = true;
    };
  }, [workspaceName, onResult]);

  if (!culprit) return null;

  return (
    <Alert variant="destructive">
      <AlertCircle className="h-4 w-4" />
      <AlertTitle>Can&apos;t load {resourceLabel}</AlertTitle>
      <AlertDescription className="flex items-center justify-between gap-4">
        <span>
          Can&apos;t load {resourceLabel} — service group &apos;{culprit.groupName}&apos; not ready →{" "}
          {culprit.service} unhealthy
        </span>
        <Button asChild variant="outline" size="sm">
          <Link href={`/workspaces/${encodeURIComponent(workspaceName)}/settings?tab=services`}>
            Open Services
          </Link>
        </Button>
      </AlertDescription>
    </Alert>
  );
}
