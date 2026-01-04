// Kubernetes common types

export interface ObjectMeta {
  name: string;
  namespace?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  creationTimestamp?: string;
  uid?: string;
}

export interface Condition {
  type: string;
  status: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface LocalObjectReference {
  name: string;
}

export interface SecretKeyRef {
  name: string;
  key?: string;
}
