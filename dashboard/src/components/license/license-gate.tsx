"use client";

import { type ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Lock, Sparkles, ExternalLink } from "lucide-react";
import { useLicense } from "@/hooks/use-license";
import { useEnterpriseConfig } from "@/hooks/use-runtime-config";
import type { LicenseFeatures } from "@/types/license";

const UPGRADE_URL = "https://altairalabs.ai/enterprise";

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
}

/**
 * Banner component prompting users to upgrade for enterprise features.
 *
 * @example
 * ```tsx
 * <UpgradeBanner feature="Git Sources" />
 * ```
 */
export function UpgradeBanner({
  feature,
  description,
  upgradeUrl = UPGRADE_URL,
  compact = false,
}: UpgradeBannerProps) {
  if (compact) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
        <Lock className="h-4 w-4 flex-shrink-0" />
        <span>
          {feature} requires an Enterprise license.{" "}
          <a
            href={upgradeUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="font-medium underline underline-offset-2 hover:no-underline"
          >
            Upgrade
          </a>
        </span>
      </div>
    );
  }

  return (
    <Alert className="border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950">
      <Sparkles className="h-4 w-4 text-amber-600 dark:text-amber-400" />
      <AlertTitle className="text-amber-800 dark:text-amber-200">
        Enterprise Feature
      </AlertTitle>
      <AlertDescription className="text-amber-700 dark:text-amber-300">
        <p className="mb-3">
          {description ?? `${feature} requires an Enterprise license. Upgrade to unlock unlimited access to all Arena Fleet features.`}
        </p>
        <Button variant="outline" size="sm" asChild>
          <a href={upgradeUrl} target="_blank" rel="noopener noreferrer">
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

  if (canUseFeature(feature)) {
    return (
      <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-900 dark:text-green-200">
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
            <a href={UPGRADE_URL} target="_blank" rel="noopener noreferrer">
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
export function EnterpriseGate({ children, fallback, featureName = "This feature" }: EnterpriseGateProps) {
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
export function EnterpriseUpgradePage({ featureName }: EnterpriseUpgradePageProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full p-8">
      <div className="max-w-md text-center space-y-6">
        <div className="mx-auto w-16 h-16 rounded-full bg-amber-100 dark:bg-amber-900 flex items-center justify-center">
          <Sparkles className="h-8 w-8 text-amber-600 dark:text-amber-400" />
        </div>
        <div className="space-y-2">
          <h1 className="text-2xl font-bold">Enterprise Feature</h1>
          <p className="text-muted-foreground">
            {featureName} is an enterprise feature. Upgrade to unlock Arena Fleet
            for AI agent evaluation, load testing, and data generation.
          </p>
        </div>
        <div className="space-y-3">
          <Button asChild size="lg" className="w-full">
            <a href={UPGRADE_URL} target="_blank" rel="noopener noreferrer">
              Upgrade to Enterprise
              <ExternalLink className="ml-2 h-4 w-4" />
            </a>
          </Button>
          <p className="text-sm text-muted-foreground">
            Contact{" "}
            <a
              href="mailto:sales@altairalabs.ai"
              className="text-primary hover:underline"
            >
              sales@altairalabs.ai
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
