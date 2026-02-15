/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import "time"

// WebhookConfig defines when and where to send a webhook alert
// based on eval pass rate thresholds.
type WebhookConfig struct {
	URL              string            `json:"url"`
	Threshold        float64           `json:"threshold"`        // pass rate threshold (0.0-1.0)
	WindowSize       int               `json:"windowSize"`       // number of recent results to evaluate
	ConsecutiveFails int               `json:"consecutiveFails"` // fire after N consecutive failures
	Headers          map[string]string `json:"headers,omitempty"`
}

// WebhookPayload is the JSON body sent to the webhook endpoint
// when an eval pass rate drops below the configured threshold.
type WebhookPayload struct {
	AgentName       string                 `json:"agentName"`
	Namespace       string                 `json:"namespace"`
	EvalID          string                 `json:"evalId"`
	CurrentPassRate float64                `json:"currentPassRate"`
	Threshold       float64                `json:"threshold"`
	WindowSize      int                    `json:"windowSize"`
	TriggeredAt     time.Time              `json:"triggeredAt"`
	RecentFailures  []WebhookFailureSample `json:"recentFailures,omitempty"`
}

// WebhookFailureSample describes a single recent eval failure
// included in the webhook payload for debugging context.
type WebhookFailureSample struct {
	SessionID string    `json:"sessionId"`
	MessageID string    `json:"messageId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
