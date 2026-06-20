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

package facade

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/pkg/policy"
)

func TestResolveVariant(t *testing.T) {
	const (
		candidate = "candidate"
		stable    = "stable"
	)
	tests := []struct {
		name     string
		header   string
		env      string
		expected string
	}{
		{name: "header wins over env", header: candidate, env: stable, expected: candidate},
		{name: "header only", header: stable, env: "", expected: stable},
		{name: "env fallback when header absent", header: "", env: candidate, expected: candidate},
		{name: "neither set", header: "", env: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envVariant, tt.env)
			r, err := http.NewRequest(http.MethodGet, "/ws", nil)
			assert.NoError(t, err)
			if tt.header != "" {
				r.Header.Set(policy.HeaderVariant, tt.header)
			}
			assert.Equal(t, tt.expected, resolveVariant(r))
		})
	}
}
