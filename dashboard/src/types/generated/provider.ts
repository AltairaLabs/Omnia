// Auto-generated from omnia.altairalabs.ai_providers.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta } from "../common";

export interface ProviderSpec {
  /** baseURL overrides the provider's default API endpoint.
   * Useful for proxies or self-hosted models. */
  baseURL?: string;
  /** defaults contains provider tuning parameters. */
  defaults?: {
    /** maxTokens limits the maximum number of tokens in the response. */
    maxTokens?: number;
    /** temperature controls randomness in responses (0.0-2.0).
     * Lower values make output more focused and deterministic.
     * Specified as a string to support decimal values (e.g., "0.7"). */
    temperature?: string;
    /** topP controls nucleus sampling (0.0-1.0).
     * Specified as a string to support decimal values (e.g., "0.9"). */
    topP?: string;
  };
  /** model specifies the model identifier (e.g., "claude-sonnet-4-20250514", "gpt-4o").
   * If not specified, the provider's default model is used. */
  model?: string;
  /** pricing configures cost tracking for this provider.
   * If not specified, PromptKit's built-in pricing is used. */
  pricing?: {
    /** cachedCostPer1K is the cost per 1000 cached tokens (e.g., "0.0003").
     * Cached tokens have reduced cost with some providers. */
    cachedCostPer1K?: string;
    /** inputCostPer1K is the cost per 1000 input tokens (e.g., "0.003"). */
    inputCostPer1K?: string;
    /** outputCostPer1K is the cost per 1000 output tokens (e.g., "0.015"). */
    outputCostPer1K?: string;
  };
  /** secretRef references a Secret containing API credentials. */
  secretRef: {
    /** key is the key within the Secret to use.
     * If not specified, the provider-appropriate key is used:
     * - ANTHROPIC_API_KEY for Claude
     * - OPENAI_API_KEY for OpenAI
     * - GEMINI_API_KEY for Gemini */
    key?: string;
    /** name is the name of the Secret. */
    name: string;
  };
  /** type specifies the provider type. */
  type: "claude" | "openai" | "gemini" | "ollama" | "mock";
  /** validateCredentials enables credential validation on reconciliation.
   * When enabled, the controller attempts to verify credentials with the provider. */
  validateCredentials?: boolean;
}

export interface ProviderStatus {
  /** conditions represent the current state of the Provider resource. */
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
  /** lastValidatedAt is the timestamp of the last successful credential validation. */
  lastValidatedAt?: string;
  /** observedGeneration is the most recent generation observed by the controller. */
  observedGeneration?: number;
  /** phase represents the current lifecycle phase of the Provider. */
  phase?: "Ready" | "Error";
}

export interface Provider {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "Provider";
  metadata: ObjectMeta;
  spec: ProviderSpec;
  status?: ProviderStatus;
}
