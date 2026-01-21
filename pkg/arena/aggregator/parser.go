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

package aggregator

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
)

// Common errors returned by parser functions.
var (
	// ErrNilWorkItem is returned when a nil work item is passed to a parser.
	ErrNilWorkItem = errors.New("work item is nil")

	// ErrEmptyResult is returned when the work item has no result data.
	ErrEmptyResult = errors.New("work item has no result data")

	// ErrInvalidFormat is returned when the result data cannot be parsed.
	ErrInvalidFormat = errors.New("invalid result format")
)

// jsonResult represents the expected JSON structure from worker results.
type jsonResult struct {
	Status     string             `json:"status"`
	Error      string             `json:"error,omitempty"`
	DurationMs float64            `json:"durationMs,omitempty"`
	Duration   string             `json:"duration,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Assertions []struct {
		Name    string `json:"name"`
		Passed  bool   `json:"passed"`
		Message string `json:"message,omitempty"`
	} `json:"assertions,omitempty"`
}

// ParseExecutionResult parses a work item's result bytes into ExecutionResult.
// It supports JSON format and falls back to inferring status from work item state.
func ParseExecutionResult(item *queue.WorkItem) (*ExecutionResult, error) {
	if item == nil {
		return nil, ErrNilWorkItem
	}

	result := newResultFromWorkItem(item)

	// If no result data, infer status from work item status
	if len(item.Result) == 0 {
		inferStatusFromWorkItem(result, item)
		return result, nil
	}

	// Try to parse as JSON
	var jr jsonResult
	if err := json.Unmarshal(item.Result, &jr); err != nil {
		// If JSON parsing fails, still return a result based on item status
		inferStatusFromWorkItem(result, item)
		return result, nil
	}

	// Populate from JSON result
	populateResultFromJSON(result, &jr, item)
	return result, nil
}

// newResultFromWorkItem creates a new ExecutionResult with basic fields from a work item.
func newResultFromWorkItem(item *queue.WorkItem) *ExecutionResult {
	result := &ExecutionResult{
		WorkItemID: item.ID,
		ScenarioID: item.ScenarioID,
		ProviderID: item.ProviderID,
	}

	// Calculate duration from work item timestamps
	if item.StartedAt != nil && item.CompletedAt != nil {
		result.Duration = item.CompletedAt.Sub(*item.StartedAt)
	}

	return result
}

// inferStatusFromWorkItem sets the result status based on work item status.
func inferStatusFromWorkItem(result *ExecutionResult, item *queue.WorkItem) {
	switch item.Status {
	case queue.ItemStatusCompleted:
		result.Status = StatusPass
	case queue.ItemStatusFailed:
		result.Status = StatusFail
		result.Error = item.Error
	default:
		result.Status = StatusUnknown
	}
}

// populateResultFromJSON populates an ExecutionResult from a parsed JSON result.
func populateResultFromJSON(result *ExecutionResult, jr *jsonResult, item *queue.WorkItem) {
	// Set status from JSON or fall back to work item status
	result.Status = determineStatus(jr.Status, item.Status)

	// Set error from JSON or fall back to work item error
	if jr.Error != "" {
		result.Error = jr.Error
	} else if item.Error != "" {
		result.Error = item.Error
	}

	// Parse duration from JSON if not already set
	if result.Duration == 0 {
		result.Duration = parseDurationFromJSON(jr)
	}

	// Copy metrics
	copyMetrics(result, jr.Metrics)

	// Copy assertions
	copyAssertions(result, jr.Assertions)
}

// determineStatus returns the appropriate status string.
func determineStatus(jsonStatus string, itemStatus queue.ItemStatus) string {
	if jsonStatus != "" {
		return jsonStatus
	}
	if itemStatus == queue.ItemStatusCompleted {
		return StatusPass
	}
	return StatusFail
}

// parseDurationFromJSON extracts duration from JSON result fields.
func parseDurationFromJSON(jr *jsonResult) time.Duration {
	if jr.DurationMs > 0 {
		return time.Duration(jr.DurationMs) * time.Millisecond
	}
	if jr.Duration != "" {
		if d, err := time.ParseDuration(jr.Duration); err == nil {
			return d
		}
	}
	return 0
}

// copyMetrics copies metrics from JSON result to execution result.
func copyMetrics(result *ExecutionResult, metrics map[string]float64) {
	if len(metrics) == 0 {
		return
	}
	result.Metrics = make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result.Metrics[k] = v
	}
}

// copyAssertions copies assertions from JSON result to execution result.
func copyAssertions(result *ExecutionResult, assertions []struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}) {
	if len(assertions) == 0 {
		return
	}
	result.Assertions = make([]AssertionResult, len(assertions))
	for i, a := range assertions {
		result.Assertions[i] = AssertionResult{
			Name:    a.Name,
			Passed:  a.Passed,
			Message: a.Message,
		}
	}
}

// JUnitTestSuites represents a collection of JUnit test suites.
type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single JUnit test suite.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents a single JUnit test case.
type JUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
	Error     *JUnitError   `xml:"error,omitempty"`
	Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

// JUnitFailure represents a test case failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitError represents a test case error.
type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test case.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// ParseJUnitXML parses JUnit XML format into an ExecutionResult.
// This is useful when outputFormats includes "junit".
func ParseJUnitXML(data []byte) (*ExecutionResult, error) {
	if len(data) == 0 {
		return nil, ErrEmptyResult
	}

	// Try parsing as test suites first
	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.TestSuites) > 0 {
		return parseJUnitSuites(&suites)
	}

	// Try parsing as single test suite
	var suite JUnitTestSuite
	if err := xml.Unmarshal(data, &suite); err == nil && suite.Tests > 0 {
		return parseJUnitSuite(&suite)
	}

	return nil, ErrInvalidFormat
}

// parseJUnitSuites converts multiple test suites into an ExecutionResult.
func parseJUnitSuites(suites *JUnitTestSuites) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Metrics:    make(map[string]float64),
		Assertions: []AssertionResult{},
	}

	var totalTests, totalFailures, totalErrors int
	var totalTime float64

	for _, suite := range suites.TestSuites {
		totalTests += suite.Tests
		totalFailures += suite.Failures
		totalErrors += suite.Errors
		totalTime += suite.Time

		for _, tc := range suite.TestCases {
			assertion := AssertionResult{
				Name:   tc.ClassName + "." + tc.Name,
				Passed: tc.Failure == nil && tc.Error == nil,
			}
			if tc.Failure != nil {
				assertion.Message = tc.Failure.Message
			} else if tc.Error != nil {
				assertion.Message = tc.Error.Message
			}
			result.Assertions = append(result.Assertions, assertion)
		}
	}

	if totalFailures > 0 || totalErrors > 0 {
		result.Status = StatusFail
	} else {
		result.Status = StatusPass
	}

	result.Duration = time.Duration(totalTime * float64(time.Second))
	result.Metrics["tests"] = float64(totalTests)
	result.Metrics["failures"] = float64(totalFailures)
	result.Metrics["errors"] = float64(totalErrors)

	return result, nil
}

// parseJUnitSuite converts a single test suite into an ExecutionResult.
func parseJUnitSuite(suite *JUnitTestSuite) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Metrics:    make(map[string]float64),
		Assertions: make([]AssertionResult, 0, len(suite.TestCases)),
	}

	for _, tc := range suite.TestCases {
		assertion := AssertionResult{
			Name:   tc.ClassName + "." + tc.Name,
			Passed: tc.Failure == nil && tc.Error == nil,
		}
		if tc.Failure != nil {
			assertion.Message = tc.Failure.Message
		} else if tc.Error != nil {
			assertion.Message = tc.Error.Message
		}
		result.Assertions = append(result.Assertions, assertion)
	}

	if suite.Failures > 0 || suite.Errors > 0 {
		result.Status = StatusFail
	} else {
		result.Status = StatusPass
	}

	result.Duration = time.Duration(suite.Time * float64(time.Second))
	result.Metrics["tests"] = float64(suite.Tests)
	result.Metrics["failures"] = float64(suite.Failures)
	result.Metrics["errors"] = float64(suite.Errors)
	result.Metrics["skipped"] = float64(suite.Skipped)

	return result, nil
}
