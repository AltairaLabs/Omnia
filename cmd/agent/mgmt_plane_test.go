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
	"testing"

	"github.com/go-logr/logr"
)

func TestLoadMgmtPlaneValidator_EnvUnset(t *testing.T) {
	t.Setenv(envMgmtPlaneJWKSURL, "")
	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v != nil {
		t.Errorf("validator = %v, want nil when env unset", v)
	}
}

func TestLoadMgmtPlaneValidator_ValidJWKSURL(t *testing.T) {
	// JWKSResolver is created lazily — we don't fetch until the first
	// JWT verifies — so a non-empty URL just needs to parse, not point
	// at a live server, for the validator to construct successfully.
	t.Setenv(envMgmtPlaneJWKSURL, "http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks")
	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil validator for valid URL")
	}
}
