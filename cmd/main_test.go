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

	"github.com/altairalabs/omnia/internal/controller"
)

func TestPolicyProxyImageForEnterprise(t *testing.T) {
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
			want:       controller.DefaultPolicyProxyImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policyProxyImageForEnterprise(tt.enterprise, tt.image)
			if got != tt.want {
				t.Errorf("policyProxyImageForEnterprise(%v, %q) = %q, want %q",
					tt.enterprise, tt.image, got, tt.want)
			}
		})
	}
}
