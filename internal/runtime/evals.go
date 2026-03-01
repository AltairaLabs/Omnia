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

package runtime

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/sdk"
)

// packEvalFields is the subset of a prompt pack needed to extract eval definitions.
type packEvalFields struct {
	Evals   []evals.EvalDef          `json:"evals"`
	Prompts map[string]promptEvalDef `json:"prompts"`
}

// promptEvalDef extracts eval definitions from individual prompts.
type promptEvalDef struct {
	Evals []evals.EvalDef `json:"evals"`
}

// LoadPackEvalDefs reads a compiled .pack.json file and returns the pack-level eval definitions.
func LoadPackEvalDefs(packPath string) ([]evals.EvalDef, error) {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pack file: %w", err)
	}

	var pack packEvalFields
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("failed to parse pack file: %w", err)
	}

	return pack.Evals, nil
}

// LoadAllEvalDefs reads a compiled .pack.json file and returns all eval definitions
// (both pack-level and prompt-level). Use this for validation at startup.
func LoadAllEvalDefs(packPath string) ([]evals.EvalDef, error) {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pack file: %w", err)
	}

	var pack packEvalFields
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("failed to parse pack file: %w", err)
	}

	all := make([]evals.EvalDef, 0, len(pack.Evals))
	all = append(all, pack.Evals...)

	for _, prompt := range pack.Prompts {
		all = append(all, prompt.Evals...)
	}

	return all, nil
}

// ValidateEvalDefs checks that every eval type in the given definitions has a
// registered handler. It returns a list of types that are NOT registered.
// Call this at startup to surface configuration mismatches early.
func ValidateEvalDefs(defs []evals.EvalDef) []string {
	registry := evals.NewEvalTypeRegistry()
	var missing []string
	seen := make(map[string]bool)
	for _, d := range defs {
		if seen[d.Type] {
			continue
		}
		seen[d.Type] = true
		if !registry.Has(d.Type) {
			missing = append(missing, d.Type)
		}
	}
	return missing
}

// buildEvalOptions builds SDK options for eval middleware when a collector is configured.
func (s *Server) buildEvalOptions() []sdk.Option {
	if s.evalCollector == nil {
		s.log.V(1).Info("eval options skipped", "reason", "no collector")
		return nil
	}

	registry := evals.NewEvalTypeRegistry()
	runner := evals.NewEvalRunner(registry)
	metricWriter := evals.NewMetricResultWriter(s.evalCollector, s.evalDefs)

	var writers []evals.ResultWriter
	writers = append(writers, metricWriter)
	if s.evalMetrics != nil {
		promWriter := NewPrometheusResultWriter(s.evalMetrics, s.evalDefs, s.log)
		writers = append(writers, promWriter)
	}
	compositeWriter := evals.NewCompositeResultWriter(writers...)

	dispatcher := evals.NewInProcDispatcher(runner, compositeWriter)

	s.log.V(1).Info("eval options built",
		"evalDefCount", len(s.evalDefs),
		"registeredTypes", registry.Types(),
		"hasDispatcher", dispatcher != nil,
		"hasMetricWriter", metricWriter != nil,
		"hasEvalMetrics", s.evalMetrics != nil,
		"writerCount", len(writers))

	return []sdk.Option{
		sdk.WithEvalDispatcher(dispatcher),
		sdk.WithResultWriters(compositeWriter),
	}
}
