/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

func TestCheckBudget_UnderLimit(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 30.00}
	result := checkBudget("50.00", "USD", stats)

	if result.Breached {
		t.Fatal("expected no breach when cost is under limit")
	}
	if result.Details != nil {
		t.Fatal("expected nil details when not breached")
	}
}

func TestCheckBudget_OverLimit(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 55.50}
	result := checkBudget("50.00", "USD", stats)

	if !result.Breached {
		t.Fatal("expected breach when cost exceeds limit")
	}
	if result.Details["budgetBreached"] != "true" {
		t.Fatalf("expected budgetBreached=true, got %q", result.Details["budgetBreached"])
	}
	if result.Details["totalCost"] != "55.50" {
		t.Fatalf("expected totalCost=55.50, got %q", result.Details["totalCost"])
	}
	if result.Details["budgetLimit"] != "50.00" {
		t.Fatalf("expected budgetLimit=50.00, got %q", result.Details["budgetLimit"])
	}
	if result.Details["budgetCurrency"] != "USD" {
		t.Fatalf("expected budgetCurrency=USD, got %q", result.Details["budgetCurrency"])
	}
}

func TestCheckBudget_ExactlyAtLimit(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 50.00}
	result := checkBudget("50.00", "USD", stats)

	if result.Breached {
		t.Fatal("expected no breach when cost equals limit")
	}
}

func TestCheckBudget_NoBudgetConfigured(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 999.99}
	result := checkBudget("", "USD", stats)

	if result.Breached {
		t.Fatal("expected no breach when budget is empty")
	}
}

func TestCheckBudget_InvalidBudgetString(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 100.00}
	result := checkBudget("not-a-number", "USD", stats)

	if result.Breached {
		t.Fatal("expected no breach when budget string is invalid")
	}
}

func TestCheckBudget_NilStats(t *testing.T) {
	result := checkBudget("50.00", "USD", nil)

	if result.Breached {
		t.Fatal("expected no breach when stats is nil")
	}
}

func TestCheckBudget_ZeroCost(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 0}
	result := checkBudget("50.00", "USD", stats)

	if result.Breached {
		t.Fatal("expected no breach when cost is zero")
	}
}

func TestCheckBudget_NonUSDCurrency(t *testing.T) {
	stats := &queue.JobStats{TotalCost: 120.00}
	result := checkBudget("100.00", "EUR", stats)

	if !result.Breached {
		t.Fatal("expected breach")
	}
	if result.Details["budgetCurrency"] != "EUR" {
		t.Fatalf("expected budgetCurrency=EUR, got %q", result.Details["budgetCurrency"])
	}
}
