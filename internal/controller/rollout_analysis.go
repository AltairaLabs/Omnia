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
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// analysisResult represents the outcome of running a RolloutAnalysis template.
type analysisResult struct {
	passed  bool
	message string
}

// conditionPattern matches expressions like "result[0] >= 0.9".
var conditionPattern = regexp.MustCompile(`^result\[\d+\]\s*(>=|<=|>|<|==|!=)\s*(-?\d+\.?\d*)$`)

// runAnalysis fetches the RolloutAnalysis template, runs all metrics,
// and returns whether the analysis passed.
func (r *AgentRuntimeReconciler) runAnalysis(
	ctx context.Context,
	namespace string,
	analysisStep *omniav1alpha1.RolloutAnalysisStep,
) (analysisResult, error) {
	log := logf.FromContext(ctx)

	template, err := r.fetchAnalysisTemplate(ctx, namespace, analysisStep.TemplateName)
	if err != nil {
		return analysisResult{}, fmt.Errorf("fetch analysis template %q: %w", analysisStep.TemplateName, err)
	}

	args := mergeArgs(template.args, analysisStep.Args)

	var failedMetrics []string
	for _, metric := range template.metrics {
		passed, err := r.evaluateMetric(ctx, metric, args)
		if err != nil {
			log.Error(err, "metric evaluation error", "metric", metric.name)
			return analysisResult{}, fmt.Errorf("evaluate metric %q: %w", metric.name, err)
		}
		if !passed {
			failedMetrics = append(failedMetrics, metric.name)
			log.V(1).Info("metric failed", "metric", metric.name)
		} else {
			log.V(1).Info("metric passed", "metric", metric.name)
		}
	}

	if len(failedMetrics) > 0 {
		return analysisResult{
			passed:  false,
			message: fmt.Sprintf("failed metrics: %s", strings.Join(failedMetrics, ", ")),
		}, nil
	}

	return analysisResult{passed: true, message: "all metrics passed"}, nil
}

// analysisTemplate holds the parsed fields from a RolloutAnalysis CRD.
type analysisTemplate struct {
	args    map[string]string
	metrics []analysisMetric
}

// analysisMetric holds the parsed fields from an AnalysisMetric.
type analysisMetric struct {
	name             string
	failureLimit     int32
	successCondition string
	prometheusAddr   string
	prometheusQuery  string
}

// fetchAnalysisTemplate fetches a RolloutAnalysis CRD via unstructured client
// to avoid importing the EE API package.
func (r *AgentRuntimeReconciler) fetchAnalysisTemplate(
	ctx context.Context,
	namespace, name string,
) (analysisTemplate, error) {
	ra := &unstructured.Unstructured{}
	ra.SetAPIVersion("omnia.altairalabs.ai/v1alpha1")
	ra.SetKind("RolloutAnalysis")

	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := r.Get(ctx, key, ra); err != nil {
		if isNoMatchError(err) {
			return analysisTemplate{}, fmt.Errorf("RolloutAnalysis CRD not installed (EE feature)")
		}
		return analysisTemplate{}, err
	}

	return parseAnalysisTemplate(ra)
}

// parseAnalysisTemplate extracts typed fields from an unstructured RolloutAnalysis.
func parseAnalysisTemplate(ra *unstructured.Unstructured) (analysisTemplate, error) {
	result := analysisTemplate{
		args: make(map[string]string),
	}

	// Parse args
	args, found, err := unstructured.NestedSlice(ra.Object, "spec", "args")
	if err != nil {
		return result, fmt.Errorf("read spec.args: %w", err)
	}
	if found {
		for _, a := range args {
			argMap, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := argMap["name"].(string)
			value, _ := argMap["value"].(string)
			if name != "" {
				result.args[name] = value
			}
		}
	}

	// Parse metrics
	metrics, found, err := unstructured.NestedSlice(ra.Object, "spec", "metrics")
	if err != nil {
		return result, fmt.Errorf("read spec.metrics: %w", err)
	}
	if !found || len(metrics) == 0 {
		return result, fmt.Errorf("RolloutAnalysis has no metrics")
	}

	for _, m := range metrics {
		metricMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		parsed, err := parseMetric(metricMap)
		if err != nil {
			return result, err
		}
		result.metrics = append(result.metrics, parsed)
	}

	return result, nil
}

// parseMetric extracts a single metric definition from an unstructured map.
func parseMetric(m map[string]interface{}) (analysisMetric, error) {
	name, _ := m["name"].(string)
	if name == "" {
		return analysisMetric{}, fmt.Errorf("metric missing name")
	}

	successCondition, _ := m["successCondition"].(string)
	if successCondition == "" {
		return analysisMetric{}, fmt.Errorf("metric %q missing successCondition", name)
	}

	failureLimit := int32(0)
	if fl, ok := m["failureLimit"]; ok {
		failureLimit = toInt32(fl)
	}

	// Extract prometheus provider
	provider, ok := m["provider"].(map[string]interface{})
	if !ok {
		return analysisMetric{}, fmt.Errorf("metric %q missing provider", name)
	}

	prom, ok := provider["prometheus"].(map[string]interface{})
	if !ok {
		return analysisMetric{}, fmt.Errorf("metric %q: only prometheus provider is supported", name)
	}

	address, _ := prom["address"].(string)
	query, _ := prom["query"].(string)
	if address == "" || query == "" {
		return analysisMetric{}, fmt.Errorf("metric %q: prometheus address and query are required", name)
	}

	return analysisMetric{
		name:             name,
		failureLimit:     failureLimit,
		successCondition: successCondition,
		prometheusAddr:   address,
		prometheusQuery:  query,
	}, nil
}

// toInt32 converts an unstructured numeric value to int32.
func toInt32(v interface{}) int32 {
	switch n := v.(type) {
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	case int32:
		return n
	default:
		return 0
	}
}

// mergeArgs merges template default args with step-level overrides.
// Step args take precedence over template defaults.
func mergeArgs(templateDefaults map[string]string, stepArgs []omniav1alpha1.AnalysisArg) map[string]string {
	merged := make(map[string]string, len(templateDefaults))
	for k, v := range templateDefaults {
		merged[k] = v
	}
	for _, arg := range stepArgs {
		merged[arg.Name] = arg.Value
	}
	return merged
}

// substituteArgs replaces {{args.name}} placeholders in a query string.
func substituteArgs(query string, args map[string]string) string {
	for name, value := range args {
		query = strings.ReplaceAll(query, "{{args."+name+"}}", value)
	}
	return query
}

// evaluateMetric runs a single metric check: queries Prometheus and evaluates
// the success condition.
func (r *AgentRuntimeReconciler) evaluateMetric(
	ctx context.Context,
	metric analysisMetric,
	args map[string]string,
) (bool, error) {
	query := substituteArgs(metric.prometheusQuery, args)

	value, err := queryPrometheus(ctx, metric.prometheusAddr, query)
	if err != nil {
		return false, fmt.Errorf("prometheus query: %w", err)
	}

	passed, err := evaluateCondition(metric.successCondition, value)
	if err != nil {
		return false, fmt.Errorf("evaluate condition %q: %w", metric.successCondition, err)
	}

	return passed, nil
}

// queryPrometheus executes an instant PromQL query and returns the first result
// as a float64.
func queryPrometheus(ctx context.Context, address, query string) (float64, error) {
	client, err := promapi.NewClient(promapi.Config{Address: address})
	if err != nil {
		return 0, fmt.Errorf("create prometheus client: %w", err)
	}

	api := promv1.NewAPI(client)
	result, warnings, err := api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	if len(warnings) > 0 {
		log := logf.FromContext(ctx)
		log.V(1).Info("prometheus query warnings", "warnings", warnings)
	}

	return extractValue(result)
}

// extractValue extracts a float64 from a Prometheus query result.
// Supports Vector (first sample) and Scalar result types.
func extractValue(result model.Value) (float64, error) {
	switch v := result.(type) {
	case model.Vector:
		if len(v) == 0 {
			return 0, fmt.Errorf("empty vector result")
		}
		return float64(v[0].Value), nil
	case *model.Scalar:
		return float64(v.Value), nil
	default:
		return 0, fmt.Errorf("unsupported result type: %T", result)
	}
}

// evaluateCondition evaluates a success condition like "result[0] >= 0.9"
// against a query result value.
func evaluateCondition(condition string, value float64) (bool, error) {
	matches := conditionPattern.FindStringSubmatch(strings.TrimSpace(condition))
	if matches == nil {
		return false, fmt.Errorf("invalid condition format: %q (expected \"result[N] <op> <number>\")", condition)
	}

	operator := matches[1]
	threshold, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return false, fmt.Errorf("parse threshold %q: %w", matches[2], err)
	}

	return compareValues(value, operator, threshold)
}

// compareValues applies the operator to value and threshold.
func compareValues(value float64, operator string, threshold float64) (bool, error) {
	switch operator {
	case ">=":
		return value >= threshold, nil
	case "<=":
		return value <= threshold, nil
	case ">":
		return value > threshold, nil
	case "<":
		return value < threshold, nil
	case "==":
		return value == threshold, nil
	case "!=":
		return value != threshold, nil
	default:
		return false, fmt.Errorf("unsupported operator: %q", operator)
	}
}
