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
	"github.com/altairalabs/omnia/pkg/metrics"
)

// Metrics is an alias to the shared LLM metrics.
// These metrics track LLM usage for cost analysis and monitoring.
type Metrics = metrics.LLMMetrics

// NewMetrics creates and registers all Prometheus metrics for the runtime.
func NewMetrics(agentName, namespace string) *Metrics {
	return metrics.NewLLMMetrics(metrics.LLMMetricsConfig{
		AgentName: agentName,
		Namespace: namespace,
	})
}

// NoOpMetrics is a no-op implementation for when metrics are disabled.
type NoOpMetrics = metrics.NoOpLLMMetrics

// RuntimeMetrics is an alias to the shared runtime metrics for tool/pipeline tracking.
type RuntimeMetrics = metrics.RuntimeMetrics

// NewRuntimeMetrics creates and registers Prometheus metrics for runtime operations.
func NewRuntimeMetrics(agentName, namespace string) *RuntimeMetrics {
	return metrics.NewRuntimeMetrics(metrics.RuntimeMetricsConfig{
		AgentName: agentName,
		Namespace: namespace,
	})
}

// NoOpRuntimeMetrics is a no-op implementation for when runtime metrics are disabled.
type NoOpRuntimeMetrics = metrics.NoOpRuntimeMetrics
