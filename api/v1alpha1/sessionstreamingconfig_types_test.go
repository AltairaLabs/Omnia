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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStreamingProviderConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant StreamingProvider
		expected string
	}{
		{
			name:     "Kafka provider",
			constant: StreamingProviderKafka,
			expected: "kafka",
		},
		{
			name:     "Kinesis provider",
			constant: StreamingProviderKinesis,
			expected: "kinesis",
		},
		{
			name:     "Pulsar provider",
			constant: StreamingProviderPulsar,
			expected: "pulsar",
		},
		{
			name:     "NATS provider",
			constant: StreamingProviderNATS,
			expected: "nats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("StreamingProvider constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestKafkaCompressionConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant KafkaCompression
		expected string
	}{
		{name: "None", constant: KafkaCompressionNone, expected: "none"},
		{name: "Gzip", constant: KafkaCompressionGzip, expected: "gzip"},
		{name: "Snappy", constant: KafkaCompressionSnappy, expected: "snappy"},
		{name: "LZ4", constant: KafkaCompressionLZ4, expected: "lz4"},
		{name: "Zstd", constant: KafkaCompressionZstd, expected: "zstd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("KafkaCompression constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestKafkaAcksConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant KafkaAcks
		expected string
	}{
		{name: "None", constant: KafkaAcksNone, expected: "none"},
		{name: "Leader", constant: KafkaAcksLeader, expected: "leader"},
		{name: "All", constant: KafkaAcksAll, expected: "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("KafkaAcks constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestPulsarAuthTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant PulsarAuthType
		expected string
	}{
		{name: "Token", constant: PulsarAuthTypeToken, expected: "token"},
		{name: "OAuth2", constant: PulsarAuthTypeOAuth2, expected: "oauth2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("PulsarAuthType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestTransformFormatConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant TransformFormat
		expected string
	}{
		{name: "JSON", constant: TransformFormatJSON, expected: "json"},
		{name: "Avro", constant: TransformFormatAvro, expected: "avro"},
		{name: "Protobuf", constant: TransformFormatProtobuf, expected: "protobuf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("TransformFormat constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionStreamingConfigPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SessionStreamingConfigPhase
		expected string
	}{
		{
			name:     "Active phase",
			constant: SessionStreamingConfigPhaseActive,
			expected: "Active",
		},
		{
			name:     "Error phase",
			constant: SessionStreamingConfigPhaseError,
			expected: "Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SessionStreamingConfigPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionStreamingConfigWithKafka(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kafka-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderKafka,
			Kafka: &KafkaConfig{
				Brokers:      []string{"kafka-0.kafka:9092", "kafka-1.kafka:9092"},
				Topic:        "omnia-sessions",
				PartitionKey: "session_id",
				Compression:  KafkaCompressionSnappy,
				Acks:         KafkaAcksAll,
				Retries:      3,
				Auth: &KafkaAuthConfig{
					Mechanism: "SASL_PLAINTEXT",
					SecretRef: LocalObjectReference{Name: "kafka-credentials"},
				},
			},
		},
	}

	if config.Name != "kafka-streaming" {
		t.Errorf("Name = %v, want kafka-streaming", config.Name)
	}

	if !config.Spec.Enabled {
		t.Error("Spec.Enabled should be true")
	}

	if config.Spec.Provider != StreamingProviderKafka {
		t.Errorf("Spec.Provider = %v, want kafka", config.Spec.Provider)
	}

	if config.Spec.Kafka == nil {
		t.Fatal("Spec.Kafka should not be nil")
	}

	if len(config.Spec.Kafka.Brokers) != 2 {
		t.Errorf("Kafka.Brokers length = %d, want 2", len(config.Spec.Kafka.Brokers))
	}

	if config.Spec.Kafka.Topic != "omnia-sessions" {
		t.Errorf("Kafka.Topic = %v, want omnia-sessions", config.Spec.Kafka.Topic)
	}

	if config.Spec.Kafka.Compression != KafkaCompressionSnappy {
		t.Errorf("Kafka.Compression = %v, want snappy", config.Spec.Kafka.Compression)
	}

	if config.Spec.Kafka.Acks != KafkaAcksAll {
		t.Errorf("Kafka.Acks = %v, want all", config.Spec.Kafka.Acks)
	}

	if config.Spec.Kafka.Retries != 3 {
		t.Errorf("Kafka.Retries = %v, want 3", config.Spec.Kafka.Retries)
	}

	if config.Spec.Kafka.Auth == nil {
		t.Fatal("Kafka.Auth should not be nil")
	}

	if config.Spec.Kafka.Auth.Mechanism != "SASL_PLAINTEXT" {
		t.Errorf("Kafka.Auth.Mechanism = %v, want SASL_PLAINTEXT", config.Spec.Kafka.Auth.Mechanism)
	}

	if config.Spec.Kafka.Auth.SecretRef.Name != "kafka-credentials" {
		t.Errorf("Kafka.Auth.SecretRef.Name = %v, want kafka-credentials", config.Spec.Kafka.Auth.SecretRef.Name)
	}
}

func TestSessionStreamingConfigWithKinesis(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kinesis-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderKinesis,
			Kinesis: &KinesisConfig{
				StreamName:   "omnia-sessions",
				Region:       "us-east-1",
				PartitionKey: "session_id",
				SecretRef:    &LocalObjectReference{Name: "aws-credentials"},
			},
		},
	}

	if config.Spec.Provider != StreamingProviderKinesis {
		t.Errorf("Spec.Provider = %v, want kinesis", config.Spec.Provider)
	}

	if config.Spec.Kinesis == nil {
		t.Fatal("Spec.Kinesis should not be nil")
	}

	if config.Spec.Kinesis.StreamName != "omnia-sessions" {
		t.Errorf("Kinesis.StreamName = %v, want omnia-sessions", config.Spec.Kinesis.StreamName)
	}

	if config.Spec.Kinesis.Region != "us-east-1" {
		t.Errorf("Kinesis.Region = %v, want us-east-1", config.Spec.Kinesis.Region)
	}

	if config.Spec.Kinesis.SecretRef.Name != "aws-credentials" {
		t.Errorf("Kinesis.SecretRef.Name = %v, want aws-credentials", config.Spec.Kinesis.SecretRef.Name)
	}
}

func TestSessionStreamingConfigWithPulsar(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pulsar-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderPulsar,
			Pulsar: &PulsarConfig{
				ServiceUrl: "pulsar://pulsar:6650",
				Topic:      "persistent://public/default/omnia-sessions",
				Auth: &PulsarAuthConfig{
					Type:      PulsarAuthTypeToken,
					SecretRef: LocalObjectReference{Name: "pulsar-token"},
				},
			},
		},
	}

	if config.Spec.Provider != StreamingProviderPulsar {
		t.Errorf("Spec.Provider = %v, want pulsar", config.Spec.Provider)
	}

	if config.Spec.Pulsar == nil {
		t.Fatal("Spec.Pulsar should not be nil")
	}

	if config.Spec.Pulsar.ServiceUrl != "pulsar://pulsar:6650" {
		t.Errorf("Pulsar.ServiceUrl = %v, want pulsar://pulsar:6650", config.Spec.Pulsar.ServiceUrl)
	}

	if config.Spec.Pulsar.Topic != "persistent://public/default/omnia-sessions" {
		t.Errorf("Pulsar.Topic = %v, want persistent://public/default/omnia-sessions", config.Spec.Pulsar.Topic)
	}

	if config.Spec.Pulsar.Auth == nil {
		t.Fatal("Pulsar.Auth should not be nil")
	}

	if config.Spec.Pulsar.Auth.Type != PulsarAuthTypeToken {
		t.Errorf("Pulsar.Auth.Type = %v, want token", config.Spec.Pulsar.Auth.Type)
	}

	if config.Spec.Pulsar.Auth.SecretRef.Name != "pulsar-token" {
		t.Errorf("Pulsar.Auth.SecretRef.Name = %v, want pulsar-token", config.Spec.Pulsar.Auth.SecretRef.Name)
	}
}

func TestSessionStreamingConfigWithNATS(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nats-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderNATS,
			NATS: &NATSConfig{
				URL:     "nats://nats:4222",
				Stream:  "omnia-sessions",
				Subject: "sessions.>",
				Auth: &NATSAuthConfig{
					SecretRef: LocalObjectReference{Name: "nats-credentials"},
				},
			},
		},
	}

	if config.Spec.Provider != StreamingProviderNATS {
		t.Errorf("Spec.Provider = %v, want nats", config.Spec.Provider)
	}

	if config.Spec.NATS == nil {
		t.Fatal("Spec.NATS should not be nil")
	}

	if config.Spec.NATS.URL != "nats://nats:4222" {
		t.Errorf("NATS.URL = %v, want nats://nats:4222", config.Spec.NATS.URL)
	}

	if config.Spec.NATS.Stream != "omnia-sessions" {
		t.Errorf("NATS.Stream = %v, want omnia-sessions", config.Spec.NATS.Stream)
	}

	if config.Spec.NATS.Subject != "sessions.>" {
		t.Errorf("NATS.Subject = %v, want sessions.>", config.Spec.NATS.Subject)
	}

	if config.Spec.NATS.Auth == nil {
		t.Fatal("NATS.Auth should not be nil")
	}

	if config.Spec.NATS.Auth.SecretRef.Name != "nats-credentials" {
		t.Errorf("NATS.Auth.SecretRef.Name = %v, want nats-credentials", config.Spec.NATS.Auth.SecretRef.Name)
	}
}

func TestSessionStreamingConfigWithFilter(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "filtered-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderKafka,
			Kafka: &KafkaConfig{
				Brokers: []string{"kafka:9092"},
				Topic:   "omnia-sessions",
			},
			Filter: &StreamingFilterConfig{
				EventTypes: []string{"message_added", "session_completed", "tool_executed"},
				Workspaces: []string{"production"},
				Agents:     []string{"agent-1"},
			},
		},
	}

	if config.Spec.Filter == nil {
		t.Fatal("Spec.Filter should not be nil")
	}

	if len(config.Spec.Filter.EventTypes) != 3 {
		t.Errorf("Filter.EventTypes length = %d, want 3", len(config.Spec.Filter.EventTypes))
	}

	if config.Spec.Filter.EventTypes[0] != "message_added" {
		t.Errorf("Filter.EventTypes[0] = %v, want message_added", config.Spec.Filter.EventTypes[0])
	}

	if len(config.Spec.Filter.Workspaces) != 1 {
		t.Errorf("Filter.Workspaces length = %d, want 1", len(config.Spec.Filter.Workspaces))
	}

	if len(config.Spec.Filter.Agents) != 1 {
		t.Errorf("Filter.Agents length = %d, want 1", len(config.Spec.Filter.Agents))
	}
}

func TestSessionStreamingConfigWithTransform(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "transformed-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderKafka,
			Kafka: &KafkaConfig{
				Brokers: []string{"kafka:9092"},
				Topic:   "omnia-sessions",
			},
			Transform: &StreamingTransformConfig{
				Format:        TransformFormatJSON,
				IncludeFields: []string{"session_id", "timestamp", "event_type", "content", "metadata"},
				ExcludePII:    true,
			},
		},
	}

	if config.Spec.Transform == nil {
		t.Fatal("Spec.Transform should not be nil")
	}

	if config.Spec.Transform.Format != TransformFormatJSON {
		t.Errorf("Transform.Format = %v, want json", config.Spec.Transform.Format)
	}

	if len(config.Spec.Transform.IncludeFields) != 5 {
		t.Errorf("Transform.IncludeFields length = %d, want 5", len(config.Spec.Transform.IncludeFields))
	}

	if !config.Spec.Transform.ExcludePII {
		t.Error("Transform.ExcludePII should be true")
	}
}

func TestSessionStreamingConfigStatus(t *testing.T) {
	now := metav1.Now()
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-config",
		},
		Status: SessionStreamingConfigStatus{
			Phase:              SessionStreamingConfigPhaseActive,
			ObservedGeneration: 5,
			Connected:          true,
			LastEventAt:        &now,
			EventsPublished:    12345,
			Errors: []StreamingErrorDetail{
				{
					Timestamp: now,
					Message:   "connection timeout",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:               "Connected",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "Connected",
					Message:            "Successfully connected to streaming provider",
				},
			},
		},
	}

	if config.Status.Phase != SessionStreamingConfigPhaseActive {
		t.Errorf("Status.Phase = %v, want Active", config.Status.Phase)
	}

	if config.Status.ObservedGeneration != 5 {
		t.Errorf("Status.ObservedGeneration = %v, want 5", config.Status.ObservedGeneration)
	}

	if !config.Status.Connected {
		t.Error("Status.Connected should be true")
	}

	if config.Status.LastEventAt == nil {
		t.Fatal("Status.LastEventAt should not be nil")
	}

	if config.Status.EventsPublished != 12345 {
		t.Errorf("Status.EventsPublished = %v, want 12345", config.Status.EventsPublished)
	}

	if len(config.Status.Errors) != 1 {
		t.Fatalf("len(Status.Errors) = %v, want 1", len(config.Status.Errors))
	}

	if config.Status.Errors[0].Message != "connection timeout" {
		t.Errorf("Status.Errors[0].Message = %v, want 'connection timeout'", config.Status.Errors[0].Message)
	}

	if len(config.Status.Conditions) != 1 {
		t.Fatalf("len(Status.Conditions) = %v, want 1", len(config.Status.Conditions))
	}

	if config.Status.Conditions[0].Type != "Connected" {
		t.Errorf("Condition type = %v, want Connected", config.Status.Conditions[0].Type)
	}
}

const testModifiedStreamingName = "modified"

func TestSessionStreamingConfigDeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "original",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: StreamingProviderKafka,
			Kafka: &KafkaConfig{
				Brokers:      []string{"kafka:9092"},
				Topic:        "omnia-sessions",
				PartitionKey: "session_id",
				Compression:  KafkaCompressionSnappy,
				Acks:         KafkaAcksAll,
				Retries:      3,
				Auth: &KafkaAuthConfig{
					Mechanism: "SASL_PLAINTEXT",
					SecretRef: LocalObjectReference{Name: "kafka-credentials"},
				},
			},
			Filter: &StreamingFilterConfig{
				EventTypes: []string{"message_added"},
			},
			Transform: &StreamingTransformConfig{
				Format:     TransformFormatJSON,
				ExcludePII: true,
			},
		},
		Status: SessionStreamingConfigStatus{
			Phase:              SessionStreamingConfigPhaseActive,
			ObservedGeneration: 1,
			Connected:          true,
			LastEventAt:        &now,
			EventsPublished:    100,
			Conditions: []metav1.Condition{
				{
					Type:   "Connected",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object, not the same pointer")
	}

	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %v, want %v", copied.Name, original.Name)
	}

	if copied.Status.Phase != original.Status.Phase {
		t.Errorf("DeepCopy().Status.Phase = %v, want %v", copied.Status.Phase, original.Status.Phase)
	}

	// Verify nested pointer fields are also deep copied
	if copied.Spec.Kafka == original.Spec.Kafka {
		t.Error("DeepCopy should create new Kafka pointer")
	}

	if copied.Spec.Kafka.Auth == original.Spec.Kafka.Auth {
		t.Error("DeepCopy should create new Kafka.Auth pointer")
	}

	if copied.Spec.Filter == original.Spec.Filter {
		t.Error("DeepCopy should create new Filter pointer")
	}

	if copied.Spec.Transform == original.Spec.Transform {
		t.Error("DeepCopy should create new Transform pointer")
	}

	// Modify the copy and verify original is unchanged
	copied.Name = testModifiedStreamingName
	if original.Name == testModifiedStreamingName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionStreamingConfigListDeepCopy(t *testing.T) {
	original := &SessionStreamingConfigList{
		Items: []SessionStreamingConfig{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "config1",
				},
				Spec: SessionStreamingConfigSpec{
					Enabled:  true,
					Provider: StreamingProviderKafka,
					Kafka: &KafkaConfig{
						Brokers: []string{"kafka:9092"},
						Topic:   "test",
					},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object")
	}

	if len(copied.Items) != len(original.Items) {
		t.Errorf("DeepCopy().Items length = %v, want %v", len(copied.Items), len(original.Items))
	}

	copied.Items[0].Name = testModifiedStreamingName
	if original.Items[0].Name == testModifiedStreamingName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionStreamingConfigTypeRegistration(t *testing.T) {
	config := &SessionStreamingConfig{}
	configList := &SessionStreamingConfigList{}

	// These should not panic if types are registered correctly
	_ = config.DeepCopyObject()
	_ = configList.DeepCopyObject()
}

func TestSessionStreamingConfigDisabled(t *testing.T) {
	config := &SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "disabled-streaming",
		},
		Spec: SessionStreamingConfigSpec{
			Enabled:  false,
			Provider: StreamingProviderKafka,
		},
	}

	if config.Spec.Enabled {
		t.Error("Spec.Enabled should be false")
	}

	if config.Spec.Kafka != nil {
		t.Error("Spec.Kafka should be nil when not configured")
	}
}
