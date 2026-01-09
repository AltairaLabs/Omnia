// Auto-generated from omnia.altairalabs.ai_agentruntimes.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta } from "../common";

export interface AgentRuntimeSpec {
  /** facade configures the client-facing connection interface. */
  facade: {
    /** handler specifies the message handler mode.
     * "echo" returns input messages back (for testing connectivity).
     * "demo" provides streaming responses with simulated tool calls (for demos).
     * "runtime" uses the runtime framework in the container (default, for production). */
    handler?: "echo" | "demo" | "runtime";
    /** image overrides the default facade container image.
     * Use this to specify a custom facade image or private registry. */
    image?: string;
    /** port is the port number for the facade service. */
    port?: number;
    /** type specifies the facade protocol type. */
    type: "websocket" | "grpc";
  };
  /** framework specifies which agent framework to use.
   * Supports PromptKit, LangChain, CrewAI, AutoGen, or a custom image.
   * If not specified, defaults to PromptKit. */
  framework?: {
    /** image overrides the default container image for the framework.
     * Required when type is "custom".
     * For built-in frameworks, this allows using a custom build or private registry. */
    image?: string;
    /** type specifies the agent framework to use. */
    type: "promptkit" | "langchain" | "crewai" | "autogen" | "custom";
    /** version specifies the framework version to use.
     * If not specified, the latest supported version is used. */
    version?: string;
  };
  /** promptPackRef references the PromptPack containing agent prompts and configuration. */
  promptPackRef: {
    /** name is the name of the PromptPack resource. */
    name: string;
    /** track specifies which release track to follow (e.g., "stable", "canary").
     * Only used if version is not specified. */
    track?: string;
    /** version specifies a specific version of the PromptPack to use.
     * If not specified, the track field is used instead. */
    version?: string;
  };
  /** provider configures the LLM provider inline (type, model, credentials, tuning).
   * If not specified and providerRef is also not specified, PromptKit's auto-detection
   * is used with credentials from a secret named "<agentruntime-name>-provider" if it exists. */
  provider?: {
    /** additionalConfig contains provider-specific settings passed to PromptKit.
     * For Ollama: "keep_alive" (e.g., "5m") to keep model loaded between requests.
     * For Mock: "mock_config" path to mock responses YAML file. */
    additionalConfig?: Record<string, string>;
    /** baseURL overrides the provider's default API endpoint.
     * Useful for proxies or self-hosted models. */
    baseURL?: string;
    /** config contains provider tuning parameters. */
    config?: {
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
    /** secretRef references a Secret containing API credentials.
     * The secret should contain a key matching the provider's expected env var:
     * - ANTHROPIC_API_KEY for Claude
     * - OPENAI_API_KEY for OpenAI
     * - GEMINI_API_KEY or GOOGLE_API_KEY for Gemini
     * Or use "api-key" as a generic key name. */
    secretRef?: {
      /** Name of the referent.
       * This field is effectively required, but due to backwards compatibility is
       * allowed to be empty. Instances of this type with an empty value here are
       * almost certainly wrong.
       * More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names */
      name?: string;
    };
    /** type specifies the provider type.
     * "auto" uses PromptKit's auto-detection based on available credentials.
     * "claude", "openai", "gemini" explicitly select a provider. */
    type?: "auto" | "claude" | "openai" | "gemini" | "ollama" | "mock";
  };
  /** providerRef references a Provider resource for LLM configuration.
   * If specified, the referenced Provider's configuration is used.
   * If both providerRef and provider are specified, providerRef takes precedence. */
  providerRef?: {
    /** name is the name of the Provider resource. */
    name: string;
    /** namespace is the namespace of the Provider resource.
     * If not specified, the same namespace as the AgentRuntime is used. */
    namespace?: string;
  };
  /** runtime configures deployment settings like replicas and resources. */
  runtime?: {
    /** affinity defines affinity rules for pod scheduling. */
    affinity?: {
      /** Describes node affinity scheduling rules for the pod. */
      nodeAffinity?: {
        /** The scheduler will prefer to schedule pods to nodes that satisfy
         * the affinity expressions specified by this field, but it may choose
         * a node that violates one or more of the expressions. The node that is
         * most preferred is the one with the greatest sum of weights, i.e.
         * for each node that meets all of the scheduling requirements (resource
         * request, requiredDuringScheduling affinity expressions, etc.),
         * compute a sum by iterating through the elements of this field and adding
         * "weight" to the sum if the node matches the corresponding matchExpressions; the
         * node(s) with the highest sum are the most preferred. */
        preferredDuringSchedulingIgnoredDuringExecution?: {
          /** A node selector term, associated with the corresponding weight. */
          preference: {
            /** A list of node selector requirements by node's labels. */
            matchExpressions?: {
              /** The label key that the selector applies to. */
              key: string;
              /** Represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. */
              operator: string;
              /** An array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. If the operator is Gt or Lt, the values
               * array must have a single element, which will be interpreted as an integer.
               * This array is replaced during a strategic merge patch. */
              values?: string[];
            }[];
            /** A list of node selector requirements by node's fields. */
            matchFields?: {
              /** The label key that the selector applies to. */
              key: string;
              /** Represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. */
              operator: string;
              /** An array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. If the operator is Gt or Lt, the values
               * array must have a single element, which will be interpreted as an integer.
               * This array is replaced during a strategic merge patch. */
              values?: string[];
            }[];
          };
          /** Weight associated with matching the corresponding nodeSelectorTerm, in the range 1-100. */
          weight: number;
        }[];
        /** If the affinity requirements specified by this field are not met at
         * scheduling time, the pod will not be scheduled onto the node.
         * If the affinity requirements specified by this field cease to be met
         * at some point during pod execution (e.g. due to an update), the system
         * may or may not try to eventually evict the pod from its node. */
        requiredDuringSchedulingIgnoredDuringExecution?: {
          /** Required. A list of node selector terms. The terms are ORed. */
          nodeSelectorTerms: {
            /** A list of node selector requirements by node's labels. */
            matchExpressions?: {
              /** The label key that the selector applies to. */
              key: string;
              /** Represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. */
              operator: string;
              /** An array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. If the operator is Gt or Lt, the values
               * array must have a single element, which will be interpreted as an integer.
               * This array is replaced during a strategic merge patch. */
              values?: string[];
            }[];
            /** A list of node selector requirements by node's fields. */
            matchFields?: {
              /** The label key that the selector applies to. */
              key: string;
              /** Represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. */
              operator: string;
              /** An array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. If the operator is Gt or Lt, the values
               * array must have a single element, which will be interpreted as an integer.
               * This array is replaced during a strategic merge patch. */
              values?: string[];
            }[];
          }[];
        };
      };
      /** Describes pod affinity scheduling rules (e.g. co-locate this pod in the same node, zone, etc. as some other pod(s)). */
      podAffinity?: {
        /** The scheduler will prefer to schedule pods to nodes that satisfy
         * the affinity expressions specified by this field, but it may choose
         * a node that violates one or more of the expressions. The node that is
         * most preferred is the one with the greatest sum of weights, i.e.
         * for each node that meets all of the scheduling requirements (resource
         * request, requiredDuringScheduling affinity expressions, etc.),
         * compute a sum by iterating through the elements of this field and adding
         * "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the
         * node(s) with the highest sum are the most preferred. */
        preferredDuringSchedulingIgnoredDuringExecution?: {
          /** Required. A pod affinity term, associated with the corresponding weight. */
          podAffinityTerm: {
            /** A label query over a set of resources, in this case pods.
             * If it's null, this PodAffinityTerm matches with no Pods. */
            labelSelector?: {
              /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
              matchExpressions?: {
                /** key is the label key that the selector applies to. */
                key: string;
                /** operator represents a key's relationship to a set of values.
                 * Valid operators are In, NotIn, Exists and DoesNotExist. */
                operator: string;
                /** values is an array of string values. If the operator is In or NotIn,
                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                 * the values array must be empty. This array is replaced during a strategic
                 * merge patch. */
                values?: string[];
              }[];
              /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
               * map is equivalent to an element of matchExpressions, whose key field is "key", the
               * operator is "In", and the values array contains only "value". The requirements are ANDed. */
              matchLabels?: Record<string, string>;
            };
            /** MatchLabelKeys is a set of pod label keys to select which pods will
             * be taken into consideration. The keys are used to lookup values from the
             * incoming pod labels, those key-value labels are merged with `labelSelector` as `key in (value)`
             * to select the group of existing pods which pods will be taken into consideration
             * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
             * pod labels will be ignored. The default value is empty.
             * The same key is forbidden to exist in both matchLabelKeys and labelSelector.
             * Also, matchLabelKeys cannot be set when labelSelector isn't set. */
            matchLabelKeys?: string[];
            /** MismatchLabelKeys is a set of pod label keys to select which pods will
             * be taken into consideration. The keys are used to lookup values from the
             * incoming pod labels, those key-value labels are merged with `labelSelector` as `key notin (value)`
             * to select the group of existing pods which pods will be taken into consideration
             * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
             * pod labels will be ignored. The default value is empty.
             * The same key is forbidden to exist in both mismatchLabelKeys and labelSelector.
             * Also, mismatchLabelKeys cannot be set when labelSelector isn't set. */
            mismatchLabelKeys?: string[];
            /** A label query over the set of namespaces that the term applies to.
             * The term is applied to the union of the namespaces selected by this field
             * and the ones listed in the namespaces field.
             * null selector and null or empty namespaces list means "this pod's namespace".
             * An empty selector ({}) matches all namespaces. */
            namespaceSelector?: {
              /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
              matchExpressions?: {
                /** key is the label key that the selector applies to. */
                key: string;
                /** operator represents a key's relationship to a set of values.
                 * Valid operators are In, NotIn, Exists and DoesNotExist. */
                operator: string;
                /** values is an array of string values. If the operator is In or NotIn,
                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                 * the values array must be empty. This array is replaced during a strategic
                 * merge patch. */
                values?: string[];
              }[];
              /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
               * map is equivalent to an element of matchExpressions, whose key field is "key", the
               * operator is "In", and the values array contains only "value". The requirements are ANDed. */
              matchLabels?: Record<string, string>;
            };
            /** namespaces specifies a static list of namespace names that the term applies to.
             * The term is applied to the union of the namespaces listed in this field
             * and the ones selected by namespaceSelector.
             * null or empty namespaces list and null namespaceSelector means "this pod's namespace". */
            namespaces?: string[];
            /** This pod should be co-located (affinity) or not co-located (anti-affinity) with the pods matching
             * the labelSelector in the specified namespaces, where co-located is defined as running on a node
             * whose value of the label with key topologyKey matches that of any node on which any of the
             * selected pods is running.
             * Empty topologyKey is not allowed. */
            topologyKey: string;
          };
          /** weight associated with matching the corresponding podAffinityTerm,
           * in the range 1-100. */
          weight: number;
        }[];
        /** If the affinity requirements specified by this field are not met at
         * scheduling time, the pod will not be scheduled onto the node.
         * If the affinity requirements specified by this field cease to be met
         * at some point during pod execution (e.g. due to a pod label update), the
         * system may or may not try to eventually evict the pod from its node.
         * When there are multiple elements, the lists of nodes corresponding to each
         * podAffinityTerm are intersected, i.e. all terms must be satisfied. */
        requiredDuringSchedulingIgnoredDuringExecution?: {
          /** A label query over a set of resources, in this case pods.
           * If it's null, this PodAffinityTerm matches with no Pods. */
          labelSelector?: {
            /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
            matchExpressions?: {
              /** key is the label key that the selector applies to. */
              key: string;
              /** operator represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists and DoesNotExist. */
              operator: string;
              /** values is an array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. This array is replaced during a strategic
               * merge patch. */
              values?: string[];
            }[];
            /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
             * map is equivalent to an element of matchExpressions, whose key field is "key", the
             * operator is "In", and the values array contains only "value". The requirements are ANDed. */
            matchLabels?: Record<string, string>;
          };
          /** MatchLabelKeys is a set of pod label keys to select which pods will
           * be taken into consideration. The keys are used to lookup values from the
           * incoming pod labels, those key-value labels are merged with `labelSelector` as `key in (value)`
           * to select the group of existing pods which pods will be taken into consideration
           * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
           * pod labels will be ignored. The default value is empty.
           * The same key is forbidden to exist in both matchLabelKeys and labelSelector.
           * Also, matchLabelKeys cannot be set when labelSelector isn't set. */
          matchLabelKeys?: string[];
          /** MismatchLabelKeys is a set of pod label keys to select which pods will
           * be taken into consideration. The keys are used to lookup values from the
           * incoming pod labels, those key-value labels are merged with `labelSelector` as `key notin (value)`
           * to select the group of existing pods which pods will be taken into consideration
           * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
           * pod labels will be ignored. The default value is empty.
           * The same key is forbidden to exist in both mismatchLabelKeys and labelSelector.
           * Also, mismatchLabelKeys cannot be set when labelSelector isn't set. */
          mismatchLabelKeys?: string[];
          /** A label query over the set of namespaces that the term applies to.
           * The term is applied to the union of the namespaces selected by this field
           * and the ones listed in the namespaces field.
           * null selector and null or empty namespaces list means "this pod's namespace".
           * An empty selector ({}) matches all namespaces. */
          namespaceSelector?: {
            /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
            matchExpressions?: {
              /** key is the label key that the selector applies to. */
              key: string;
              /** operator represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists and DoesNotExist. */
              operator: string;
              /** values is an array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. This array is replaced during a strategic
               * merge patch. */
              values?: string[];
            }[];
            /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
             * map is equivalent to an element of matchExpressions, whose key field is "key", the
             * operator is "In", and the values array contains only "value". The requirements are ANDed. */
            matchLabels?: Record<string, string>;
          };
          /** namespaces specifies a static list of namespace names that the term applies to.
           * The term is applied to the union of the namespaces listed in this field
           * and the ones selected by namespaceSelector.
           * null or empty namespaces list and null namespaceSelector means "this pod's namespace". */
          namespaces?: string[];
          /** This pod should be co-located (affinity) or not co-located (anti-affinity) with the pods matching
           * the labelSelector in the specified namespaces, where co-located is defined as running on a node
           * whose value of the label with key topologyKey matches that of any node on which any of the
           * selected pods is running.
           * Empty topologyKey is not allowed. */
          topologyKey: string;
        }[];
      };
      /** Describes pod anti-affinity scheduling rules (e.g. avoid putting this pod in the same node, zone, etc. as some other pod(s)). */
      podAntiAffinity?: {
        /** The scheduler will prefer to schedule pods to nodes that satisfy
         * the anti-affinity expressions specified by this field, but it may choose
         * a node that violates one or more of the expressions. The node that is
         * most preferred is the one with the greatest sum of weights, i.e.
         * for each node that meets all of the scheduling requirements (resource
         * request, requiredDuringScheduling anti-affinity expressions, etc.),
         * compute a sum by iterating through the elements of this field and subtracting
         * "weight" from the sum if the node has pods which matches the corresponding podAffinityTerm; the
         * node(s) with the highest sum are the most preferred. */
        preferredDuringSchedulingIgnoredDuringExecution?: {
          /** Required. A pod affinity term, associated with the corresponding weight. */
          podAffinityTerm: {
            /** A label query over a set of resources, in this case pods.
             * If it's null, this PodAffinityTerm matches with no Pods. */
            labelSelector?: {
              /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
              matchExpressions?: {
                /** key is the label key that the selector applies to. */
                key: string;
                /** operator represents a key's relationship to a set of values.
                 * Valid operators are In, NotIn, Exists and DoesNotExist. */
                operator: string;
                /** values is an array of string values. If the operator is In or NotIn,
                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                 * the values array must be empty. This array is replaced during a strategic
                 * merge patch. */
                values?: string[];
              }[];
              /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
               * map is equivalent to an element of matchExpressions, whose key field is "key", the
               * operator is "In", and the values array contains only "value". The requirements are ANDed. */
              matchLabels?: Record<string, string>;
            };
            /** MatchLabelKeys is a set of pod label keys to select which pods will
             * be taken into consideration. The keys are used to lookup values from the
             * incoming pod labels, those key-value labels are merged with `labelSelector` as `key in (value)`
             * to select the group of existing pods which pods will be taken into consideration
             * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
             * pod labels will be ignored. The default value is empty.
             * The same key is forbidden to exist in both matchLabelKeys and labelSelector.
             * Also, matchLabelKeys cannot be set when labelSelector isn't set. */
            matchLabelKeys?: string[];
            /** MismatchLabelKeys is a set of pod label keys to select which pods will
             * be taken into consideration. The keys are used to lookup values from the
             * incoming pod labels, those key-value labels are merged with `labelSelector` as `key notin (value)`
             * to select the group of existing pods which pods will be taken into consideration
             * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
             * pod labels will be ignored. The default value is empty.
             * The same key is forbidden to exist in both mismatchLabelKeys and labelSelector.
             * Also, mismatchLabelKeys cannot be set when labelSelector isn't set. */
            mismatchLabelKeys?: string[];
            /** A label query over the set of namespaces that the term applies to.
             * The term is applied to the union of the namespaces selected by this field
             * and the ones listed in the namespaces field.
             * null selector and null or empty namespaces list means "this pod's namespace".
             * An empty selector ({}) matches all namespaces. */
            namespaceSelector?: {
              /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
              matchExpressions?: {
                /** key is the label key that the selector applies to. */
                key: string;
                /** operator represents a key's relationship to a set of values.
                 * Valid operators are In, NotIn, Exists and DoesNotExist. */
                operator: string;
                /** values is an array of string values. If the operator is In or NotIn,
                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                 * the values array must be empty. This array is replaced during a strategic
                 * merge patch. */
                values?: string[];
              }[];
              /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
               * map is equivalent to an element of matchExpressions, whose key field is "key", the
               * operator is "In", and the values array contains only "value". The requirements are ANDed. */
              matchLabels?: Record<string, string>;
            };
            /** namespaces specifies a static list of namespace names that the term applies to.
             * The term is applied to the union of the namespaces listed in this field
             * and the ones selected by namespaceSelector.
             * null or empty namespaces list and null namespaceSelector means "this pod's namespace". */
            namespaces?: string[];
            /** This pod should be co-located (affinity) or not co-located (anti-affinity) with the pods matching
             * the labelSelector in the specified namespaces, where co-located is defined as running on a node
             * whose value of the label with key topologyKey matches that of any node on which any of the
             * selected pods is running.
             * Empty topologyKey is not allowed. */
            topologyKey: string;
          };
          /** weight associated with matching the corresponding podAffinityTerm,
           * in the range 1-100. */
          weight: number;
        }[];
        /** If the anti-affinity requirements specified by this field are not met at
         * scheduling time, the pod will not be scheduled onto the node.
         * If the anti-affinity requirements specified by this field cease to be met
         * at some point during pod execution (e.g. due to a pod label update), the
         * system may or may not try to eventually evict the pod from its node.
         * When there are multiple elements, the lists of nodes corresponding to each
         * podAffinityTerm are intersected, i.e. all terms must be satisfied. */
        requiredDuringSchedulingIgnoredDuringExecution?: {
          /** A label query over a set of resources, in this case pods.
           * If it's null, this PodAffinityTerm matches with no Pods. */
          labelSelector?: {
            /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
            matchExpressions?: {
              /** key is the label key that the selector applies to. */
              key: string;
              /** operator represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists and DoesNotExist. */
              operator: string;
              /** values is an array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. This array is replaced during a strategic
               * merge patch. */
              values?: string[];
            }[];
            /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
             * map is equivalent to an element of matchExpressions, whose key field is "key", the
             * operator is "In", and the values array contains only "value". The requirements are ANDed. */
            matchLabels?: Record<string, string>;
          };
          /** MatchLabelKeys is a set of pod label keys to select which pods will
           * be taken into consideration. The keys are used to lookup values from the
           * incoming pod labels, those key-value labels are merged with `labelSelector` as `key in (value)`
           * to select the group of existing pods which pods will be taken into consideration
           * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
           * pod labels will be ignored. The default value is empty.
           * The same key is forbidden to exist in both matchLabelKeys and labelSelector.
           * Also, matchLabelKeys cannot be set when labelSelector isn't set. */
          matchLabelKeys?: string[];
          /** MismatchLabelKeys is a set of pod label keys to select which pods will
           * be taken into consideration. The keys are used to lookup values from the
           * incoming pod labels, those key-value labels are merged with `labelSelector` as `key notin (value)`
           * to select the group of existing pods which pods will be taken into consideration
           * for the incoming pod's pod (anti) affinity. Keys that don't exist in the incoming
           * pod labels will be ignored. The default value is empty.
           * The same key is forbidden to exist in both mismatchLabelKeys and labelSelector.
           * Also, mismatchLabelKeys cannot be set when labelSelector isn't set. */
          mismatchLabelKeys?: string[];
          /** A label query over the set of namespaces that the term applies to.
           * The term is applied to the union of the namespaces selected by this field
           * and the ones listed in the namespaces field.
           * null selector and null or empty namespaces list means "this pod's namespace".
           * An empty selector ({}) matches all namespaces. */
          namespaceSelector?: {
            /** matchExpressions is a list of label selector requirements. The requirements are ANDed. */
            matchExpressions?: {
              /** key is the label key that the selector applies to. */
              key: string;
              /** operator represents a key's relationship to a set of values.
               * Valid operators are In, NotIn, Exists and DoesNotExist. */
              operator: string;
              /** values is an array of string values. If the operator is In or NotIn,
               * the values array must be non-empty. If the operator is Exists or DoesNotExist,
               * the values array must be empty. This array is replaced during a strategic
               * merge patch. */
              values?: string[];
            }[];
            /** matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
             * map is equivalent to an element of matchExpressions, whose key field is "key", the
             * operator is "In", and the values array contains only "value". The requirements are ANDed. */
            matchLabels?: Record<string, string>;
          };
          /** namespaces specifies a static list of namespace names that the term applies to.
           * The term is applied to the union of the namespaces listed in this field
           * and the ones selected by namespaceSelector.
           * null or empty namespaces list and null namespaceSelector means "this pod's namespace". */
          namespaces?: string[];
          /** This pod should be co-located (affinity) or not co-located (anti-affinity) with the pods matching
           * the labelSelector in the specified namespaces, where co-located is defined as running on a node
           * whose value of the label with key topologyKey matches that of any node on which any of the
           * selected pods is running.
           * Empty topologyKey is not allowed. */
          topologyKey: string;
        }[];
      };
    };
    /** autoscaling configures horizontal pod autoscaling for the agent. */
    autoscaling?: {
      /** enabled specifies whether autoscaling is enabled.
       * When enabled, the autoscaler will manage replica count instead of spec.runtime.replicas. */
      enabled?: boolean;
      /** keda contains KEDA-specific configuration. Only used when type is "keda". */
      keda?: {
        /** cooldownPeriod is the wait period in seconds after last trigger before scaling down. Defaults to 300. */
        cooldownPeriod?: number;
        /** pollingInterval is the interval in seconds to check triggers. Defaults to 30. */
        pollingInterval?: number;
        /** triggers is the list of KEDA triggers for scaling.
         * If empty, a default Prometheus trigger for connections is configured. */
        triggers?: {
          /** metadata contains trigger-specific configuration.
           * For prometheus: serverAddress, query, threshold
           * For cron: timezone, start, end, desiredReplicas */
          metadata: Record<string, string>;
          /** type is the KEDA trigger type (e.g., "prometheus", "cron"). */
          type: string;
        }[];
      };
      /** maxReplicas is the maximum number of replicas. */
      maxReplicas?: number;
      /** minReplicas is the minimum number of replicas.
       * For KEDA, set to 0 to enable scale-to-zero. */
      minReplicas?: number;
      /** scaleDownStabilizationSeconds is the number of seconds to wait before
       * scaling down after a scale-up. This prevents thrashing when connections
       * are bursty. Defaults to 300 (5 minutes). Only used for HPA type. */
      scaleDownStabilizationSeconds?: number;
      /** targetCPUUtilizationPercentage is the target average CPU utilization.
       * CPU is a secondary metric since agents are typically I/O bound.
       * Set to nil to disable CPU-based scaling. Defaults to 90% as a safety valve.
       * Only used for HPA type. */
      targetCPUUtilizationPercentage?: number;
      /** targetMemoryUtilizationPercentage is the target average memory utilization.
       * Memory is the primary scaling metric since each WebSocket connection and
       * session consumes memory. Defaults to 70%. Only used for HPA type. */
      targetMemoryUtilizationPercentage?: number;
      /** type specifies which autoscaler to use. Defaults to "hpa".
       * Use "keda" for advanced scaling (scale to zero, Prometheus metrics, cron). */
      type?: "hpa" | "keda";
    };
    /** nodeSelector is a map of node labels for pod scheduling. */
    nodeSelector?: Record<string, string>;
    /** replicas is the desired number of agent runtime pods.
     * This field is ignored when autoscaling is enabled. */
    replicas?: number;
    /** resources defines compute resource requirements for the agent container. */
    resources?: {
      /** Claims lists the names of resources, defined in spec.resourceClaims,
       * that are used by this container.
       * 
       * This field depends on the
       * DynamicResourceAllocation feature gate.
       * 
       * This field is immutable. It can only be set for containers. */
      claims?: {
        /** Name must match the name of one entry in pod.spec.resourceClaims of
         * the Pod where this field is used. It makes that resource available
         * inside a container. */
        name: string;
        /** Request is the name chosen for a request in the referenced claim.
         * If empty, everything from the claim is made available, otherwise
         * only the result of this request. */
        request?: string;
      }[];
      /** Limits describes the maximum amount of compute resources allowed.
       * More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ */
      limits?: Record<string, unknown>;
      /** Requests describes the minimum amount of compute resources required.
       * If Requests is omitted for a container, it defaults to Limits if that is explicitly specified,
       * otherwise to an implementation-defined value. Requests cannot exceed Limits.
       * More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ */
      requests?: Record<string, unknown>;
    };
    /** tolerations are tolerations for pod scheduling. */
    tolerations?: {
      /** Effect indicates the taint effect to match. Empty means match all taint effects.
       * When specified, allowed values are NoSchedule, PreferNoSchedule and NoExecute. */
      effect?: string;
      /** Key is the taint key that the toleration applies to. Empty means match all taint keys.
       * If the key is empty, operator must be Exists; this combination means to match all values and all keys. */
      key?: string;
      /** Operator represents a key's relationship to the value.
       * Valid operators are Exists, Equal, Lt, and Gt. Defaults to Equal.
       * Exists is equivalent to wildcard for value, so that a pod can
       * tolerate all taints of a particular category.
       * Lt and Gt perform numeric comparisons (requires feature gate TaintTolerationComparisonOperators). */
      operator?: string;
      /** TolerationSeconds represents the period of time the toleration (which must be
       * of effect NoExecute, otherwise this field is ignored) tolerates the taint. By default,
       * it is not set, which means tolerate the taint forever (do not evict). Zero and
       * negative values will be treated as 0 (evict immediately) by the system. */
      tolerationSeconds?: number;
      /** Value is the taint value the toleration matches to.
       * If the operator is Exists, the value should be empty, otherwise just a regular string. */
      value?: string;
    }[];
  };
  /** session configures session management and storage. */
  session?: {
    /** storeRef references a secret containing connection details for the session store.
     * Required for redis and postgres store types. */
    storeRef?: {
      /** Name of the referent.
       * This field is effectively required, but due to backwards compatibility is
       * allowed to be empty. Instances of this type with an empty value here are
       * almost certainly wrong.
       * More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names */
      name?: string;
    };
    /** ttl is the time-to-live for sessions in duration format (e.g., "24h", "30m"). */
    ttl?: string;
    /** type specifies the session store backend. */
    type: "memory" | "redis" | "postgres";
  };
  /** toolRegistryRef optionally references a ToolRegistry for available tools. */
  toolRegistryRef?: {
    /** name is the name of the ToolRegistry resource. */
    name: string;
    /** namespace is the namespace of the ToolRegistry resource.
     * If not specified, the same namespace as the AgentRuntime is used. */
    namespace?: string;
  };
}

export interface AgentRuntimeStatus {
  /** activeVersion is the currently deployed PromptPack version. */
  activeVersion?: string;
  /** conditions represent the current state of the AgentRuntime resource. */
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
  /** phase represents the current lifecycle phase of the AgentRuntime. */
  phase?: "Pending" | "Running" | "Failed";
  /** replicas tracks the replica counts for the deployment. */
  replicas?: {
    /** available is the number of available replicas. */
    available: number;
    /** desired is the desired number of replicas. */
    desired: number;
    /** ready is the number of ready replicas. */
    ready: number;
  };
  /** serviceEndpoint is the internal Kubernetes service endpoint for the agent facade.
   * Format: {name}.{namespace}.svc.cluster.local:{port}
   * This can be used by dashboard or other services to connect to the agent. */
  serviceEndpoint?: string;
}

export interface AgentRuntime {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "AgentRuntime";
  metadata: ObjectMeta;
  spec: AgentRuntimeSpec;
  status?: AgentRuntimeStatus;
}
