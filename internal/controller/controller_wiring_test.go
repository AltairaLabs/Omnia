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

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Wiring test: verifies that the field indexers and the AgentRuntime
// controller (including the ToolRegistry watch, #1491) register against a real
// manager without error. Code that exists but isn't wired into the manager is a
// common silent failure mode, so this asserts the registration contract.
var _ = Describe("AgentRuntime controller wiring", func() {
	It("registers field indexers and the controller with the manager", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  scheme.Scheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())

		// SetupIndexers registers the Provider/PromptPack/ToolRegistry field indexers.
		Expect(SetupIndexers(ctx, mgr)).To(Succeed())

		// SetupWithManager wires the reconciler and its watches (Provider,
		// PromptPack, ToolRegistry, Secret).
		r := &AgentRuntimeReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		Expect(r.SetupWithManager(mgr)).To(Succeed())
	})
})
