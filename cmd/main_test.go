/*
Copyright 2026 Altaira Labs.

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

	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/altairalabs/omnia/internal/controller"
)

func TestPolicyBrokerImageForEnterprise(t *testing.T) {
	tests := []struct {
		name       string
		enterprise bool
		image      string
		want       string
	}{
		{
			name:       "enterprise disabled ignores image",
			enterprise: false,
			image:      "custom:v1",
			want:       "",
		},
		{
			name:       "enterprise disabled with empty image",
			enterprise: false,
			image:      "",
			want:       "",
		},
		{
			name:       "enterprise enabled with custom image",
			enterprise: true,
			image:      "custom:v1",
			want:       "custom:v1",
		},
		{
			name:       "enterprise enabled with empty image uses default",
			enterprise: true,
			image:      "",
			want:       controller.DefaultPolicyBrokerImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policyBrokerImageForEnterprise(tt.enterprise, tt.image)
			if got != tt.want {
				t.Errorf("policyBrokerImageForEnterprise(%v, %q) = %q, want %q",
					tt.enterprise, tt.image, got, tt.want)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"   ", []string{}},
		{"a", []string{"a"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitAndTrim(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitAndTrim(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("splitAndTrim(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestSchemeKnowsGatewayAPI(t *testing.T) {
	gvBase := schema.GroupVersion{Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version}
	if !scheme.Recognizes(gvBase.WithKind("HTTPRoute")) {
		t.Fatal("scheme does not recognize HTTPRoute")
	}
	if !scheme.Recognizes(gvBase.WithKind("Gateway")) {
		t.Fatal("scheme does not recognize Gateway")
	}
}
