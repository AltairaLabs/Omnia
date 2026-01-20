import { describe, it, expect } from "vitest";
import {
  OPEN_CORE_LICENSE,
  SOURCE_TYPE_FEATURE_MAP,
  JOB_TYPE_FEATURE_MAP,
  canUseSourceType,
  canUseJobType,
  canUseScheduling,
  canUseWorkerReplicas,
  canUseScenarioCount,
  isLicenseExpired,
  isEnterpriseLicense,
  type License,
} from "./license";

describe("license types", () => {
  describe("OPEN_CORE_LICENSE", () => {
    it("should have open-core tier", () => {
      expect(OPEN_CORE_LICENSE.tier).toBe("open-core");
    });

    it("should have id 'open-core'", () => {
      expect(OPEN_CORE_LICENSE.id).toBe("open-core");
    });

    it("should have all features disabled", () => {
      expect(OPEN_CORE_LICENSE.features.gitSource).toBe(false);
      expect(OPEN_CORE_LICENSE.features.ociSource).toBe(false);
      expect(OPEN_CORE_LICENSE.features.s3Source).toBe(false);
      expect(OPEN_CORE_LICENSE.features.loadTesting).toBe(false);
      expect(OPEN_CORE_LICENSE.features.dataGeneration).toBe(false);
      expect(OPEN_CORE_LICENSE.features.scheduling).toBe(false);
      expect(OPEN_CORE_LICENSE.features.distributedWorkers).toBe(false);
    });

    it("should have correct limits", () => {
      expect(OPEN_CORE_LICENSE.limits.maxScenarios).toBe(10);
      expect(OPEN_CORE_LICENSE.limits.maxWorkerReplicas).toBe(1);
    });
  });

  describe("SOURCE_TYPE_FEATURE_MAP", () => {
    it("should map configmap to null (always allowed)", () => {
      expect(SOURCE_TYPE_FEATURE_MAP.configmap).toBeNull();
    });

    it("should map git to gitSource feature", () => {
      expect(SOURCE_TYPE_FEATURE_MAP.git).toBe("gitSource");
    });

    it("should map oci to ociSource feature", () => {
      expect(SOURCE_TYPE_FEATURE_MAP.oci).toBe("ociSource");
    });

    it("should map s3 to s3Source feature", () => {
      expect(SOURCE_TYPE_FEATURE_MAP.s3).toBe("s3Source");
    });
  });

  describe("JOB_TYPE_FEATURE_MAP", () => {
    it("should map evaluation to null (always allowed)", () => {
      expect(JOB_TYPE_FEATURE_MAP.evaluation).toBeNull();
    });

    it("should map loadtest to loadTesting feature", () => {
      expect(JOB_TYPE_FEATURE_MAP.loadtest).toBe("loadTesting");
    });

    it("should map datagen to dataGeneration feature", () => {
      expect(JOB_TYPE_FEATURE_MAP.datagen).toBe("dataGeneration");
    });
  });

  describe("canUseSourceType", () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        gitSource: true,
        ociSource: true,
        s3Source: false,
      },
    };

    it("should always allow configmap source type", () => {
      expect(canUseSourceType(OPEN_CORE_LICENSE, "configmap")).toBe(true);
      expect(canUseSourceType(enterpriseLicense, "configmap")).toBe(true);
    });

    it("should not allow git source for open-core", () => {
      expect(canUseSourceType(OPEN_CORE_LICENSE, "git")).toBe(false);
    });

    it("should allow git source when feature is enabled", () => {
      expect(canUseSourceType(enterpriseLicense, "git")).toBe(true);
    });

    it("should not allow oci source for open-core", () => {
      expect(canUseSourceType(OPEN_CORE_LICENSE, "oci")).toBe(false);
    });

    it("should allow oci source when feature is enabled", () => {
      expect(canUseSourceType(enterpriseLicense, "oci")).toBe(true);
    });

    it("should not allow s3 source when feature is disabled", () => {
      expect(canUseSourceType(enterpriseLicense, "s3")).toBe(false);
    });

    it("should return false for unknown source types", () => {
      expect(canUseSourceType(OPEN_CORE_LICENSE, "unknown")).toBe(false);
      expect(canUseSourceType(enterpriseLicense, "foobar")).toBe(false);
    });
  });

  describe("canUseJobType", () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        loadTesting: true,
        dataGeneration: false,
      },
    };

    it("should always allow evaluation job type", () => {
      expect(canUseJobType(OPEN_CORE_LICENSE, "evaluation")).toBe(true);
      expect(canUseJobType(enterpriseLicense, "evaluation")).toBe(true);
    });

    it("should not allow loadtest for open-core", () => {
      expect(canUseJobType(OPEN_CORE_LICENSE, "loadtest")).toBe(false);
    });

    it("should allow loadtest when feature is enabled", () => {
      expect(canUseJobType(enterpriseLicense, "loadtest")).toBe(true);
    });

    it("should not allow datagen when feature is disabled", () => {
      expect(canUseJobType(enterpriseLicense, "datagen")).toBe(false);
    });

    it("should return false for unknown job types", () => {
      expect(canUseJobType(OPEN_CORE_LICENSE, "unknown")).toBe(false);
      expect(canUseJobType(enterpriseLicense, "foobar")).toBe(false);
    });
  });

  describe("canUseScheduling", () => {
    it("should not allow scheduling for open-core", () => {
      expect(canUseScheduling(OPEN_CORE_LICENSE)).toBe(false);
    });

    it("should allow scheduling when feature is enabled", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        features: { ...OPEN_CORE_LICENSE.features, scheduling: true },
      };
      expect(canUseScheduling(license)).toBe(true);
    });
  });

  describe("canUseWorkerReplicas", () => {
    it("should allow 1 replica for open-core", () => {
      expect(canUseWorkerReplicas(OPEN_CORE_LICENSE, 1)).toBe(true);
    });

    it("should not allow more than 1 replica for open-core", () => {
      expect(canUseWorkerReplicas(OPEN_CORE_LICENSE, 2)).toBe(false);
      expect(canUseWorkerReplicas(OPEN_CORE_LICENSE, 10)).toBe(false);
    });

    it("should allow unlimited replicas when limit is 0", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        limits: { ...OPEN_CORE_LICENSE.limits, maxWorkerReplicas: 0 },
      };
      expect(canUseWorkerReplicas(license, 100)).toBe(true);
      expect(canUseWorkerReplicas(license, 1000)).toBe(true);
    });

    it("should respect custom limits", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        limits: { ...OPEN_CORE_LICENSE.limits, maxWorkerReplicas: 5 },
      };
      expect(canUseWorkerReplicas(license, 5)).toBe(true);
      expect(canUseWorkerReplicas(license, 6)).toBe(false);
    });
  });

  describe("canUseScenarioCount", () => {
    it("should allow up to 10 scenarios for open-core", () => {
      expect(canUseScenarioCount(OPEN_CORE_LICENSE, 10)).toBe(true);
      expect(canUseScenarioCount(OPEN_CORE_LICENSE, 5)).toBe(true);
    });

    it("should not allow more than 10 scenarios for open-core", () => {
      expect(canUseScenarioCount(OPEN_CORE_LICENSE, 11)).toBe(false);
      expect(canUseScenarioCount(OPEN_CORE_LICENSE, 100)).toBe(false);
    });

    it("should allow unlimited scenarios when limit is 0", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        limits: { ...OPEN_CORE_LICENSE.limits, maxScenarios: 0 },
      };
      expect(canUseScenarioCount(license, 1000)).toBe(true);
      expect(canUseScenarioCount(license, 10000)).toBe(true);
    });

    it("should respect custom limits", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        limits: { ...OPEN_CORE_LICENSE.limits, maxScenarios: 50 },
      };
      expect(canUseScenarioCount(license, 50)).toBe(true);
      expect(canUseScenarioCount(license, 51)).toBe(false);
    });
  });

  describe("isLicenseExpired", () => {
    it("should return false for non-expired license", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        expiresAt: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
      };
      expect(isLicenseExpired(license)).toBe(false);
    });

    it("should return true for expired license", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        expiresAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
      };
      expect(isLicenseExpired(license)).toBe(true);
    });

    it("should return false for license expiring far in the future", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
      };
      expect(isLicenseExpired(license)).toBe(false);
    });
  });

  describe("isEnterpriseLicense", () => {
    it("should return false for open-core license", () => {
      expect(isEnterpriseLicense(OPEN_CORE_LICENSE)).toBe(false);
    });

    it("should return true for enterprise license", () => {
      const license: License = {
        ...OPEN_CORE_LICENSE,
        tier: "enterprise",
      };
      expect(isEnterpriseLicense(license)).toBe(true);
    });
  });
});
