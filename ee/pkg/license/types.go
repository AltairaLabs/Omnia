/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
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
	// WhiteLabel enables dashboard white-label / custom branding.
	WhiteLabel bool `json:"whiteLabel"`
	// MemoryEnterprise enables the enterprise memory features: Memory Galaxy
	// projection, memory analytics, institutional memory, multi-tier recall,
	// and consolidation (memory-api enterprise paths).
	MemoryEnterprise bool `json:"memoryEnterprise"`
	// PrivacyEnterprise enables the privacy/compliance suite served by
	// privacy-api: consent, opt-out, DSAR erasure, the central audit hub, and
	// enforcement-stats.
	PrivacyEnterprise bool `json:"privacyEnterprise"`
	// ToolPolicy licenses ToolPolicy CEL enforcement via the policy-broker.
	// (AgentPolicy's toolAccess is core/OSS Istio-based and not gated here.)
	// json tag stays "policyProxy" for signed-license wire compatibility; the
	// feature is ToolPolicy/CEL enforcement (policy-proxy was retired).
	ToolPolicy bool `json:"policyProxy"`
	// CustomFacade licenses bring-your-own-container facades
	// (spec.facades[].type == "custom") on AgentRuntimes. Enforced at admission
	// by the EE validating webhook; core AgentRuntime reconciliation is not
	// license-aware.
	CustomFacade bool `json:"customFacade"`
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

// License represents a validated license. Its JSON form is the single
// canonical wire representation, produced both by the validator (from the
// signed JWT in the Secret/ConfigMap) and by the operator's /api/v1/license
// endpoint, and parsed back into this same struct by license.Client — one
// struct, one parser, regardless of source. The signed-JWT claim keys
// (lid/iat/exp) live on the separate licenseClaims struct in the validator, so
// these tags are free to be the friendly names every consumer already expects.
type License struct {
	// ID is the unique license identifier.
	ID string `json:"id"`
	// Tier is the license tier (open-core or enterprise).
	Tier Tier `json:"tier"`
	// Customer is the name of the licensed customer.
	Customer string `json:"customer"`
	// Features defines which features are enabled.
	Features Features `json:"features"`
	// Limits defines resource limits.
	Limits Limits `json:"limits"`
	// IssuedAt is when the license was issued.
	IssuedAt time.Time `json:"issuedAt"`
	// ExpiresAt is when the license expires.
	ExpiresAt time.Time `json:"expiresAt"`
}

// OpenCoreLicense returns a default open-core license.
func OpenCoreLicense() *License {
	return &License{
		ID:       "open-core",
		Tier:     TierOpenCore,
		Customer: "Open Core User",
		Features: Features{
			GitSource:          true, // Git sources are included in open-core
			OCISource:          false,
			S3Source:           false,
			LoadTesting:        false,
			DataGeneration:     false,
			Scheduling:         false,
			DistributedWorkers: false,
			// Enterprise memory/privacy/policy features are paid; open-core
			// deployments degrade to open-core behavior (feature disabled).
			MemoryEnterprise:  false,
			PrivacyEnterprise: false,
			ToolPolicy:        false,
			CustomFacade:      false,
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

// DevLicense returns a full-featured development license.
// This should ONLY be used for testing and development, never in production.
func DevLicense() *License {
	return &License{
		ID:       "dev-mode",
		Tier:     TierEnterprise,
		Customer: "Development Mode",
		Features: Features{
			GitSource:          true,
			OCISource:          true,
			S3Source:           true,
			LoadTesting:        true,
			DataGeneration:     true,
			Scheduling:         true,
			DistributedWorkers: true,
			WhiteLabel:         true,
			MemoryEnterprise:   true,
			PrivacyEnterprise:  true,
			ToolPolicy:         true,
			CustomFacade:       true,
		},
		Limits: Limits{
			MaxScenarios:      0, // unlimited
			MaxWorkerReplicas: 0, // unlimited
		},
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

// IsValidEnterprise reports whether this is a currently-valid enterprise
// license — enterprise tier and not expired. It's the condition the startup
// nag uses: anything else (open-core, missing, or expired) is treated as
// "unlicensed" and triggers the reminder.
func (l *License) IsValidEnterprise() bool {
	return l.IsEnterprise() && !l.IsExpired()
}

// CanUseSourceType returns true if the given source type is allowed.
func (l *License) CanUseSourceType(sourceType string) bool {
	switch sourceType {
	case "configmap", "workspace":
		// ConfigMap and workspace are deploy infrastructure, always allowed.
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

// Enterprise-bundle entitlements are tier-derived: any valid enterprise
// license grants them, including capabilities added after the license was
// issued. The per-feature Features bit is an optional override that can grant a
// single capability to a non-enterprise (e.g. trial) license. This is what
// keeps "ship a new enterprise feature" from requiring every customer's license
// to be re-issued — the enterprise tier already covers it.

// CanUseMemoryEnterprise returns true if the enterprise memory features
// (Galaxy, analytics, institutional, multi-tier, consolidation) are licensed.
func (l *License) CanUseMemoryEnterprise() bool {
	return l.Features.MemoryEnterprise || l.IsEnterprise()
}

// CanUsePrivacyEnterprise returns true if the privacy/compliance suite
// (consent, DSAR, audit hub, enforcement-stats) is licensed.
func (l *License) CanUsePrivacyEnterprise() bool {
	return l.Features.PrivacyEnterprise || l.IsEnterprise()
}

// CanUseToolPolicy returns true if ToolPolicy CEL enforcement is licensed.
func (l *License) CanUseToolPolicy() bool {
	return l.Features.ToolPolicy || l.IsEnterprise()
}

// CanUseCustomFacade returns true if bring-your-own-container "custom" facades
// (spec.facades[].type == "custom") are licensed.
func (l *License) CanUseCustomFacade() bool {
	return l.Features.CustomFacade || l.IsEnterprise()
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
