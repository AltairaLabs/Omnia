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

package license

import (
	"time"
)

// Tier represents the license tier.
type Tier string

const (
	// TierOpenCore is the free open-core tier with limited features.
	TierOpenCore Tier = "open-core"
	// TierEnterprise is the paid enterprise tier with full features.
	TierEnterprise Tier = "enterprise"
)

// Features defines which features are enabled in the license.
type Features struct {
	// GitSource enables Git repository sources for ArenaSources.
	GitSource bool `json:"gitSource"`
	// OCISource enables OCI registry sources for ArenaSources.
	OCISource bool `json:"ociSource"`
	// S3Source enables S3 sources for ArenaSources.
	S3Source bool `json:"s3Source"`
	// LoadTesting enables load testing job type.
	LoadTesting bool `json:"loadTesting"`
	// DataGeneration enables data generation job type.
	DataGeneration bool `json:"dataGeneration"`
	// Scheduling enables cron-based job scheduling.
	Scheduling bool `json:"scheduling"`
	// DistributedWorkers enables multiple worker replicas.
	DistributedWorkers bool `json:"distributedWorkers"`
}

// Limits defines the resource limits in the license.
type Limits struct {
	// MaxScenarios is the maximum number of scenarios allowed.
	// A value of 0 means unlimited.
	MaxScenarios int `json:"maxScenarios"`
	// MaxWorkerReplicas is the maximum number of worker replicas allowed.
	// A value of 0 means unlimited.
	MaxWorkerReplicas int `json:"maxWorkerReplicas"`
	// MaxActivations is the maximum number of cluster activations allowed.
	// A value of 0 means unlimited (typically used for trial licenses).
	MaxActivations int `json:"maxActivations,omitempty"`
}

// License represents a validated license.
type License struct {
	// ID is the unique license identifier.
	ID string `json:"lid"`
	// Tier is the license tier (open-core or enterprise).
	Tier Tier `json:"tier"`
	// Customer is the name of the licensed customer.
	Customer string `json:"customer"`
	// Features defines which features are enabled.
	Features Features `json:"features"`
	// Limits defines resource limits.
	Limits Limits `json:"limits"`
	// IssuedAt is when the license was issued.
	IssuedAt time.Time `json:"iat"`
	// ExpiresAt is when the license expires.
	ExpiresAt time.Time `json:"exp"`
}

// OpenCoreLicense returns a default open-core license.
func OpenCoreLicense() *License {
	return &License{
		ID:       "open-core",
		Tier:     TierOpenCore,
		Customer: "Open Core User",
		Features: Features{
			GitSource:          false,
			OCISource:          false,
			S3Source:           false,
			LoadTesting:        false,
			DataGeneration:     false,
			Scheduling:         false,
			DistributedWorkers: false,
		},
		Limits: Limits{
			MaxScenarios:      10,
			MaxWorkerReplicas: 1,
		},
		// Open core license never expires
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().AddDate(100, 0, 0),
	}
}

// IsExpired returns true if the license has expired.
func (l *License) IsExpired() bool {
	return time.Now().After(l.ExpiresAt)
}

// IsEnterprise returns true if this is an enterprise license.
func (l *License) IsEnterprise() bool {
	return l.Tier == TierEnterprise
}

// CanUseSourceType returns true if the given source type is allowed.
func (l *License) CanUseSourceType(sourceType string) bool {
	switch sourceType {
	case "configmap":
		// ConfigMap is always allowed
		return true
	case "git":
		return l.Features.GitSource
	case "oci":
		return l.Features.OCISource
	case "s3":
		return l.Features.S3Source
	default:
		return false
	}
}

// CanUseJobType returns true if the given job type is allowed.
func (l *License) CanUseJobType(jobType string) bool {
	switch jobType {
	case "evaluation":
		// Evaluation is always allowed
		return true
	case "loadtest":
		return l.Features.LoadTesting
	case "datagen":
		return l.Features.DataGeneration
	default:
		return false
	}
}

// CanUseScheduling returns true if cron scheduling is allowed.
func (l *License) CanUseScheduling() bool {
	return l.Features.Scheduling
}

// CanUseWorkerReplicas returns true if the given number of replicas is allowed.
func (l *License) CanUseWorkerReplicas(replicas int) bool {
	if l.Limits.MaxWorkerReplicas == 0 {
		// Unlimited
		return true
	}
	return replicas <= l.Limits.MaxWorkerReplicas
}

// CanUseScenarioCount returns true if the given number of scenarios is allowed.
func (l *License) CanUseScenarioCount(count int) bool {
	if l.Limits.MaxScenarios == 0 {
		// Unlimited
		return true
	}
	return count <= l.Limits.MaxScenarios
}

// ActivationState represents the activation status stored in a ConfigMap.
// This persists the activation state to survive pod restarts.
type ActivationState struct {
	// ActivationID is the unique identifier returned by the activation server.
	ActivationID string `json:"activation_id"`
	// ClusterFingerprint is the fingerprint of this cluster.
	ClusterFingerprint string `json:"cluster_fingerprint"`
	// LicenseID is the ID of the activated license.
	LicenseID string `json:"license_id"`
	// ActivatedAt is when the license was first activated on this cluster.
	ActivatedAt time.Time `json:"activated_at"`
	// LastHeartbeat is the time of the last successful heartbeat.
	LastHeartbeat time.Time `json:"last_heartbeat"`
	// HeartbeatFailures tracks consecutive heartbeat failures for grace period.
	HeartbeatFailures int `json:"heartbeat_failures,omitempty"`
}

// NeedsHeartbeat returns true if a heartbeat should be sent.
func (s *ActivationState) NeedsHeartbeat(interval time.Duration) bool {
	return time.Since(s.LastHeartbeat) >= interval
}

// IsInGracePeriod returns true if the activation is within the heartbeat grace period.
// The grace period allows the system to continue operating during temporary network issues.
const HeartbeatGracePeriod = 7 * 24 * time.Hour // 7 days

func (s *ActivationState) IsInGracePeriod() bool {
	if s.HeartbeatFailures == 0 {
		return true
	}
	return time.Since(s.LastHeartbeat) < HeartbeatGracePeriod
}
