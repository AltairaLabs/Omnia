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
	"strconv"

	"github.com/go-logr/logr"
)

// envFacadeAllowUnauthenticated is the escape hatch used by dev / CI to
// let the facade accept unauthenticated requests even when the auth
// chain is empty. Production MUST leave this unset — the default is
// strict rejection, which is what closes pen-test finding C-3 in the
// "no externalAuth, no mgmt-plane" configuration.
//
// Accepted values are "1", "true", "TRUE", "t", "T" (via strconv.ParseBool).
const envFacadeAllowUnauthenticated = "OMNIA_FACADE_ALLOW_UNAUTHENTICATED"

// allowUnauthenticatedFallback returns the effective allowUnauthenticated
// setting passed to facade.WithAllowUnauthenticated. Default is false
// (strict). Setting the env var to a truthy value flips to permissive.
//
// Called by both the WS and A2A startup paths so the strict default
// applies uniformly — no code path can accidentally serve traffic when
// the chain ends up empty (e.g. mgmt-plane pubkey mirror unreadable at
// pod start because the Workspace controller has not yet reconciled).
func allowUnauthenticatedFallback(log logr.Logger) bool {
	raw := os.Getenv(envFacadeAllowUnauthenticated)
	if raw == "" {
		return false
	}
	allow, err := strconv.ParseBool(raw)
	if err != nil {
		// Unparseable — fail safe. Operators who misspell the flag
		// don't silently downgrade security.
		log.Error(err, "invalid value for env var — falling back to strict rejection",
			"var", envFacadeAllowUnauthenticated,
			"value", raw)
		return false
	}
	if allow {
		log.Info("facade strict-default DISABLED — unauthenticated requests will be admitted when the auth chain is empty",
			"var", envFacadeAllowUnauthenticated,
			"reason", "dev/test escape hatch; never set in production")
	}
	return allow
}
