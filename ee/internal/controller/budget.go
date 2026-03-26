/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"fmt"
	"strconv"

	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// BudgetResult holds the outcome of a budget check.
type BudgetResult struct {
	// Breached is true when TotalCost exceeds the configured limit.
	Breached bool
	// Details contains summary key-value pairs for the ArenaJob status.
	// Populated only when Breached is true.
	Details map[string]string
}

// checkBudget compares the current cost accumulator against a budget limit.
// Returns a zero-value BudgetResult (Breached=false) when:
//   - budgetLimit is empty or nil
//   - budgetLimit cannot be parsed as a float64
//   - stats is nil
//   - cost is at or below the limit
func checkBudget(budgetLimit string, budgetCurrency string, stats *queue.JobStats) BudgetResult {
	if budgetLimit == "" {
		return BudgetResult{}
	}

	limit, err := strconv.ParseFloat(budgetLimit, 64)
	if err != nil {
		return BudgetResult{}
	}

	if stats == nil {
		return BudgetResult{}
	}

	if stats.TotalCost <= limit {
		return BudgetResult{}
	}

	return BudgetResult{
		Breached: true,
		Details: map[string]string{
			"budgetBreached": "true",
			"totalCost":      fmt.Sprintf("%.2f", stats.TotalCost),
			"budgetLimit":    fmt.Sprintf("%.2f", limit),
			"budgetCurrency": budgetCurrency,
		},
	}
}
