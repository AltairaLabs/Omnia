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

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/runtime/conformance"
)

func TestExitCode(t *testing.T) {
	require.Equal(t, 0, exitCode(conformance.Result{Passed: true}))
	require.Equal(t, 1, exitCode(conformance.Result{Passed: false}))
}

func TestPrintResult_RendersTableAndSummary(t *testing.T) {
	res := conformance.Result{
		Passed: false,
		Checks: []conformance.CheckResult{
			{Name: "health/contract", Status: conformance.StatusPass, Detail: ""},
			{Name: "duplex-honesty", Status: conformance.StatusSkip, Detail: "duplex_audio not advertised"},
			{Name: "invoke-honesty", Status: conformance.StatusFail, Detail: "over-claims invoke"},
		},
	}
	var buf bytes.Buffer
	printResult(&buf, res)
	out := buf.String()

	require.Contains(t, out, "CHECK")
	require.Contains(t, out, "health/contract")
	require.Contains(t, out, "duplex-honesty")
	require.Contains(t, out, "invoke-honesty")
	require.Contains(t, out, "FAIL: runtime is not conformant")
}

func TestPrintResult_PassSummary(t *testing.T) {
	var buf bytes.Buffer
	printResult(&buf, conformance.Result{Passed: true})
	require.Contains(t, buf.String(), "PASS: runtime is conformant")
}
