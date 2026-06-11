/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/altairalabs/omnia/ee/internal/controller"
	arenawebhook "github.com/altairalabs/omnia/ee/internal/webhook"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// Controller names. Declared as constants to satisfy goconst (the names
// appear in both setup.go and setup_test.go); also gives the test a
// single source of truth to assert against.
const (
	controllerArenaSource         = "ArenaSource"
	controllerArenaTemplateSource = "ArenaTemplateSource"
	controllerArenaJob            = "ArenaJob"
	controllerArenaDevSession     = "ArenaDevSession"
	controllerKeyRotation         = "KeyRotation"
)

// registrationOptions bundles every flag-derived input registerArenaWorkloads
// needs. Extracted so main() shrinks to a 3-line wiring call + error handler
// — the bulk of the registration logic lives in this file where it can be
// unit-tested without spinning up the full operator binary.
type registrationOptions struct {
	Controllers        setupOptions
	Webhooks           webhookOptions
	EnableWebhooks     bool
	EnableLicenseHooks bool
}

// registerArenaWorkloads runs the controller + webhook registration
// against the supplied manager. The two `Info` lines about how many
// controllers/webhooks registered are kept in setup.go so they stay
// covered by the wiring tests that exercise this function with a real
// envtest-backed manager.
func registerArenaWorkloads(mgr ctrl.Manager, opts registrationOptions, log logr.Logger) error {
	registered, err := setupControllers(mgr, opts.Controllers)
	if err != nil {
		return fmt.Errorf("setup controllers (last attempted: %s): %w",
			lastOrEmpty(registered), err)
	}
	log.Info("controllers registered", "count", len(registered), "controllers", registered)

	if !opts.EnableWebhooks {
		return nil
	}
	registeredWebhooks, whErr := setupWebhooks(mgr, opts.Webhooks)
	if whErr != nil {
		return fmt.Errorf("setup webhooks (last attempted: %s): %w",
			lastOrEmpty(registeredWebhooks), whErr)
	}
	log.Info("webhooks registered", "count", len(registeredWebhooks), "webhooks", registeredWebhooks)
	log.Info("webhook server enabled")
	return nil
}

func lastOrEmpty(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[len(names)-1]
}

// setupOptions bundles every dependency needed to register the 6
// Arena-controller reconcilers. Splitting out of main() lets a wiring
// test assert each reconciler is registered without re-reading the
// 100-line inline block in main().
type setupOptions struct {
	WorkerImage              string
	WorkerImagePullPolicy    corev1.PullPolicy
	DevConsoleImage          string
	DevConsoleServiceAccount string
	DevConsolePodLabels      map[string]string
	WorkspaceContentPath     string
	WorkspaceContentScoped   bool
	NFSServer                string
	NFSPath                  string
	LicenseValidator         *license.Validator
	StorageManager           *workspace.StorageManager
	Aggregator               *aggregator.Aggregator
	RedisURL                 string
	RedisURLSecretName       string
	RedisURLSecretKey        string
	TracingEnabled           bool
	TracingEndpoint          string
	MgmtPlaneTokenURL        string
	PrivacyPolicyMetrics     *metrics.PrivacyPolicyMetrics
	ReEncryptionStore        func() (encryption.ReEncryptionStore, error)
}

// namedReconciler pairs a reconciler with its display name so the
// wiring test can assert "all six reconcilers are registered" rather
// than parsing reconciler-runtime internals (which expose no public
// introspection of the registered controller set).
type namedReconciler struct {
	Name  string
	Setup func(ctrl.Manager) error
}

// buildReconcilers returns the canonical 6-reconciler list the binary
// registers. Pure function — no manager interaction — so the wiring
// test can assert the name set without spinning up envtest.
//
// Order matters: setupControllers calls them in this order so a name
// mismatch in the test output points at the right line.
func buildReconcilers(opts setupOptions) []namedReconciler {
	return []namedReconciler{
		{
			Name: controllerArenaSource,
			Setup: func(mgr ctrl.Manager) error {
				return (&controller.ArenaSourceReconciler{
					Client:               mgr.GetClient(),
					Scheme:               mgr.GetScheme(),
					Recorder:             mgr.GetEventRecorderFor("arenasource-controller"),
					WorkspaceContentPath: opts.WorkspaceContentPath,
					MaxVersionsPerSource: 10,
					LicenseValidator:     opts.LicenseValidator,
					StorageManager:       opts.StorageManager,
				}).SetupWithManager(mgr)
			},
		},
		{
			Name: controllerArenaTemplateSource,
			Setup: func(mgr ctrl.Manager) error {
				return (&controller.ArenaTemplateSourceReconciler{
					Client:               mgr.GetClient(),
					Scheme:               mgr.GetScheme(),
					Recorder:             mgr.GetEventRecorderFor("arenatemplatesource-controller"),
					WorkspaceContentPath: opts.WorkspaceContentPath,
					MaxVersionsPerSource: 10,
					LicenseValidator:     opts.LicenseValidator,
					StorageManager:       opts.StorageManager,
				}).SetupWithManager(mgr)
			},
		},
		{
			Name: controllerArenaJob,
			Setup: func(mgr ctrl.Manager) error {
				return (&controller.ArenaJobReconciler{
					Client:                 mgr.GetClient(),
					Scheme:                 mgr.GetScheme(),
					Recorder:               mgr.GetEventRecorderFor("arenajob-controller"),
					WorkerImage:            opts.WorkerImage,
					WorkerImagePullPolicy:  opts.WorkerImagePullPolicy,
					LicenseValidator:       opts.LicenseValidator,
					Aggregator:             opts.Aggregator,
					RedisURL:               opts.RedisURL,
					RedisURLSecretName:     opts.RedisURLSecretName,
					RedisURLSecretKey:      opts.RedisURLSecretKey,
					WorkspaceContentPath:   opts.WorkspaceContentPath,
					WorkspaceContentScoped: opts.WorkspaceContentScoped,
					NFSServer:              opts.NFSServer,
					NFSPath:                opts.NFSPath,
					StorageManager:         opts.StorageManager,
					TracingEnabled:         opts.TracingEnabled,
					TracingEndpoint:        opts.TracingEndpoint,
					MgmtPlaneTokenURL:      opts.MgmtPlaneTokenURL,
				}).SetupWithManager(mgr)
			},
		},
		{
			Name: controllerArenaDevSession,
			Setup: func(mgr ctrl.Manager) error {
				return (&controller.ArenaDevSessionReconciler{
					Client:                   mgr.GetClient(),
					Scheme:                   mgr.GetScheme(),
					DevConsoleImage:          opts.DevConsoleImage,
					DevConsoleServiceAccount: opts.DevConsoleServiceAccount,
					DevConsolePodLabels:      opts.DevConsolePodLabels,
				}).SetupWithManager(mgr)
			},
		},
		{
			Name: controllerKeyRotation,
			Setup: func(mgr ctrl.Manager) error {
				return (&controller.KeyRotationReconciler{
					Client:          mgr.GetClient(),
					Scheme:          mgr.GetScheme(),
					Recorder:        mgr.GetEventRecorderFor("keyrotation-controller"),
					ProviderFactory: encryptionProviderFactory,
					StoreFactory:    opts.ReEncryptionStore,
				}).SetupWithManager(mgr)
			},
		},
	}
}

// newPrivacyPolicyMetrics is the privacy-policy metrics constructor +
// initialiser wired by main(). Extracted so the setup_test.go
// integration test can use the same metrics instance the binary uses
// (avoids drift between binary and test wiring).
func newPrivacyPolicyMetrics() *metrics.PrivacyPolicyMetrics {
	m := metrics.NewPrivacyPolicyMetrics()
	m.Initialize()
	return m
}

// encryptionProviderFactory is the KeyRotation controller's
// ProviderFactory hook. Extracted out of buildReconcilers so the
// closure body is unit-testable — wiring tests can invoke it directly
// rather than dragging in the full key-rotation reconcile loop.
func encryptionProviderFactory(cfg encryption.ProviderConfig) (encryption.Provider, error) {
	return encryption.NewProvider(cfg)
}

// setupControllers registers every reconciler buildReconcilers returns,
// in declaration order. Returns the names of the reconcilers that were
// successfully registered; on error the last-attempted name is the
// final entry in the returned slice (useful for the error log in main).
func setupControllers(mgr ctrl.Manager, opts setupOptions) ([]string, error) {
	return registerNamed(mgr, buildReconcilers(opts))
}

// registerNamed iterates a named-reconciler list, calling Setup on each
// with the given manager. Extracted from setupControllers so the
// success + error paths are unit-testable without standing up envtest
// (envtest covers the closure bodies; this helper covers the loop).
func registerNamed(mgr ctrl.Manager, items []namedReconciler) ([]string, error) {
	registered := make([]string, 0, len(items))
	for _, r := range items {
		if err := r.Setup(mgr); err != nil {
			return append(registered, r.Name), fmt.Errorf("%s: %w", r.Name, err)
		}
		registered = append(registered, r.Name)
	}
	return registered, nil
}

// webhookOptions bundles the inputs setupWebhooks needs. Mirrors
// setupOptions in shape so tests can construct it standalone.
type webhookOptions struct {
	LicenseValidator    *license.Validator
	IncludeLicenseHooks bool
}

// namedWebhook pairs a webhook with its display name so the wiring
// test can assert "the right webhooks are registered for the given
// enable flags".
type namedWebhook struct {
	Name  string
	Setup func(ctrl.Manager) error
}

// buildWebhooks returns the list of webhooks the binary registers.
// SessionPrivacyPolicy is owned by the operator (the always-present
// enterprise controller-manager), not this license-gated binary; this
// binary serves only the license-validation Arena webhooks, conditional
// on the IncludeLicenseHooks flag. Pure function — tests assert the name
// set for both flag states.
func buildWebhooks(opts webhookOptions) []namedWebhook {
	var hooks []namedWebhook
	if opts.IncludeLicenseHooks {
		hooks = append(hooks,
			namedWebhook{
				Name: controllerArenaSource,
				Setup: func(mgr ctrl.Manager) error {
					return arenawebhook.SetupArenaSourceWebhookWithManager(mgr, opts.LicenseValidator)
				},
			},
			namedWebhook{
				Name: controllerArenaJob,
				Setup: func(mgr ctrl.Manager) error {
					return arenawebhook.SetupArenaJobWebhookWithManager(mgr, opts.LicenseValidator)
				},
			},
			namedWebhook{
				Name: controllerArenaTemplateSource,
				Setup: func(mgr ctrl.Manager) error {
					return arenawebhook.SetupArenaTemplateSourceWebhookWithManager(mgr, opts.LicenseValidator)
				},
			},
		)
	}
	return hooks
}

// setupWebhooks registers every webhook buildWebhooks returns, in
// declaration order. Same return-on-error contract as setupControllers.
func setupWebhooks(mgr ctrl.Manager, opts webhookOptions) ([]string, error) {
	return registerNamedWebhooks(mgr, buildWebhooks(opts))
}

// registerNamedWebhooks is the webhook-shaped twin of registerNamed.
// Extracted so the success + error paths are unit-testable.
func registerNamedWebhooks(mgr ctrl.Manager, items []namedWebhook) ([]string, error) {
	registered := make([]string, 0, len(items))
	for _, h := range items {
		if err := h.Setup(mgr); err != nil {
			return append(registered, h.Name), fmt.Errorf("%s: %w", h.Name, err)
		}
		registered = append(registered, h.Name)
	}
	return registered, nil
}
