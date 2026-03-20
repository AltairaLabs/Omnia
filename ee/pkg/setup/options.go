/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package setup

import (
	"github.com/altairalabs/omnia/ee/pkg/metrics"
)

// EnterpriseOptions contains all configuration needed by EE controllers.
type EnterpriseOptions struct {
	// LicenseServerURL is the URL of the license activation server.
	LicenseServerURL string

	// ClusterName is the human-readable name for this cluster in license records.
	ClusterName string

	// EnableWebhooks enables admission webhook registration for EE resources.
	EnableWebhooks bool

	// EnableAnalytics enables the SessionAnalyticsSync controller.
	EnableAnalytics bool

	// EnableStreaming enables the SessionStreamingConfig controller.
	EnableStreaming bool

	// PrivacyMetrics provides pre-created privacy policy metrics.
	// If nil, new metrics are created using the default Prometheus registry.
	PrivacyMetrics *metrics.PrivacyPolicyMetrics
}
