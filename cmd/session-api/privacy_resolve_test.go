/*
Copyright 2025.

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
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/privacy/httpclient"
)

// TestResolvePrivacyPrefStore_EnvURL verifies that when PRIVACY_API_URL is set,
// resolvePrivacyPrefStore returns an *httpclient.Client.
func TestResolvePrivacyPrefStore_EnvURL(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "http://privacy-api.omnia-system:8080")

	store := resolvePrivacyPrefStore(context.Background(), "", "", nil, logr.Discard())
	if _, ok := store.(*httpclient.Client); !ok {
		t.Errorf("expected *httpclient.Client when PRIVACY_API_URL is set, got %T", store)
	}
}

// TestResolvePrivacyPrefStore_NoEnvEmptyWorkspace verifies that when no env var
// is set and workspace is empty, resolvePrivacyPrefStore returns the permissive
// store whose GetPreferences returns ErrPreferencesNotFound.
func TestResolvePrivacyPrefStore_NoEnvEmptyWorkspace(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")

	store := resolvePrivacyPrefStore(context.Background(), "", "", nil, logr.Discard())
	_, err := store.GetPreferences(context.Background(), "some-user")
	if !errors.Is(err, privacy.ErrPreferencesNotFound) {
		t.Errorf("expected ErrPreferencesNotFound from permissive store, got %v", err)
	}
}
