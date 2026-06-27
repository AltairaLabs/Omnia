// Auto-generated from omnia.altairalabs.ai_skillsources.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta } from "../common";

export interface SkillSourceSpec {
  /** configMap specifies a ConfigMap source. */
  configMap?: {
    /** key is the key within the ConfigMap containing the content.
     * Defaults to "pack.json". */
    key?: string;
    /** name is the name of the ConfigMap. */
    name: string;
  };
  /** createVersionOnSync mirrors the ArenaSource field: when true, each sync
   * produces a content-addressable snapshot alongside the HEAD pointer. */
  createVersionOnSync?: boolean;
  /** filter narrows which skills from the synced tree are exposed. */
  filter?: {
    /** exclude is a list of glob patterns to drop after include is applied. */
    exclude?: string[];
    /** include is a list of glob patterns (matched against each skill's
     * directory path relative to targetPath). An empty list means include all. */
    include?: string[];
    /** names pins individual skills by frontmatter name. Applied after
     * include/exclude. */
    names?: string[];
  };
  /** git specifies a Git repository source. */
  git?: {
    /** path is the path within the repository to the content.
     * Defaults to the repository root. */
    path?: string;
    /** ref specifies the Git reference to checkout.
     * If not specified, defaults to the default branch. */
    ref?: {
      /** branch to checkout. Takes precedence over tag and commit. */
      branch?: string;
      /** commit SHA to checkout. Used when branch and tag are not specified. */
      commit?: string;
      /** tag to checkout. Takes precedence over commit. */
      tag?: string;
    };
    /** secretRef references a Secret containing Git credentials.
     * The Secret should contain 'username' and 'password' keys for HTTPS,
     * or 'identity' and 'known_hosts' keys for SSH. */
    secretRef?: {
      /** key is the key within the Secret to use.
       * If not specified, the provider-appropriate key is used:
       * - ANTHROPIC_API_KEY for Claude
       * - OPENAI_API_KEY for OpenAI
       * - GEMINI_API_KEY for Gemini */
      key?: string;
      /** name is the name of the Secret. */
      name: string;
    };
    /** url is the Git repository URL.
     * Supports https:// and ssh:// protocols. */
    url: string;
  };
  /** interval is the reconciliation poll interval (e.g. "1h"). */
  interval: string;
  /** oci specifies an OCI registry source. */
  oci?: {
    /** insecure allows connecting to registries without TLS verification. */
    insecure?: boolean;
    /** secretRef references a Secret containing registry credentials.
     * The Secret should contain 'username' and 'password' keys,
     * or a '.dockerconfigjson' key for Docker config format. */
    secretRef?: {
      /** key is the key within the Secret to use.
       * If not specified, the provider-appropriate key is used:
       * - ANTHROPIC_API_KEY for Claude
       * - OPENAI_API_KEY for OpenAI
       * - GEMINI_API_KEY for Gemini */
      key?: string;
      /** name is the name of the Secret. */
      name: string;
    };
    /** url is the OCI artifact URL.
     * Format: oci://registry/repository:tag or oci://registry/repository@digest */
    url: string;
  };
  /** suspend prevents the source from being reconciled when set to true. */
  suspend?: boolean;
  /** targetPath is the path under the workspace content PVC where synced
   * content lands, e.g. "skills/anthropic". Defaults to "skills/{source-name}". */
  targetPath?: string;
  /** timeout is the maximum duration for a single fetch (e.g. "5m"). */
  timeout?: string;
  /** type selects the source variant. Exactly one of git/oci/configMap must be set. */
  type: "git" | "oci" | "configmap";
}

export interface SkillSourceStatus {
  /** artifact describes the last successfully fetched artifact. */
  artifact?: {
    /** checksum is the SHA256 checksum of the artifact. */
    checksum?: string;
    /** contentPath is the filesystem path where the content is synced.
     * This is relative to the workspace content volume root.
     * Workers can mount the PVC directly and access content at this path. */
    contentPath?: string;
    /** lastUpdateTime is when the artifact was last updated. */
    lastUpdateTime: string;
    /** revision is the source revision identifier.
     * For Git: branch@sha1:commit or tag@sha1:commit
     * For OCI: tag@sha256:digest
     * For ConfigMap: resourceVersion */
    revision: string;
    /** size is the size of the artifact in bytes. */
    size?: number;
    /** url is the URL where the artifact can be downloaded (legacy tar.gz serving).
     * Deprecated: Use contentPath for filesystem-based access. */
    url?: string;
    /** version is the content-addressable version hash (SHA256).
     * This identifies a specific immutable snapshot of the synced content. */
    version?: string;
  };
  /** conditions report detailed status. Known types:
   *   SourceAvailable — upstream reachable + fetched
   *   ContentValid    — every resolved SKILL.md frontmatter parses
   *                     cleanly, no duplicate names inside this source */
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
  /** lastFetchTime is the timestamp of the last fetch attempt. */
  lastFetchTime?: string;
  /** nextFetchTime is the scheduled time for the next fetch. */
  nextFetchTime?: string;
  /** observedGeneration tracks the last spec generation the controller saw. */
  observedGeneration?: number;
  /** phase reports the lifecycle phase. */
  phase?: "Pending" | "Initializing" | "Ready" | "Fetching" | "Error";
  /** skillCount is the number of SKILL.md directories that pass the filter
   * and parse successfully. */
  skillCount?: number;
}

export interface SkillSource {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "SkillSource";
  metadata: ObjectMeta;
  spec: SkillSourceSpec;
  status?: SkillSourceStatus;
}
