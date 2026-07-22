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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

// capabilityReportGracePeriod is how long after a Deployment becomes Available
// the operator waits for the runtime to self-report its capabilities before
// treating a non-reporting runtime as legacy (advertising nothing).
const capabilityReportGracePeriod = 60 * time.Second

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

// earliestRequeue returns the soonest non-zero requeue delay among a and b, or 0
// when both are 0 (no requeue).
func earliestRequeue(a, b time.Duration) time.Duration {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

// deploymentAvailable reports whether a Deployment's Available condition is True,
// and how long it has been available.
func deploymentAvailable(d *appsv1.Deployment, now time.Time) (bool, time.Duration) {
	if d == nil {
		return false, 0
	}
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			return true, now.Sub(c.LastTransitionTime.Time)
		}
	}
	return false, 0
}

// enforceCapabilities gates the AgentRuntime on the capabilities its running
// runtime advertises (§4.4). It sets the CapabilitiesSatisfied condition and, on
// a missing required capability, emits a Warning event; the deployment builder
// scales to 0 while the condition is False for the current generation. Returns a
// requeue delay while still waiting within the report grace window.
func (r *AgentRuntimeReconciler) enforceCapabilities(
	log logr.Logger, ar *omniav1alpha1.AgentRuntime, deployment *appsv1.Deployment,
) time.Duration {
	// Already determined missing for this generation: the runtime proved it lacks
	// a required capability and the Deployment is scaled to 0. Keep the verdict
	// until a spec/image change bumps the generation — re-evaluating now (no pod
	// running, so not Available) would flip to pending, scale back up, and
	// oscillate.
	if capabilitiesMismatchForCurrentGen(ar) {
		return 0
	}

	required := requiredCapabilities(ar)
	if len(required) == 0 {
		SetCondition(&ar.Status.Conditions, ar.Generation, ConditionTypeCapabilitiesSatisfied,
			metav1.ConditionTrue, reasonCapabilitiesSatisfied, "no capability requirements")
		return 0
	}

	available, since := deploymentAvailable(deployment, time.Now())
	reported := meta.IsStatusConditionPresentAndEqual(ar.Status.Conditions,
		k8s.ConditionRuntimeCapabilitiesReported, metav1.ConditionTrue)

	decision, missing := evaluateCapabilities(required, ar.Status.RuntimeCapabilities,
		reported, available, since, capabilityReportGracePeriod)

	switch decision {
	case capsSatisfied:
		SetCondition(&ar.Status.Conditions, ar.Generation, ConditionTypeCapabilitiesSatisfied,
			metav1.ConditionTrue, reasonCapabilitiesSatisfied, "runtime advertises all required capabilities")
		return 0
	case capsPending:
		SetCondition(&ar.Status.Conditions, ar.Generation, ConditionTypeCapabilitiesSatisfied,
			metav1.ConditionUnknown, reasonCapabilitiesPending, "waiting for the runtime to report its capabilities")
		if available && since < capabilityReportGracePeriod {
			return capabilityReportGracePeriod - since // re-check when the grace window elapses
		}
		return 0 // not yet Available — a deployment/status event will re-trigger
	default: // capsMissing
		msg := fmt.Sprintf("runtime is missing required capabilities %v; advertises %v",
			missing, ar.Status.RuntimeCapabilities)
		log.Info("capability mismatch: scaling agent to zero", "missing", missing,
			"advertised", ar.Status.RuntimeCapabilities)
		SetCondition(&ar.Status.Conditions, ar.Generation, ConditionTypeCapabilitiesSatisfied,
			metav1.ConditionFalse, reasonCapabilitiesMissing, msg)
		if r.Recorder != nil {
			r.Recorder.Event(ar, corev1.EventTypeWarning, reasonCapabilitiesMissing, msg)
		}
		return 0
	}
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
