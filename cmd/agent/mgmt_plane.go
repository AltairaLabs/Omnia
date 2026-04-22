/*
Copyright 2026.

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

package main

import (
	"errors"
	"os"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// envMgmtPlanePubkeyPath is the env var the operator sets on the facade
// container pointing at the mounted dashboard mgmt-plane public key. Kept
// in sync with internal/controller/constants.go:EnvMgmtPlanePubkeyPath.
// Duplicated as a literal here to avoid importing the controller package
// into the facade binary.
const envMgmtPlanePubkeyPath = "OMNIA_MGMT_PLANE_PUBKEY_PATH"

// loadMgmtPlaneValidator constructs an auth.MgmtPlaneValidator when the
// env var points at an existing, readable PEM file. Returns (nil, nil)
// when the var is unset, the file is absent (the ConfigMap mirror hasn't
// landed yet, or the dashboard isn't deployed), or the file is empty —
// in every case the caller runs without mgmt-plane validation, which is
// PR 1a's default.
//
// Any other error (malformed PEM, non-RSA key) is surfaced so the facade
// startup fails loudly rather than silently running without auth — an
// actively-broken keypair shouldn't downgrade to "no mgmt plane".
func loadMgmtPlaneValidator(log logr.Logger) (auth.Validator, error) {
	path := os.Getenv(envMgmtPlanePubkeyPath)
	if path == "" {
		log.V(1).Info("mgmt-plane validator skipped",
			"reason", "env var unset",
			"envVar", envMgmtPlanePubkeyPath)
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.V(1).Info("mgmt-plane validator skipped",
				"reason", "pubkey file missing",
				"path", path)
			return nil, nil
		}
		return nil, err
	}
	if info.Size() == 0 {
		log.V(1).Info("mgmt-plane validator skipped",
			"reason", "pubkey file empty",
			"path", path)
		return nil, nil
	}

	v, err := auth.NewMgmtPlaneValidator(path)
	if err != nil {
		return nil, err
	}
	log.Info("mgmt-plane validator enabled", "pubkeyPath", path)
	return v, nil
}
