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
	"testing"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
)

func TestParseExecutionResult_NilItem(t *testing.T) {
	_, err := ParseExecutionResult(nil)
	if err != ErrNilWorkItem {
		t.Errorf("ParseExecutionResult(nil) error = %v, want %v", err, ErrNilWorkItem)
	}
}

func TestParseExecutionResult_NoResult(t *testing.T) {
	now := time.Now()
	completed := now.Add(time.Second)

	item := &queue.WorkItem{
		ID:          "item-1",
		ScenarioID:  "scenario-1",
		ProviderID:  "provider-1",
		Status:      queue.ItemStatusCompleted,
		StartedAt:   &now,
		CompletedAt: &completed,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
	if result.WorkItemID != "item-1" {
		t.Errorf("WorkItemID = %s, want item-1", result.WorkItemID)
	}
	if result.ScenarioID != "scenario-1" {
		t.Errorf("ScenarioID = %s, want scenario-1", result.ScenarioID)
	}
	if result.Duration != time.Second {
		t.Errorf("Duration = %v, want 1s", result.Duration)
	}
}

func TestParseExecutionResult_FailedItem(t *testing.T) {
	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusFailed,
		Error:  "connection timeout",
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusFail {
		t.Errorf("Status = %s, want %s", result.Status, StatusFail)
	}
	if result.Error != "connection timeout" {
		t.Errorf("Error = %s, want 'connection timeout'", result.Error)
	}
}

func TestParseExecutionResult_JSONResult(t *testing.T) {
	jsonData := []byte(`{
		"status": "pass",
		"durationMs": 1500,
		"metrics": {
			"tokens": 100,
			"cost": 0.05,
			"latency_ms": 250
		},
		"assertions": [
			{"name": "check_output", "passed": true},
			{"name": "check_format", "passed": false, "message": "invalid format"}
		]
	}`)

	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: jsonData,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
	if result.Duration != 1500*time.Millisecond {
		t.Errorf("Duration = %v, want 1.5s", result.Duration)
	}
	if result.Metrics["tokens"] != 100 {
		t.Errorf("Metrics[tokens] = %v, want 100", result.Metrics["tokens"])
	}
	if result.Metrics["cost"] != 0.05 {
		t.Errorf("Metrics[cost] = %v, want 0.05", result.Metrics["cost"])
	}
	if len(result.Assertions) != 2 {
		t.Errorf("Assertions count = %d, want 2", len(result.Assertions))
	}
	if !result.Assertions[0].Passed {
		t.Error("First assertion should have passed")
	}
	if result.Assertions[1].Passed {
		t.Error("Second assertion should have failed")
	}
}

func TestParseExecutionResult_JSONWithDurationString(t *testing.T) {
	jsonData := []byte(`{
		"status": "pass",
		"duration": "2s"
	}`)

	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: jsonData,
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	if result.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", result.Duration)
	}
}

func TestParseExecutionResult_InvalidJSON(t *testing.T) {
	item := &queue.WorkItem{
		ID:     "item-1",
		Status: queue.ItemStatusCompleted,
		Result: []byte("not valid json"),
	}

	result, err := ParseExecutionResult(item)
	if err != nil {
		t.Fatalf("ParseExecutionResult() error = %v", err)
	}

	// Should still return a result based on item status
	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
}

func TestParseJUnitXML_Empty(t *testing.T) {
	_, err := ParseJUnitXML(nil)
	if err != ErrEmptyResult {
		t.Errorf("ParseJUnitXML(nil) error = %v, want %v", err, ErrEmptyResult)
	}

	_, err = ParseJUnitXML([]byte{})
	if err != ErrEmptyResult {
		t.Errorf("ParseJUnitXML([]) error = %v, want %v", err, ErrEmptyResult)
	}
}

func TestParseJUnitXML_SingleSuite(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="3" failures="1" errors="0" skipped="0" time="1.5">
    <testcase classname="TestClass" name="test1" time="0.5"/>
    <testcase classname="TestClass" name="test2" time="0.5">
        <failure message="assertion failed">Expected X, got Y</failure>
    </testcase>
    <testcase classname="TestClass" name="test3" time="0.5"/>
</testsuite>`)

	result, err := ParseJUnitXML(xml)
	if err != nil {
		t.Fatalf("ParseJUnitXML() error = %v", err)
	}

	if result.Status != StatusFail {
		t.Errorf("Status = %s, want %s", result.Status, StatusFail)
	}
	if result.Metrics["tests"] != 3 {
		t.Errorf("Metrics[tests] = %v, want 3", result.Metrics["tests"])
	}
	if result.Metrics["failures"] != 1 {
		t.Errorf("Metrics[failures] = %v, want 1", result.Metrics["failures"])
	}
	if len(result.Assertions) != 3 {
		t.Errorf("Assertions count = %d, want 3", len(result.Assertions))
	}
	if result.Duration != 1500*time.Millisecond {
		t.Errorf("Duration = %v, want 1.5s", result.Duration)
	}
}

func TestParseJUnitXML_TestSuites(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
    <testsuite name="Suite1" tests="2" failures="0" errors="0" time="1.0">
        <testcase classname="Class1" name="test1" time="0.5"/>
        <testcase classname="Class1" name="test2" time="0.5"/>
    </testsuite>
    <testsuite name="Suite2" tests="1" failures="0" errors="0" time="0.5">
        <testcase classname="Class2" name="test3" time="0.5"/>
    </testsuite>
</testsuites>`)

	result, err := ParseJUnitXML(xml)
	if err != nil {
		t.Fatalf("ParseJUnitXML() error = %v", err)
	}

	if result.Status != StatusPass {
		t.Errorf("Status = %s, want %s", result.Status, StatusPass)
	}
	if result.Metrics["tests"] != 3 {
		t.Errorf("Metrics[tests] = %v, want 3", result.Metrics["tests"])
	}
	if len(result.Assertions) != 3 {
		t.Errorf("Assertions count = %d, want 3", len(result.Assertions))
	}
}

func TestParseJUnitXML_WithErrors(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="2" failures="0" errors="1" time="1.0">
    <testcase classname="TestClass" name="test1" time="0.5"/>
    <testcase classname="TestClass" name="test2" time="0.5">
        <error message="runtime error" type="Exception">stack trace</error>
    </testcase>
</testsuite>`)

	result, err := ParseJUnitXML(xml)
	if err != nil {
		t.Fatalf("ParseJUnitXML() error = %v", err)
	}

	if result.Status != StatusFail {
		t.Errorf("Status = %s, want %s", result.Status, StatusFail)
	}
	if result.Metrics["errors"] != 1 {
		t.Errorf("Metrics[errors] = %v, want 1", result.Metrics["errors"])
	}

	// Second assertion should have the error message
	if result.Assertions[1].Passed {
		t.Error("Second assertion should have failed")
	}
	if result.Assertions[1].Message != "runtime error" {
		t.Errorf("Assertion message = %s, want 'runtime error'", result.Assertions[1].Message)
	}
}

func TestParseJUnitXML_InvalidFormat(t *testing.T) {
	_, err := ParseJUnitXML([]byte("not xml"))
	if err != ErrInvalidFormat {
		t.Errorf("ParseJUnitXML() error = %v, want %v", err, ErrInvalidFormat)
	}

	// Valid XML but not JUnit format
	_, err = ParseJUnitXML([]byte("<root><child/></root>"))
	if err != ErrInvalidFormat {
		t.Errorf("ParseJUnitXML() error = %v, want %v", err, ErrInvalidFormat)
	}
}
