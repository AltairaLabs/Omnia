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

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

// capabilityDecision is the outcome of comparing required vs advertised caps.
type capabilityDecision int

const (
	capsSatisfied capabilityDecision = iota
	capsMissing
	capsPending
)

// evaluateCapabilities decides whether a runtime satisfies its required caps.
// Gated on the Deployment being Available first, so a crash-looping runtime
// (never Available) is never judged here. Before the runtime reports, it stays
// pending until the grace window elapses, after which a non-reporting (legacy)
// runtime is treated as advertising nothing.
func evaluateCapabilities(
	required, advertised []string, reported, deployAvailable bool,
	sinceAvailable, grace time.Duration,
) (capabilityDecision, []string) {
	if len(required) == 0 {
		return capsSatisfied, nil
	}
	if !deployAvailable {
		return capsPending, nil
	}
	if !reported && sinceAvailable < grace {
		return capsPending, nil
	}
	have := make(map[string]bool, len(advertised))
	for _, a := range advertised {
		have[a] = true
	}
	var missing []string
	for _, r := range required {
		if !have[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return capsMissing, missing
	}
	return capsSatisfied, nil
}

// capabilitiesMismatchForCurrentGen reports whether the AgentRuntime is known to
// be missing a required capability for its CURRENT spec generation. A spec/image
// change bumps the generation, so a prior mismatch no longer applies and the
// runtime is allowed to run and re-report.
func capabilitiesMismatchForCurrentGen(ar *omniav1alpha1.AgentRuntime) bool {
	cond := meta.FindStatusCondition(ar.Status.Conditions, ConditionTypeCapabilitiesSatisfied)
	return cond != nil && cond.Status == metav1.ConditionFalse && cond.ObservedGeneration == ar.Generation
}

// requiredCapabilities returns the runtime capabilities this AgentRuntime's spec
// requires. Facade-visible and derivable only (§4.4): a duplex/voice agent needs
// realtime audio + interruption; a function-mode facade (rest/mcp) needs Invoke.
func requiredCapabilities(ar *omniav1alpha1.AgentRuntime) []string {
	var req []string
	if ar.Spec.Duplex != nil && ar.Spec.Duplex.Enabled {
		req = append(req, contract.CapabilityDuplexAudio, contract.CapabilityInterruption)
	}
	for _, f := range ar.Spec.Facades {
		if f.Type == omniav1alpha1.FacadeTypeREST || f.Type == omniav1alpha1.FacadeTypeMCP {
			req = append(req, contract.CapabilityInvoke)
			break
		}
	}
	return req
}
