// Auto-generated from omnia.altairalabs.ai_sessionretentionpolicies.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta } from "../common";

export interface SessionRetentionPolicySpec {
  /** default defines the default retention configuration across all storage tiers. */
  default: {
    /** coldArchive configures the cold archive tier (e.g., S3, GCS). */
    coldArchive?: {
      /** compactionSchedule is a cron expression for when to run compaction/archival. */
      compactionSchedule?: string;
      /** enabled specifies whether cold archival is active. */
      enabled?: boolean;
      /** retentionDays is the number of days to retain data in the cold archive.
       * Required when enabled is true. */
      retentionDays?: number;
    };
    /** hotCache configures the Redis hot cache tier. */
    hotCache?: {
      /** enabled specifies whether the hot cache is active. */
      enabled?: boolean;
      /** maxMessagesPerSession is the maximum number of messages per session in the hot cache. */
      maxMessagesPerSession?: number;
      /** maxSessions is the maximum number of sessions to keep in the hot cache. */
      maxSessions?: number;
      /** ttlAfterInactive is the duration after which inactive sessions are evicted from the hot cache.
       * Must be a Go duration string (e.g., "24h", "30m", "1h30m"). */
      ttlAfterInactive?: string;
    };
    /** warmStore configures the Postgres warm store tier. */
    warmStore?: {
      /** partitionBy defines the partitioning strategy for warm store tables. */
      partitionBy?: "week";
      /** retentionDays is the number of days to retain data in the warm store. */
      retentionDays?: number;
    };
  };
  /** perWorkspace defines per-workspace retention overrides.
   * Map keys are Workspace resource names. */
  perWorkspace?: Record<string, {
    /** coldArchive overrides the cold archive configuration for this workspace. */
    coldArchive?: {
      /** compactionSchedule is a cron expression for when to run compaction/archival. */
      compactionSchedule?: string;
      /** enabled specifies whether cold archival is active. */
      enabled?: boolean;
      /** retentionDays is the number of days to retain data in the cold archive.
       * Required when enabled is true. */
      retentionDays?: number;
    };
    /** warmStore overrides the warm store configuration for this workspace. */
    warmStore?: {
      /** partitionBy defines the partitioning strategy for warm store tables. */
      partitionBy?: "week";
      /** retentionDays is the number of days to retain data in the warm store. */
      retentionDays?: number;
    };
  }>;
}

export interface SessionRetentionPolicyStatus {
  /** conditions represent the current state of the SessionRetentionPolicy resource. */
  conditions?: {
    /** lastTransitionTime is the last time the condition transitioned from one status to another.
     * This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable. */
    lastTransitionTime: string;
    /** message is a human readable message indicating details about the transition.
     * This may be an empty string. */
    message: string;
    /** observedGeneration represents the .metadata.generation that the condition was set based upon.
     * For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
     * with respect to the current state of the instance. */
    observedGeneration?: number;
    /** reason contains a programmatic identifier indicating the reason for the condition's last transition.
     * Producers of specific condition types may define expected values and meanings for this field,
     * and whether the values are considered a guaranteed API.
     * The value should be a CamelCase string.
     * This field may not be empty. */
    reason: string;
    /** status of the condition, one of True, False, Unknown. */
    status: "True" | "False" | "Unknown";
    /** type of condition in CamelCase or in foo.example.com/CamelCase. */
    type: string;
  }[];
  /** observedGeneration is the most recent generation observed by the controller. */
  observedGeneration?: number;
  /** phase represents the current lifecycle phase of the policy. */
  phase?: "Active" | "Error";
  /** workspaceCount is the number of workspaces with per-workspace overrides that were resolved. */
  workspaceCount?: number;
}

export interface SessionRetentionPolicy {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "SessionRetentionPolicy";
  metadata: ObjectMeta;
  spec: SessionRetentionPolicySpec;
  status?: SessionRetentionPolicyStatus;
}
