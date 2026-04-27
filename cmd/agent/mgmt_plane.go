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
	"os"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// envMgmtPlaneJWKSURL is the env var the operator sets on the facade
// container pointing at the dashboard's JWKS endpoint
// (e.g. http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks).
// Kept in sync with internal/controller/constants.go:EnvMgmtPlaneJWKSURL.
// Duplicated as a literal here to avoid importing the controller package
// into the facade binary.
const envMgmtPlaneJWKSURL = "OMNIA_MGMT_PLANE_JWKS_URL"

// loadMgmtPlaneValidator constructs an auth.MgmtPlaneValidator backed by
// a JWKS resolver pointed at the dashboard's signing-key endpoint.
//
// Returns (nil, nil) when the env var is unset — that's the expected
// shape for installs without a dashboard (Arena E2E, headless runtimes).
// Any construction error is surfaced so a misconfigured URL trips boot
// rather than silently downgrading to "no mgmt-plane validation".
//
// Errors at JWT-validation time (DNS lookup failure, dashboard down,
// 5xx response) bubble up to the auth chain as ErrInvalidCredential —
// see auth.JWKSResolver. Tested in
// internal/facade/auth/mgmt_plane_test.go's
// TestMgmtPlaneValidator_FetchesFromJWKSEndpoint.
func loadMgmtPlaneValidator(log logr.Logger) (auth.Validator, error) {
	url := os.Getenv(envMgmtPlaneJWKSURL)
	if url == "" {
		log.V(1).Info("mgmt-plane validator skipped",
			"reason", "env var unset",
			"envVar", envMgmtPlaneJWKSURL)
		return nil, nil
	}
	v, err := auth.NewMgmtPlaneValidator(url)
	if err != nil {
		return nil, err
	}
	log.Info("mgmt-plane validator enabled", "jwksURL", url)
	return v, nil
}
