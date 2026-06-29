/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import corev1 "k8s.io/api/core/v1"

// PodOverrides carries per-workload scheduling, identity, and injection
// overrides applied to every operator-generated Pod. All fields optional.
//
// Pod-level fields (serviceAccountName, labels, annotations, nodeSelector,
// tolerations, priorityClassName, imagePullSecrets, extraVolumes) are merged
// into the PodSpec.
//
// Container-level fields (extraEnv, extraEnvFrom, extraVolumeMounts) are
// appended to every non-operator-injected container in the pod.
//
// Existing per-CRD hooks (e.g. FacadeConfig.ExtraEnv, RuntimeConfig.NodeSelector)
// take precedence; PodOverrides values are merged/concatenated after them.
type PodOverrides struct {
	// serviceAccountName overrides the default service account.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// labels are merged into pod labels. Operator-set labels take precedence
	// on key collision (they are load-bearing for Service selectors).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are merged into pod annotations. User values override
	// operator-set defaults on key collision.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// nodeSelector keys are merged with operator-set keys; user values win on collision.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations are appended to the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// priorityClassName overrides the operator-default priority class when set.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// imagePullSecrets are appended to the pod's imagePullSecrets.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// extraEnv is appended to every non-operator-injected container's env.
	// +optional
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`

	// extraEnvFrom is appended to every non-operator-injected container's envFrom.
	// +optional
	ExtraEnvFrom []corev1.EnvFromSource `json:"extraEnvFrom,omitempty"`

	// extraVolumes are appended to the PodSpec.Volumes. Schemaless to avoid
	// inlining the full corev1.Volume union (~30 volume types) into the CRD;
	// validated by the apiserver at Deployment apply.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// extraVolumeMounts are appended to every non-operator-injected container's
	// volumeMounts. Schemaless (see extraVolumes).
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
}

// RedisConfig points a consumer at a Redis instance. Mirrors the chart's
// per-consumer override block shape (redis.default and the consumer
// blocks under workspaceServices.memoryApi.cache.redis et al). Three
// input forms with resolution order: existingSecret > url > host.
//
// existingSecret (preferred for production) reads the full Redis URL
// from a Kubernetes Secret. The Secret's value must be a complete
// connection string (`redis(s)://[user:pass@]host:port[/db]`).
//
// url is a literal connection string in the CRD spec. Acceptable when
// the password is non-secret or already managed by external-secrets.
//
// host is the decomposed convenience form: cleartext only, no auth.
// For auth'd Redis use existingSecret instead.
//
// serviceRef is the in-cluster Service form: the operator builds
// redis://<name>.<namespace>:<port>. Unauthenticated, like host.
//
// All four forms (existingSecret, url, host, serviceRef) are mutually
// exclusive on a single block; CEL validation enforces this at admission.
type RedisConfig struct {
	// serviceRef points at an in-cluster Redis Service by name. The
	// operator synthesises redis://<name>.<namespace>:<port> from it.
	// This is the K8s-idiomatic, in-cluster, UNAUTHENTICATED form — the
	// sibling of host. For any auth'd / TLS / cloud Redis use
	// existingSecret (a full rediss://user:pass@host URL) instead.
	// +optional
	ServiceRef *RedisServiceRef `json:"serviceRef,omitempty"`

	// existingSecret references a Kubernetes Secret containing the
	// full Redis URL. The Secret must hold a single key with a
	// complete `redis(s)://...` connection string.
	// +optional
	ExistingSecret *RedisSecretRef `json:"existingSecret,omitempty"`

	// url is a literal Redis connection string. Use only when the URL
	// either contains no password or carries a password injected via
	// external-secrets / KV-CSI rather than typed into chart values.
	// +optional
	// +kubebuilder:validation:Pattern=`^rediss?://`
	URL string `json:"url,omitempty"`

	// host is the Redis hostname for the decomposed form. Combined
	// with port, db, and user to synthesise an unauthenticated URL.
	// For any auth'd Redis, use existingSecret.
	// +optional
	Host string `json:"host,omitempty"`

	// port is the Redis TCP port. Defaults to 6379 when host is set.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// db is the Redis database index. Defaults to 0.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=15
	DB int32 `json:"db,omitempty"`

	// user is the Redis ACL username. Empty defaults to "default"
	// (the built-in Redis 6+ user).
	// +optional
	User string `json:"user,omitempty"`
}

// RedisSecretRef references a Kubernetes Secret containing a Redis
// connection URL. Both fields required when the parent block uses the
// existingSecret form.
type RedisSecretRef struct {
	// name of the Secret in the same namespace as the workspace's
	// service Deployment.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key within the Secret whose value is the Redis URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// RedisServiceRef references an in-cluster Redis Service by name. The
// operator builds redis://<name>.<namespace>:<port> from it — no Secret
// and no auth. Use existingSecret for any authenticated or TLS Redis.
type RedisServiceRef struct {
	// name of the Redis Service.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace of the Redis Service. Defaults to the workspace
	// namespace when empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// port of the Redis Service. Defaults to 6379 when zero.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
}
