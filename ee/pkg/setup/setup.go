/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package setup

import (
	"fmt"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	eecontroller "github.com/altairalabs/omnia/ee/internal/controller"
	"github.com/altairalabs/omnia/ee/internal/webhook"
	"github.com/altairalabs/omnia/ee/pkg/analyticsfactory"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

// eventRecorder returns the legacy event recorder for the given name.
// TODO: migrate EE controllers to events.EventRecorder, then use mgr.GetEventRecorder.
func eventRecorder(mgr ctrl.Manager, name string) record.EventRecorder {
	return mgr.GetEventRecorderFor(name) //nolint:staticcheck
}

const (
	logKeyController          = "controller"
	errControllerRegistration = "enterprise controller registration failed"
	errWebhookRegistration    = "enterprise webhook registration failed"
)

// RegisterEnterpriseControllers registers all enterprise-edition controllers
// and webhooks with the given manager based on the provided options.
func RegisterEnterpriseControllers(mgr ctrl.Manager, opts EnterpriseOptions) error {
	log := ctrl.Log.WithName("ee-setup")
	log.Info("registering enterprise controllers")

	if err := registerSessionPrivacyPolicy(mgr, opts.PrivacyMetrics); err != nil {
		return fmt.Errorf("%s: %w", errControllerRegistration, err)
	}
	log.V(1).Info("controller registered", logKeyController, "SessionPrivacyPolicy")

	if err := registerToolPolicy(mgr); err != nil {
		return fmt.Errorf("%s: %w", errControllerRegistration, err)
	}
	log.V(1).Info("controller registered", logKeyController, "ToolPolicy")

	if err := registerLicenseActivation(mgr, opts); err != nil {
		return fmt.Errorf("%s: %w", errControllerRegistration, err)
	}
	log.V(1).Info("controller registered", logKeyController, "LicenseActivation")

	if err := registerConditionalControllers(mgr, opts); err != nil {
		return err
	}

	if opts.EnableWebhooks {
		if err := registerWebhooks(mgr); err != nil {
			return fmt.Errorf("%s: %w", errWebhookRegistration, err)
		}
		log.V(1).Info("webhooks registered")
	}

	log.Info("enterprise controllers registered")
	return nil
}

// registerSessionPrivacyPolicy sets up the SessionPrivacyPolicy controller.
// If privacyMetrics is nil, new metrics are created using the default Prometheus registry.
func registerSessionPrivacyPolicy(mgr ctrl.Manager, privacyMetrics *metrics.PrivacyPolicyMetrics) error {
	if privacyMetrics == nil {
		privacyMetrics = metrics.NewPrivacyPolicyMetrics()
	}
	return (&eecontroller.SessionPrivacyPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: eventRecorder(mgr, "sessionprivacypolicy-controller"),
		Metrics:  privacyMetrics,
	}).SetupWithManager(mgr)
}

// registerToolPolicy sets up the ToolPolicy controller.
func registerToolPolicy(mgr ctrl.Manager) error {
	evaluator, err := policy.NewEvaluator()
	if err != nil {
		return fmt.Errorf("failed to create policy evaluator: %w", err)
	}
	return (&eecontroller.ToolPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  eventRecorder(mgr, "toolpolicy-controller"),
		Evaluator: evaluator,
	}).SetupWithManager(mgr)
}

// registerLicenseActivation sets up the LicenseActivation controller.
func registerLicenseActivation(mgr ctrl.Manager, opts EnterpriseOptions) error {
	validator, err := license.NewValidator(mgr.GetClient())
	if err != nil {
		return fmt.Errorf("failed to create license validator: %w", err)
	}

	var clientOpts []license.ActivationClientOption
	if opts.LicenseServerURL != "" {
		clientOpts = append(clientOpts, license.WithServerURL(opts.LicenseServerURL))
	}

	return (&eecontroller.LicenseActivationReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		Recorder:         eventRecorder(mgr, "license-activation-controller"),
		LicenseValidator: validator,
		ActivationClient: license.NewActivationClient(clientOpts...),
		ClusterName:      opts.ClusterName,
	}).SetupWithManager(mgr)
}

// registerConditionalControllers registers controllers gated by feature flags.
func registerConditionalControllers(mgr ctrl.Manager, opts EnterpriseOptions) error {
	log := ctrl.Log.WithName("ee-setup")

	if opts.EnableAnalytics {
		if err := registerSessionAnalyticsSync(mgr); err != nil {
			return fmt.Errorf("%s: %w", errControllerRegistration, err)
		}
		log.V(1).Info("controller registered", logKeyController, "SessionAnalyticsSync")
	}

	if opts.EnableStreaming {
		if err := registerSessionStreamingConfig(mgr); err != nil {
			return fmt.Errorf("%s: %w", errControllerRegistration, err)
		}
		log.V(1).Info("controller registered", logKeyController, "SessionStreamingConfig")
	}

	return nil
}

// registerSessionAnalyticsSync sets up the SessionAnalyticsSync controller.
func registerSessionAnalyticsSync(mgr ctrl.Manager) error {
	return (&eecontroller.SessionAnalyticsSyncReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        eventRecorder(mgr, "sessionanalyticssync-controller"),
		ProviderFactory: &analyticsfactory.Factory{},
	}).SetupWithManager(mgr)
}

// registerSessionStreamingConfig sets up the SessionStreamingConfig controller.
func registerSessionStreamingConfig(mgr ctrl.Manager) error {
	return (&eecontroller.SessionStreamingConfigReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: eventRecorder(mgr, "sessionstreamingconfig-controller"),
	}).SetupWithManager(mgr)
}

// registerWebhooks sets up all enterprise admission webhooks.
func registerWebhooks(mgr ctrl.Manager) error {
	return webhook.SetupSessionPrivacyPolicyWebhookWithManager(mgr)
}
