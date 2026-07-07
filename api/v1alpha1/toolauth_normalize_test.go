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

package v1alpha1

import "testing"

func toolAuthStrPtr(s string) *string { return &s }

func TestHandlerDefinition_EffectiveAuth(t *testing.T) {
	secret := &SecretKeySelector{Name: "creds", Key: "token"}

	tests := []struct {
		name    string
		handler HandlerDefinition
		want    *ToolAuth
	}{
		{
			name:    "no auth configured returns nil",
			handler: HandlerDefinition{HTTPConfig: &HTTPConfig{}},
			want:    nil,
		},
		{
			name: "explicit auth stanza wins verbatim",
			handler: HandlerDefinition{
				Auth: &ToolAuth{Type: ToolAuthTypeBasic, SecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeBasic, SecretRef: secret},
		},
		{
			name: "explicit stanza takes precedence over legacy fields",
			handler: HandlerDefinition{
				Auth:       &ToolAuth{Type: ToolAuthTypeNone},
				HTTPConfig: &HTTPConfig{AuthType: toolAuthStrPtr("bearer"), AuthSecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeNone},
		},
		{
			name: "legacy http bearer + secretRef normalizes",
			handler: HandlerDefinition{
				HTTPConfig: &HTTPConfig{AuthType: toolAuthStrPtr("bearer"), AuthSecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeBearer, SecretRef: secret},
		},
		{
			name: "legacy http basic normalizes",
			handler: HandlerDefinition{
				HTTPConfig: &HTTPConfig{AuthType: toolAuthStrPtr("basic"), AuthSecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeBasic, SecretRef: secret},
		},
		{
			name: "legacy secretRef without type defaults to bearer",
			handler: HandlerDefinition{
				HTTPConfig: &HTTPConfig{AuthSecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeBearer, SecretRef: secret},
		},
		{
			name: "legacy openapi normalizes",
			handler: HandlerDefinition{
				OpenAPIConfig: &OpenAPIConfig{AuthType: toolAuthStrPtr("bearer"), AuthSecretRef: secret},
			},
			want: &ToolAuth{Type: ToolAuthTypeBearer, SecretRef: secret},
		},
		{
			name:    "grpc handler with no legacy fields returns nil",
			handler: HandlerDefinition{GRPCConfig: &GRPCConfig{Endpoint: "svc:9000"}},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.handler.EffectiveAuth()
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("EffectiveAuth() = %+v, want nil", got)
			case tt.want == nil:
				return
			case got == nil:
				t.Fatalf("EffectiveAuth() = nil, want %+v", tt.want)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.want.Type)
			}
			if (got.SecretRef == nil) != (tt.want.SecretRef == nil) {
				t.Fatalf("SecretRef presence mismatch: got %v want %v", got.SecretRef, tt.want.SecretRef)
			}
			if got.SecretRef != nil && *got.SecretRef != *tt.want.SecretRef {
				t.Errorf("SecretRef = %+v, want %+v", *got.SecretRef, *tt.want.SecretRef)
			}
		})
	}
}
