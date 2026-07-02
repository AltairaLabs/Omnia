"use client";

import { useCallback, useSyncExternalStore, type ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Lock, Sparkles, ExternalLink, X } from "lucide-react";
import { useLicense } from "@/hooks/auth";
import { useEnterpriseConfig } from "@/hooks/core";
import { useBrand } from "@/hooks/use-brand";
import { getStatusClasses } from "@/lib/colors/status";
import type { LicenseFeatures } from "@/types/license";

const DISMISS_KEY_PREFIX = "omnia.upgradeBanner.dismissed.";

// useSyncExternalStore subscribe arg: localStorage doesn't fire on same-tab
// writes by default, so register a custom event channel for cross-component
// updates. The native `storage` event covers other tabs; we synthesise an
// in-tab event when our own writes happen.
const DISMISS_EVENT = "omnia:upgrade-banner-dismissed";

function subscribeToDismiss(onChange: () => void): () => void {
  if (typeof window === "undefined") {
    return () => {};
  }
  window.addEventListener("storage", onChange);
  window.addEventListener(DISMISS_EVENT, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(DISMISS_EVENT, onChange);
  };
}

const UPGRADE_URL = "https://altairalabs.ai/enterprise";
const SALES_EMAIL = "sales@altairalabs.ai";

/**
 * Feature names for display purposes.
 */
const FEATURE_DISPLAY_NAMES: Record<keyof LicenseFeatures, string> = {
  gitSource: "Git Sources",
  ociSource: "OCI Sources",
  s3Source: "S3 Sources",
  loadTesting: "Load Testing",
  dataGeneration: "Data Generation",
  scheduling: "Job Scheduling",
  distributedWorkers: "Distributed Workers",
  whiteLabel: "White-Label Branding",
  memoryEnterprise: "Memory Enterprise",
  privacyEnterprise: "Privacy & Compliance",
  policyProxy: "Policy Enforcement",
};

export interface LicenseGateProps {
  /** Feature required to show children */
  feature: keyof LicenseFeatures;
  /** Content to show when feature is enabled */
  children: ReactNode;
  /** Content to show when feature is disabled (defaults to UpgradeBanner) */
  fallback?: ReactNode;
}

/**
 * Gate component that conditionally renders content based on license features.
 *
 * @example
 * ```tsx
 * <LicenseGate feature="gitSource" fallback={<UpgradeBanner feature="Git Sources" />}>
 *   <GitSourceForm />
 * </LicenseGate>
 * ```
 */
export function LicenseGate({ feature, children, fallback }: LicenseGateProps) {
  const { canUseFeature } = useLicense();

  if (canUseFeature(feature)) {
    return <>{children}</>;
  }

  return (
    <>
      {fallback ?? (
        <UpgradeBanner feature={FEATURE_DISPLAY_NAMES[feature] ?? feature} />
      )}
    </>
  );
}

export interface RequireEnterpriseProps {
  /** Content to show when enterprise license is active */
  children: ReactNode;
  /** Content to show when not enterprise (defaults to UpgradeBanner) */
  fallback?: ReactNode;
}

/**
 * Gate component that requires an enterprise license.
 *
 * @example
 * ```tsx
 * <RequireEnterprise>
 *   <EnterpriseOnlyFeature />
 * </RequireEnterprise>
 * ```
 */
export function RequireEnterprise({ children, fallback }: RequireEnterpriseProps) {
  const { isEnterprise } = useLicense();

  if (isEnterprise) {
    return <>{children}</>;
  }

  return (
    <>
      {fallback ?? (
        <UpgradeBanner feature="Enterprise features" />
      )}
    </>
  );
}

export interface UpgradeBannerProps {
  /** Feature name to display in the banner */
  feature: string;
  /** Optional description text */
  description?: string;
  /** Custom upgrade URL */
  upgradeUrl?: string;
  /** Whether to show as a compact inline banner */
  compact?: boolean;
  /**
   * When set, renders a close button and persists the dismissed state in
   * localStorage under `omnia.upgradeBanner.dismissed.<dismissKey>`.
   * Subsequent renders return null until the key is cleared.
   */
  dismissKey?: string;
}

/**
 * Banner component prompting users to upgrade for enterprise features.
 *
 * @example
 * ```tsx
 * <UpgradeBanner feature="Git Sources" />
 * <UpgradeBanner feature="Privacy consent" dismissKey="memory-consent" compact />
 * ```
 */
export function UpgradeBanner({
  feature,
  description,
  upgradeUrl,
  compact = false,
  dismissKey,
}: UpgradeBannerProps) {
  const { brand } = useBrand();
  const resolvedUpgradeUrl = upgradeUrl ?? brand.links?.upgradeUrl ?? UPGRADE_URL;
  const storageKey = dismissKey ? DISMISS_KEY_PREFIX + dismissKey : null;
  const getSnapshot = useCallback(
    () => (storageKey ? window.localStorage.getItem(storageKey) === "1" : false),
    [storageKey],
  );
  // SSR snapshot: assume not-dismissed; client effect resyncs to localStorage.
  // Returning the dismissed state during SSR would briefly hide a banner the
  // server can't actually know about, defeating the point.
  const dismissed = useSyncExternalStore(
    subscribeToDismiss,
    getSnapshot,
    () => false,
  );

  if (dismissed) {
    return null;
  }

  const handleDismiss = () => {
    if (storageKey) {
      window.localStorage.setItem(storageKey, "1");
      window.dispatchEvent(new Event(DISMISS_EVENT));
    }
  };

  if (compact) {
    return (
      <div
        className="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
        data-testid="upgrade-banner-compact"
      >
        <Sparkles className="h-4 w-4 flex-shrink-0 text-amber-600 dark:text-amber-400" />
        <span className="flex-1">
          {description ?? `${feature} requires an Enterprise license.`}{" "}
          <a
            href={resolvedUpgradeUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="font-medium underline underline-offset-2 hover:no-underline"
          >
            Upgrade
          </a>
        </span>
        {storageKey && (
          <button
            type="button"
            onClick={handleDismiss}
            aria-label="Dismiss"
            data-testid="upgrade-banner-dismiss"
            className="rounded p-0.5 hover:bg-amber-100 dark:hover:bg-amber-900"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
    );
  }

  return (
    <Alert className="border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950">
      <Sparkles className="h-4 w-4 text-amber-600 dark:text-amber-400" />
      <AlertTitle className="flex items-center justify-between text-amber-800 dark:text-amber-200">
        <span>Enterprise Feature</span>
        {storageKey && (
          <button
            type="button"
            onClick={handleDismiss}
            aria-label="Dismiss"
            data-testid="upgrade-banner-dismiss"
            className="rounded p-0.5 text-amber-700 hover:bg-amber-100 dark:text-amber-300 dark:hover:bg-amber-900"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </AlertTitle>
      <AlertDescription className="text-amber-700 dark:text-amber-300">
        <p className="mb-3">
          {description ?? `${feature} requires an Enterprise license. Upgrade to unlock it and the rest of ${brand.productName}'s enterprise features.`}
        </p>
        <Button variant="outline" size="sm" asChild>
          <a href={resolvedUpgradeUrl} target="_blank" rel="noopener noreferrer">
            Upgrade to Enterprise
            <ExternalLink className="ml-2 h-3 w-3" />
          </a>
        </Button>
      </AlertDescription>
    </Alert>
  );
}

export interface FeatureBadgeProps {
  /** Feature to check */
  feature: keyof LicenseFeatures;
  /** Badge text when feature is available */
  availableText?: string;
  /** Badge text when feature requires enterprise */
  enterpriseText?: string;
}

/**
 * Badge component showing feature availability status.
 *
 * @example
 * ```tsx
 * <FeatureBadge feature="gitSource" />
 * ```
 */
export function FeatureBadge({
  feature,
  availableText = "Available",
  enterpriseText = "Enterprise",
}: FeatureBadgeProps) {
  const { canUseFeature } = useLicense();
  const availableBadge = getStatusClasses("success");

  if (canUseFeature(feature)) {
    return (
      <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${availableBadge.bg} ${availableBadge.text}`}>
        {availableText}
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-900 dark:text-amber-200">
      <Lock className="h-3 w-3" />
      {enterpriseText}
    </span>
  );
}

export interface LicenseInfoProps {
  /** Whether to show detailed license information */
  detailed?: boolean;
}

/**
 * Component displaying current license information.
 *
 * @example
 * ```tsx
 * <LicenseInfo detailed />
 * ```
 */
export function LicenseInfo({ detailed = false }: LicenseInfoProps) {
  const { license, isEnterprise, isExpired } = useLicense();
  const { brand } = useBrand();
  const upgradeUrl = brand.links?.upgradeUrl ?? UPGRADE_URL;

  if (!detailed) {
    return (
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">License:</span>
        {isEnterprise ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-800 dark:bg-purple-900 dark:text-purple-200">
            <Sparkles className="h-3 w-3" />
            Enterprise
          </span>
        ) : (
          <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-gray-800 dark:text-gray-200">
            Open Core
          </span>
        )}
        {isExpired && (
          <span className="inline-flex items-center rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800 dark:bg-red-900 dark:text-red-200">
            Expired
          </span>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-2 rounded-md border p-4">
      <div className="flex items-center justify-between">
        <span className="font-medium">License</span>
        {isEnterprise ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-800 dark:bg-purple-900 dark:text-purple-200">
            <Sparkles className="h-3 w-3" />
            Enterprise
          </span>
        ) : (
          <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-gray-800 dark:text-gray-200">
            Open Core
          </span>
        )}
      </div>
      {isEnterprise && (
        <>
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Customer</span>
            <span>{license.customer}</span>
          </div>
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Expires</span>
            <span className={isExpired ? "text-red-600" : ""}>
              {new Date(license.expiresAt).toLocaleDateString()}
              {isExpired && " (Expired)"}
            </span>
          </div>
        </>
      )}
      {!isEnterprise && (
        <div className="pt-2">
          <Button variant="outline" size="sm" asChild className="w-full">
            <a href={upgradeUrl} target="_blank" rel="noopener noreferrer">
              Upgrade to Enterprise
              <ExternalLink className="ml-2 h-3 w-3" />
            </a>
          </Button>
        </div>
      )}
    </div>
  );
}

export interface EnterpriseGateProps {
  /** Content to show when enterprise is enabled */
  children: ReactNode;
  /** Content to show when enterprise is not enabled but not hidden (defaults to EnterpriseUpgradePage) */
  fallback?: ReactNode;
  /** Feature name for the upgrade prompt */
  featureName?: string;
}

/**
 * Gate component that checks infrastructure-level enterprise enablement.
 *
 * This is different from LicenseGate which checks license features.
 * EnterpriseGate checks whether enterprise CRDs/controllers are deployed.
 *
 * Behavior:
 * - enterpriseEnabled=true: Show children normally
 * - enterpriseEnabled=false, hideEnterprise=false: Show upgrade prompt
 * - hideEnterprise=true: Render nothing (return null)
 *
 * @example
 * ```tsx
 * <EnterpriseGate featureName="Arena Fleet">
 *   <ArenaPage />
 * </EnterpriseGate>
 * ```
 */
export function EnterpriseGate({ children, fallback, featureName = "This feature" }: Readonly<EnterpriseGateProps>) {
  const { enterpriseEnabled, hideEnterprise, loading } = useEnterpriseConfig();

  // While loading, render nothing to avoid flash
  if (loading) {
    return null;
  }

  // If enterprise is enabled, show children
  if (enterpriseEnabled) {
    return <>{children}</>;
  }

  // If hideEnterprise is true, don't render anything
  if (hideEnterprise) {
    return null;
  }

  // Show upgrade prompt
  return (
    <>
      {fallback ?? (
        <EnterpriseUpgradePage featureName={featureName} />
      )}
    </>
  );
}

export interface EnterpriseUpgradePageProps {
  /** Feature name to display */
  featureName: string;
}

/**
 * Full-page upgrade prompt for enterprise features.
 */
export function EnterpriseUpgradePage({ featureName }: Readonly<EnterpriseUpgradePageProps>) {
  const { brand } = useBrand();
  const upgradeUrl = brand.links?.upgradeUrl ?? UPGRADE_URL;
  const salesEmail = brand.links?.sales ?? SALES_EMAIL;
  return (
    <div className="flex flex-col items-center justify-center h-full p-8">
      <div className="max-w-md text-center space-y-6">
        <div className="mx-auto w-16 h-16 rounded-full bg-amber-100 dark:bg-amber-900 flex items-center justify-center">
          <Sparkles className="h-8 w-8 text-amber-600 dark:text-amber-400" />
        </div>
        <div className="space-y-2">
          <h1 className="text-2xl font-bold">Enterprise Feature</h1>
          <p className="text-muted-foreground">
            {featureName} is an Enterprise feature. Upgrade to unlock it along
            with the rest of {brand.productName}&apos;s enterprise capabilities.
          </p>
        </div>
        <div className="space-y-3">
          <Button asChild size="lg" className="w-full">
            <a href={upgradeUrl} target="_blank" rel="noopener noreferrer">
              Upgrade to Enterprise
              <ExternalLink className="ml-2 h-4 w-4" />
            </a>
          </Button>
          <p className="text-sm text-muted-foreground">
            Contact{" "}
            <a
              href={`mailto:${salesEmail}`}
              className="text-primary hover:underline"
            >
              {salesEmail}
            </a>{" "}
            for pricing and trial licenses.
          </p>
        </div>
      </div>
    </div>
  );
}

/**
 * Hook to check if enterprise features should be visible in navigation.
 * Returns false if hideEnterprise is true, otherwise true.
 */
export function useShowEnterpriseNav() {
  const { hideEnterprise, loading } = useEnterpriseConfig();
  return { showEnterpriseNav: !hideEnterprise, loading };
}
