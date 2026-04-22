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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
)

func writeTestPubKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "pub.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create pem file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); err != nil {
		t.Fatalf("encode pem: %v", err)
	}
	return path
}

func TestLoadMgmtPlaneValidator_EnvUnset(t *testing.T) {
	t.Setenv(envMgmtPlanePubkeyPath, "")
	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v != nil {
		t.Errorf("validator = %v, want nil when env unset", v)
	}
}

func TestLoadMgmtPlaneValidator_FileMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.pem")
	t.Setenv(envMgmtPlanePubkeyPath, missing)
	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v, want nil (missing file is not fatal)", err)
	}
	if v != nil {
		t.Errorf("validator = %v, want nil when file missing", v)
	}
}

func TestLoadMgmtPlaneValidator_FileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	t.Setenv(envMgmtPlanePubkeyPath, path)

	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v, want nil (empty file treated as absent)", err)
	}
	if v != nil {
		t.Errorf("validator = %v, want nil when file empty", v)
	}
}

func TestLoadMgmtPlaneValidator_ValidPubkey(t *testing.T) {
	path := writeTestPubKey(t)
	t.Setenv(envMgmtPlanePubkeyPath, path)

	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil validator for valid pub key")
	}
}

func TestLoadMgmtPlaneValidator_MalformedPEM_Errors(t *testing.T) {
	// Malformed PEM must surface as a startup error, not a silent
	// downgrade to no-auth — otherwise an operator misconfiguration
	// looks identical to "mgmt-plane not configured".
	path := filepath.Join(t.TempDir(), "garbage.pem")
	if err := os.WriteFile(path, []byte("this is not pem"), 0o600); err != nil {
		t.Fatalf("write garbage pem: %v", err)
	}
	t.Setenv(envMgmtPlanePubkeyPath, path)

	v, err := loadMgmtPlaneValidator(logr.Discard())
	if err == nil {
		t.Errorf("err = nil, want non-nil for malformed PEM (got validator %v)", v)
	}
}
