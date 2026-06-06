/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"testing"
)

// TestMain isolates the cmd/agent test binary from the developer's ambient
// kubeconfig. Several constructors (buildWebSocketServer → buildAuthChain →
// buildK8sClient) call ctrl.GetConfig(); with a live kubeconfig present that
// reaches a real cluster and the auth-chain build fails, which previously
// crashed the whole test binary via os.Exit(1) — silently, since the tests
// pass logr.Discard() (#1208). Pointing KUBECONFIG at nothing makes
// buildK8sClient return nil so the no-cluster (mgmt-only) auth path is taken
// deterministically, matching CI. The os.Exit was also removed from
// buildWebSocketServer (it now returns an error); this keeps the tests
// kubeconfig-independent regardless.
func TestMain(m *testing.M) {
	_ = os.Setenv("KUBECONFIG", "/dev/null")
	os.Exit(m.Run())
}
