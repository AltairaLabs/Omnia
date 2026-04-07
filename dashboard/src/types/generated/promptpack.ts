// Auto-generated from omnia.altairalabs.ai_promptpacks.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta } from "../common";

export interface PromptPackSpec {
  /** source specifies where the prompt configuration is stored. */
  source: {
    /** configMapRef references a ConfigMap containing the prompt configuration.
     * Required when type is "configmap". */
    configMapRef?: {
      /** Name of the referent.
       * This field is effectively required, but due to backwards compatibility is
       * allowed to be empty. Instances of this type with an empty value here are
       * almost certainly wrong.
       * More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names */
      name?: string;
    };
    /** type specifies the type of source for the prompt configuration.
     * Currently only "configmap" is supported. */
    type: "configmap";
  };
  /** version specifies the semantic version of this prompt pack.
   * Must follow semver format (e.g., "1.0.0", "2.1.0-beta.1"). */
  version: string;
}

export interface PromptPackStatus {
  /** activeVersion is the currently active version serving production traffic. */
  activeVersion?: string;
  /** conditions represent the current state of the PromptPack resource. */
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
  /** lastUpdated is the timestamp of the last status update. */
  lastUpdated?: string;
  /** phase represents the current lifecycle phase of the PromptPack. */
  phase?: "Pending" | "Active" | "Superseded" | "Failed";
}

export interface PromptPack {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "PromptPack";
  metadata: ObjectMeta;
  spec: PromptPackSpec;
  status?: PromptPackStatus;
}
