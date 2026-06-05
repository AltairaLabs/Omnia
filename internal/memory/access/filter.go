/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package access

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
)

// DenyFilter evaluates a CEL deny expression over a memory item's metadata.
// An empty expression allows everything. Construction fails on a bad
// expression so the service refuses to serve rather than silently allow.
type DenyFilter struct {
	program cel.Program // nil ⇒ allow-all
}

// NewDenyFilter compiles a deny expression. The expression sees a single
// variable `metadata` (map<string, dyn>) and may use string extensions
// (e.g. .contains). Empty expr ⇒ allow-all filter.
func NewDenyFilter(expr string) (*DenyFilter, error) {
	if expr == "" {
		return &DenyFilter{}, nil
	}
	env, err := cel.NewEnv(
		cel.Variable("metadata", cel.MapType(cel.StringType, cel.DynType)),
		ext.Strings(),
	)
	if err != nil {
		return nil, fmt.Errorf("access: cel env: %w", err)
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("access: compile deny expr: %w", issues.Err())
	}
	prog, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("access: program: %w", err)
	}
	return &DenyFilter{program: prog}, nil
}

// Allowed reports whether an item with the given metadata may be returned.
// Fail-closed: any eval error or non-bool result denies the item.
func (f *DenyFilter) Allowed(metadata map[string]any) bool {
	if f.program == nil {
		return true
	}
	out, _, err := f.program.Eval(map[string]any{"metadata": metadata})
	if err != nil {
		return false // missing key / type error ⇒ deny
	}
	if out.Type() != types.BoolType {
		return false // non-bool result ⇒ deny
	}
	denied, ok := out.Value().(bool)
	if !ok {
		return false
	}
	return !denied
}
