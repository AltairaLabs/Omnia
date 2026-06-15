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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/policy"
)

func meshCfg() *omniav1alpha1.MeshTrafficRouting {
	return &omniav1alpha1.MeshTrafficRouting{StableSubset: trackStable, CandidateSubset: trackCanary}
}

func TestBuildOwnedDestinationRule_Subsets(t *testing.T) {
	dr := buildOwnedDestinationRule("agent1", "ns1", "agent1.ns1.svc.cluster.local", meshCfg())
	subsets, _, _ := unstructured.NestedSlice(dr.Object, "spec", "subsets")
	if len(subsets) != 2 {
		t.Fatalf("want 2 subsets, got %d", len(subsets))
	}
	first := subsets[0].(map[string]interface{})
	labels, _, _ := unstructured.NestedStringMap(first, "labels")
	// The subset MUST select on labelOmniaTrack — the exact key the deployment
	// builder stamps on pods. A bare "track" key matches zero pods → the mesh
	// data plane 503s (No Healthy Upstream) even though the VS/DR look correct.
	if labels[labelOmniaTrack] != trackStable {
		t.Fatalf("stable subset must select %s=stable, got %v", labelOmniaTrack, labels)
	}
	if _, hasBare := labels["track"]; hasBare {
		t.Fatalf("subset must not use the bare \"track\" key (won't match pods): %v", labels)
	}
}

func TestBuildOwnedVirtualService_Weights(t *testing.T) {
	vs := buildOwnedVirtualService("agent1", "ns1", []string{"agent1.ns1.svc.cluster.local"}, meshCfg(), 30)
	routes, _, _ := unstructured.NestedSlice(vs.Object, "spec", "http")
	dests, _, _ := unstructured.NestedSlice(routes[0].(map[string]interface{}), "route")
	weights := map[string]int64{}
	for _, d := range dests {
		dm := d.(map[string]interface{})
		subset, _, _ := unstructured.NestedString(dm, "destination", "subset")
		w, _, _ := unstructured.NestedInt64(dm, "weight")
		weights[subset] = w
	}
	if weights[trackStable] != 70 || weights[trackCanary] != 30 {
		t.Fatalf("want stable=70 canary=30, got %v", weights)
	}
}

// TestBuildOwnedVirtualService_VariantHeader asserts each weighted destination
// stamps the x-omnia-variant request header with its rollout-semantic variant
// (stable/candidate). Without this, candidate-routed sessions reach the facade
// with no variant header and are recorded variant="" — so RolloutAnalysis gates
// keyed on {variant="candidate"} never match and the rollout can't be analysed.
func TestBuildOwnedVirtualService_VariantHeader(t *testing.T) {
	vs := buildOwnedVirtualService("agent1", "ns1", []string{"agent1.ns1.svc.cluster.local"}, meshCfg(), 30)
	routes, _, _ := unstructured.NestedSlice(vs.Object, "spec", "http")
	dests, _, _ := unstructured.NestedSlice(routes[0].(map[string]interface{}), "route")

	got := map[string]string{}
	for _, d := range dests {
		dm := d.(map[string]interface{})
		subset, _, _ := unstructured.NestedString(dm, "destination", "subset")
		variant, found, _ := unstructured.NestedString(dm, "headers", "request", "set", variantHeader)
		if !found {
			t.Fatalf("subset %q destination missing %s request header", subset, variantHeader)
		}
		got[subset] = variant
	}
	if got[trackStable] != variantStable {
		t.Fatalf("stable subset must set %s=%s, got %q", variantHeader, variantStable, got[trackStable])
	}
	if got[trackCanary] != variantCandidate {
		t.Fatalf("candidate subset must set %s=%s, got %q", variantHeader, variantCandidate, got[trackCanary])
	}
}

// TestVariantHeader_MatchesPolicy guards against drift between the header the
// VirtualService stamps and the one the facade reads (pkg/policy.HeaderVariant).
func TestVariantHeader_MatchesPolicy(t *testing.T) {
	if variantHeader != policy.HeaderVariant {
		t.Fatalf("variantHeader %q must equal policy.HeaderVariant %q", variantHeader, policy.HeaderVariant)
	}
}
