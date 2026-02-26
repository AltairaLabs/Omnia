"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type License,
  type LicenseFeatures,
  OPEN_CORE_LICENSE,
  canUseSourceType,
  canUseJobType,
  canUseScheduling,
  canUseWorkerReplicas,
  canUseScenarioCount,
  isLicenseExpired,
  isEnterpriseLicense,
} from "@/types/license";

const LICENSE_REFRESH_INTERVAL = 5 * 60 * 1000; // 5 minutes

/**
 * Fetcher function for license data.
 */
async function fetcher(): Promise<License> {
  const response = await fetch("/api/license");
  if (!response.ok) {
    throw new Error("Failed to fetch license");
  }
  return response.json();
}

/**
 * Return type for useLicense hook.
 */
export interface UseLicenseResult {
  /** Current license data */
  license: License;
  /** Whether license data is loading */
  isLoading: boolean;
  /** Error if license fetch failed */
  error: Error | undefined;
  /** Check if a feature is enabled */
  canUseFeature: (feature: keyof LicenseFeatures) => boolean;
  /** Check if a source type is allowed */
  canUseSourceType: (sourceType: string) => boolean;
  /** Check if a job type is allowed */
  canUseJobType: (jobType: string) => boolean;
  /** Check if scheduling is allowed */
  canUseScheduling: () => boolean;
  /** Check if replica count is allowed */
  canUseWorkerReplicas: (replicas: number) => boolean;
  /** Check if scenario count is allowed */
  canUseScenarioCount: (count: number) => boolean;
  /** Whether the license is expired */
  isExpired: boolean;
  /** Whether this is an enterprise license */
  isEnterprise: boolean;
  /** Refresh license data */
  refresh: () => void;
}

/**
 * Hook to access license information and check feature availability.
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { license, canUseFeature, isEnterprise } = useLicense();
 *
 *   if (!canUseFeature("gitSource")) {
 *     return <UpgradeBanner />;
 *   }
 *
 *   return <GitSourceForm />;
 * }
 * ```
 */
export function useLicense(): UseLicenseResult {
  const queryClient = useQueryClient();

  const { data, error, isLoading } = useQuery({
    queryKey: ["license"],
    queryFn: fetcher,
    placeholderData: OPEN_CORE_LICENSE,
    refetchInterval: LICENSE_REFRESH_INTERVAL,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Use open-core license as fallback
  const license = data ?? OPEN_CORE_LICENSE;

  return {
    license,
    isLoading,
    error: error as Error | undefined,
    canUseFeature: (feature: keyof LicenseFeatures) => license.features[feature],
    canUseSourceType: (sourceType: string) => canUseSourceType(license, sourceType),
    canUseJobType: (jobType: string) => canUseJobType(license, jobType),
    canUseScheduling: () => canUseScheduling(license),
    canUseWorkerReplicas: (replicas: number) => canUseWorkerReplicas(license, replicas),
    canUseScenarioCount: (count: number) => canUseScenarioCount(license, count),
    isExpired: isLicenseExpired(license),
    isEnterprise: isEnterpriseLicense(license),
    refresh: () => queryClient.invalidateQueries({ queryKey: ["license"] }),
  };
}
