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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

func TestCapabilitiesMismatchForCurrentGen(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "no condition -> no mismatch")

	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionFalse,
		ObservedGeneration: 5, Reason: "CapabilitiesMissing",
	})
	assert.True(t, capabilitiesMismatchForCurrentGen(ar), "False at current gen -> mismatch")

	ar.Generation = 6 // spec/image changed
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "stale generation -> no mismatch")

	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionTrue,
		ObservedGeneration: 6, Reason: "CapabilitiesSatisfied",
	})
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "True -> no mismatch")
}

func TestEvaluateCapabilities(t *testing.T) {
	dup := []string{contract.CapabilityDuplexAudio}
	cases := []struct {
		name         string
		req, adv     []string
		reported     bool
		available    bool
		since, grace time.Duration
		want         capabilityDecision
		wantMissing  []string
	}{
		{"none required", nil, nil, true, true, 0, time.Minute, capsSatisfied, nil},
		{"satisfied", dup, []string{contract.CapabilityDuplexAudio, contract.CapabilityInvoke}, true, true, 0, time.Minute, capsSatisfied, nil},
		{"missing reported", dup, []string{contract.CapabilityInvoke}, true, true, 0, time.Minute, capsMissing, dup},
		{"not available", dup, nil, false, false, 0, time.Minute, capsPending, nil},
		{"within grace", dup, nil, false, true, 10 * time.Second, time.Minute, capsPending, nil},
		{"legacy past grace", dup, nil, false, true, 2 * time.Minute, time.Minute, capsMissing, dup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, missing := evaluateCapabilities(tc.req, tc.adv, tc.reported, tc.available, tc.since, tc.grace)
			assert.Equal(t, tc.want, got)
			assert.ElementsMatch(t, tc.wantMissing, missing)
		})
	}
}

func TestRequiredCapabilities(t *testing.T) {
	dup := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Duplex: &omniav1alpha1.DuplexConfig{Enabled: true},
	}}
	assert.ElementsMatch(t,
		[]string{contract.CapabilityDuplexAudio, contract.CapabilityInterruption},
		requiredCapabilities(dup))

	fn := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeMCP}},
	}}
	assert.Equal(t, []string{contract.CapabilityInvoke}, requiredCapabilities(fn))

	rest := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeREST}},
	}}
	assert.Equal(t, []string{contract.CapabilityInvoke}, requiredCapabilities(rest))

	plain := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}},
	}}
	assert.Empty(t, requiredCapabilities(plain))
}
