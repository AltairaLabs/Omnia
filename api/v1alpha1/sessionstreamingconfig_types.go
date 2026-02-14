/*
Copyright 2025.

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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StreamingProvider defines the supported streaming provider types.
// +kubebuilder:validation:Enum=kafka;kinesis;pulsar;nats
type StreamingProvider string

const (
	// StreamingProviderKafka uses Apache Kafka for event streaming.
	StreamingProviderKafka StreamingProvider = "kafka"
	// StreamingProviderKinesis uses AWS Kinesis for event streaming.
	StreamingProviderKinesis StreamingProvider = "kinesis"
	// StreamingProviderPulsar uses Apache Pulsar for event streaming.
	StreamingProviderPulsar StreamingProvider = "pulsar"
	// StreamingProviderNATS uses NATS JetStream for event streaming.
	StreamingProviderNATS StreamingProvider = "nats"
)

// KafkaCompression defines the supported Kafka compression types.
// +kubebuilder:validation:Enum=none;gzip;snappy;lz4;zstd
type KafkaCompression string

const (
	// KafkaCompressionNone disables compression.
	KafkaCompressionNone KafkaCompression = "none"
	// KafkaCompressionGzip uses gzip compression.
	KafkaCompressionGzip KafkaCompression = "gzip"
	// KafkaCompressionSnappy uses snappy compression.
	KafkaCompressionSnappy KafkaCompression = "snappy"
	// KafkaCompressionLZ4 uses LZ4 compression.
	KafkaCompressionLZ4 KafkaCompression = "lz4"
	// KafkaCompressionZstd uses Zstandard compression.
	KafkaCompressionZstd KafkaCompression = "zstd"
)

// KafkaAcks defines the Kafka acknowledgment level.
// +kubebuilder:validation:Enum=none;leader;all
type KafkaAcks string

const (
	// KafkaAcksNone does not wait for acknowledgment.
	KafkaAcksNone KafkaAcks = "none"
	// KafkaAcksLeader waits for the leader to acknowledge.
	KafkaAcksLeader KafkaAcks = "leader"
	// KafkaAcksAll waits for all in-sync replicas to acknowledge.
	KafkaAcksAll KafkaAcks = "all"
)

// PulsarAuthType defines the supported Pulsar authentication types.
// +kubebuilder:validation:Enum=token;oauth2
type PulsarAuthType string

const (
	// PulsarAuthTypeToken uses JWT token authentication.
	PulsarAuthTypeToken PulsarAuthType = "token"
	// PulsarAuthTypeOAuth2 uses OAuth2 authentication.
	PulsarAuthTypeOAuth2 PulsarAuthType = "oauth2"
)

// SessionStreamingConfigPhase represents the current phase of the streaming config.
// +kubebuilder:validation:Enum=Active;Error
type SessionStreamingConfigPhase string

const (
	// SessionStreamingConfigPhaseActive indicates the config is valid and active.
	SessionStreamingConfigPhaseActive SessionStreamingConfigPhase = "Active"
	// SessionStreamingConfigPhaseError indicates the config has an error.
	SessionStreamingConfigPhaseError SessionStreamingConfigPhase = "Error"
)

// TransformFormat defines the supported output formats for event transformation.
// +kubebuilder:validation:Enum=json;avro;protobuf
type TransformFormat string

const (
	// TransformFormatJSON outputs events as JSON.
	TransformFormatJSON TransformFormat = "json"
	// TransformFormatAvro outputs events as Avro.
	TransformFormatAvro TransformFormat = "avro"
	// TransformFormatProtobuf outputs events as Protobuf.
	TransformFormatProtobuf TransformFormat = "protobuf"
)

// KafkaAuthConfig defines authentication configuration for Kafka.
type KafkaAuthConfig struct {
	// mechanism is the SASL mechanism to use.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Mechanism string `json:"mechanism"`

	// secretRef references a Secret containing Kafka credentials.
	// +kubebuilder:validation:Required
	SecretRef LocalObjectReference `json:"secretRef"`
}

// KafkaConfig defines Kafka-specific streaming configuration.
type KafkaConfig struct {
	// brokers is the list of Kafka broker addresses.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Brokers []string `json:"brokers"`

	// topic is the Kafka topic to publish events to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Topic string `json:"topic"`

	// partitionKey is the field used for partition assignment.
	// +kubebuilder:default="session_id"
	// +optional
	PartitionKey string `json:"partitionKey,omitempty"`

	// compression is the compression algorithm for published messages.
	// +kubebuilder:default=snappy
	// +optional
	Compression KafkaCompression `json:"compression,omitempty"`

	// acks is the acknowledgment level for published messages.
	// +kubebuilder:default=all
	// +optional
	Acks KafkaAcks `json:"acks,omitempty"`

	// retries is the number of retries for failed publish attempts.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=3
	// +optional
	Retries int32 `json:"retries,omitempty"`

	// auth defines authentication configuration for Kafka.
	// +optional
	Auth *KafkaAuthConfig `json:"auth,omitempty"`
}

// KinesisConfig defines AWS Kinesis-specific streaming configuration.
type KinesisConfig struct {
	// streamName is the Kinesis stream name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	StreamName string `json:"streamName"`

	// region is the AWS region where the Kinesis stream resides.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// partitionKey is the field used for partition assignment.
	// +kubebuilder:default="session_id"
	// +optional
	PartitionKey string `json:"partitionKey,omitempty"`

	// secretRef references a Secret containing AWS credentials.
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty"`
}

// PulsarAuthConfig defines authentication configuration for Pulsar.
type PulsarAuthConfig struct {
	// type is the Pulsar authentication type.
	// +kubebuilder:validation:Required
	Type PulsarAuthType `json:"type"`

	// secretRef references a Secret containing Pulsar credentials.
	// +kubebuilder:validation:Required
	SecretRef LocalObjectReference `json:"secretRef"`
}

// PulsarConfig defines Apache Pulsar-specific streaming configuration.
type PulsarConfig struct {
	// serviceUrl is the Pulsar service URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceUrl string `json:"serviceUrl"`

	// topic is the Pulsar topic to publish events to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Topic string `json:"topic"`

	// auth defines authentication configuration for Pulsar.
	// +optional
	Auth *PulsarAuthConfig `json:"auth,omitempty"`
}

// NATSAuthConfig defines authentication configuration for NATS.
type NATSAuthConfig struct {
	// secretRef references a Secret containing NATS credentials.
	// +kubebuilder:validation:Required
	SecretRef LocalObjectReference `json:"secretRef"`
}

// NATSConfig defines NATS-specific streaming configuration.
type NATSConfig struct {
	// url is the NATS server URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// stream is the NATS JetStream stream name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Stream string `json:"stream"`

	// subject is the NATS subject to publish events to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Subject string `json:"subject"`

	// auth defines authentication configuration for NATS.
	// +optional
	Auth *NATSAuthConfig `json:"auth,omitempty"`
}

// StreamingFilterConfig defines filtering rules for which events to stream.
type StreamingFilterConfig struct {
	// eventTypes is the list of event types to stream. If empty, all event types are streamed.
	// +optional
	EventTypes []string `json:"eventTypes,omitempty"`

	// workspaces is the list of workspace names to stream events for. If empty, all workspaces are included.
	// +optional
	Workspaces []string `json:"workspaces,omitempty"`

	// agents is the list of agent names to stream events for. If empty, all agents are included.
	// +optional
	Agents []string `json:"agents,omitempty"`
}

// StreamingTransformConfig defines how events are transformed before publishing.
type StreamingTransformConfig struct {
	// format is the output format for streamed events.
	// +kubebuilder:default=json
	// +optional
	Format TransformFormat `json:"format,omitempty"`

	// includeFields specifies which fields to include in the output. If empty, all fields are included.
	// +optional
	IncludeFields []string `json:"includeFields,omitempty"`

	// excludePII specifies whether to strip personally identifiable information from events.
	// +kubebuilder:default=false
	// +optional
	ExcludePII bool `json:"excludePII,omitempty"`
}

// SessionStreamingConfigSpec defines the desired state of SessionStreamingConfig.
// +kubebuilder:validation:XValidation:rule="!self.enabled || (self.provider == 'kafka' && has(self.kafka)) || (self.provider == 'kinesis' && has(self.kinesis)) || (self.provider == 'pulsar' && has(self.pulsar)) || (self.provider == 'nats' && has(self.nats))",message="provider-specific configuration must be set when streaming is enabled"
type SessionStreamingConfigSpec struct {
	// enabled specifies whether event streaming is active.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// provider is the streaming provider to use.
	// +kubebuilder:validation:Required
	Provider StreamingProvider `json:"provider"`

	// kafka defines Kafka-specific configuration. Required when provider is "kafka".
	// +optional
	Kafka *KafkaConfig `json:"kafka,omitempty"`

	// kinesis defines AWS Kinesis-specific configuration. Required when provider is "kinesis".
	// +optional
	Kinesis *KinesisConfig `json:"kinesis,omitempty"`

	// pulsar defines Apache Pulsar-specific configuration. Required when provider is "pulsar".
	// +optional
	Pulsar *PulsarConfig `json:"pulsar,omitempty"`

	// nats defines NATS-specific configuration. Required when provider is "nats".
	// +optional
	NATS *NATSConfig `json:"nats,omitempty"`

	// filter defines filtering rules for which events to stream.
	// +optional
	Filter *StreamingFilterConfig `json:"filter,omitempty"`

	// transform defines how events are transformed before publishing.
	// +optional
	Transform *StreamingTransformConfig `json:"transform,omitempty"`
}

// StreamingErrorDetail captures a single streaming error.
type StreamingErrorDetail struct {
	// timestamp is when the error occurred.
	// +kubebuilder:validation:Required
	Timestamp metav1.Time `json:"timestamp"`

	// message is the error message.
	// +kubebuilder:validation:Required
	Message string `json:"message"`
}

// SessionStreamingConfigStatus defines the observed state of SessionStreamingConfig.
type SessionStreamingConfigStatus struct {
	// phase represents the current lifecycle phase of the config.
	// +optional
	Phase SessionStreamingConfigPhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// connected indicates whether the streaming provider is currently connected.
	// +optional
	Connected bool `json:"connected,omitempty"`

	// lastEventAt is the timestamp of the last successfully published event.
	// +optional
	LastEventAt *metav1.Time `json:"lastEventAt,omitempty"`

	// eventsPublished is the total number of events published since the config was created.
	// +optional
	EventsPublished int64 `json:"eventsPublished,omitempty"`

	// errors is a list of recent streaming errors.
	// +optional
	Errors []StreamingErrorDetail `json:"errors,omitempty"`

	// conditions represent the current state of the SessionStreamingConfig resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Connected",type=boolean,JSONPath=`.status.connected`
// +kubebuilder:printcolumn:name="Events",type=integer,JSONPath=`.status.eventsPublished`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SessionStreamingConfig is the Schema for the sessionstreamingconfigs API.
// It defines configuration for real-time event streaming of session data to external systems.
type SessionStreamingConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SessionStreamingConfig
	// +required
	Spec SessionStreamingConfigSpec `json:"spec"`

	// status defines the observed state of SessionStreamingConfig
	// +optional
	Status SessionStreamingConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SessionStreamingConfigList contains a list of SessionStreamingConfig.
type SessionStreamingConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SessionStreamingConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SessionStreamingConfig{}, &SessionStreamingConfigList{})
}
