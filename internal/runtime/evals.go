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

// buildEvalOptions builds SDK options for eval middleware when a collector is configured.
func (s *Server) buildEvalOptions() []sdk.Option {
	if s.evalCollector == nil {
		return nil
	}

	registry := evals.NewEvalTypeRegistry()
	runner := evals.NewEvalRunner(registry)
	metricWriter := evals.NewMetricResultWriter(s.evalCollector, s.evalDefs)
	dispatcher := evals.NewInProcDispatcher(runner, metricWriter)

	return []sdk.Option{
		sdk.WithEvalDispatcher(dispatcher),
		sdk.WithResultWriters(metricWriter),
	}
}
