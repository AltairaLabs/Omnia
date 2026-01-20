/**
 * License types for Arena Fleet license gating.
 */

/**
 * License tier - open-core or enterprise.
 */
export type LicenseTier = "open-core" | "enterprise";

/**
 * Features enabled by the license.
 */
export interface LicenseFeatures {
  /** Git repository sources for ArenaSources */
  gitSource: boolean;
  /** OCI registry sources for ArenaSources */
  ociSource: boolean;
  /** S3 sources for ArenaSources */
  s3Source: boolean;
  /** Load testing job type */
  loadTesting: boolean;
  /** Data generation job type */
  dataGeneration: boolean;
  /** Cron-based job scheduling */
  scheduling: boolean;
  /** Multiple worker replicas */
  distributedWorkers: boolean;
}

/**
 * Resource limits from the license.
 */
export interface LicenseLimits {
  /** Maximum number of scenarios (0 = unlimited) */
  maxScenarios: number;
  /** Maximum number of worker replicas (0 = unlimited) */
  maxWorkerReplicas: number;
}

/**
 * License information.
 */
export interface License {
  /** Unique license identifier */
  id: string;
  /** License tier */
  tier: LicenseTier;
  /** Customer name */
  customer: string;
  /** Enabled features */
  features: LicenseFeatures;
  /** Resource limits */
  limits: LicenseLimits;
  /** When the license was issued */
  issuedAt: string;
  /** When the license expires */
  expiresAt: string;
}

/**
 * Default open-core license.
 */
export const OPEN_CORE_LICENSE: License = {
  id: "open-core",
  tier: "open-core",
  customer: "Open Core User",
  features: {
    gitSource: false,
    ociSource: false,
    s3Source: false,
    loadTesting: false,
    dataGeneration: false,
    scheduling: false,
    distributedWorkers: false,
  },
  limits: {
    maxScenarios: 10,
    maxWorkerReplicas: 1,
  },
  issuedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 100 * 365 * 24 * 60 * 60 * 1000).toISOString(),
};

/**
 * Map of source types to feature keys.
 */
export const SOURCE_TYPE_FEATURE_MAP: Record<string, keyof LicenseFeatures | null> = {
  configmap: null, // Always allowed
  git: "gitSource",
  oci: "ociSource",
  s3: "s3Source",
};

/**
 * Map of job types to feature keys.
 */
export const JOB_TYPE_FEATURE_MAP: Record<string, keyof LicenseFeatures | null> = {
  evaluation: null, // Always allowed
  loadtest: "loadTesting",
  datagen: "dataGeneration",
};

/**
 * Check if a source type is allowed by the license.
 */
export function canUseSourceType(license: License, sourceType: string): boolean {
  const featureKey = SOURCE_TYPE_FEATURE_MAP[sourceType];
  if (featureKey === null) {
    // Always allowed (configmap)
    return true;
  }
  if (featureKey === undefined) {
    // Unknown source type
    return false;
  }
  return license.features[featureKey];
}

/**
 * Check if a job type is allowed by the license.
 */
export function canUseJobType(license: License, jobType: string): boolean {
  const featureKey = JOB_TYPE_FEATURE_MAP[jobType];
  if (featureKey === null) {
    // Always allowed (evaluation)
    return true;
  }
  if (featureKey === undefined) {
    // Unknown job type
    return false;
  }
  return license.features[featureKey];
}

/**
 * Check if scheduling is allowed by the license.
 */
export function canUseScheduling(license: License): boolean {
  return license.features.scheduling;
}

/**
 * Check if the given number of worker replicas is allowed.
 */
export function canUseWorkerReplicas(license: License, replicas: number): boolean {
  if (license.limits.maxWorkerReplicas === 0) {
    // Unlimited
    return true;
  }
  return replicas <= license.limits.maxWorkerReplicas;
}

/**
 * Check if the given number of scenarios is allowed.
 */
export function canUseScenarioCount(license: License, count: number): boolean {
  if (license.limits.maxScenarios === 0) {
    // Unlimited
    return true;
  }
  return count <= license.limits.maxScenarios;
}

/**
 * Check if the license is expired.
 */
export function isLicenseExpired(license: License): boolean {
  return new Date() > new Date(license.expiresAt);
}

/**
 * Check if this is an enterprise license.
 */
export function isEnterpriseLicense(license: License): boolean {
  return license.tier === "enterprise";
}
