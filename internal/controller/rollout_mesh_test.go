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
)

func meshCfg() *omniav1alpha1.MeshTrafficRouting {
	return &omniav1alpha1.MeshTrafficRouting{StableSubset: "stable", CandidateSubset: "canary"}
}

func TestBuildOwnedDestinationRule_Subsets(t *testing.T) {
	dr := buildOwnedDestinationRule("agent1", "ns1", "agent1.ns1.svc.cluster.local", meshCfg())
	subsets, _, _ := unstructured.NestedSlice(dr.Object, "spec", "subsets")
	if len(subsets) != 2 {
		t.Fatalf("want 2 subsets, got %d", len(subsets))
	}
	first := subsets[0].(map[string]interface{})
	labels, _, _ := unstructured.NestedStringMap(first, "labels")
	if labels["track"] != "stable" {
		t.Fatalf("stable subset must select track=stable, got %v", labels)
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
	if weights["stable"] != 70 || weights["canary"] != 30 {
		t.Fatalf("want stable=70 canary=30, got %v", weights)
	}
}
