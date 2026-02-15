/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import "sync"

// BudgetTracker tracks per-namespace spend and enforces budget limits
// for judge LLM evaluations. When a namespace's accumulated spend reaches
// or exceeds its budget, the namespace is paused and no further judge
// evaluations should be run until the budget is reset.
type BudgetTracker struct {
	mu      sync.RWMutex
	spent   map[string]float64 // namespace -> total spend USD
	budgets map[string]float64 // namespace -> budget limit USD
	paused  map[string]bool    // namespace -> paused flag
}

// NewBudgetTracker creates a BudgetTracker with no budgets configured.
func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{
		spent:   make(map[string]float64),
		budgets: make(map[string]float64),
		paused:  make(map[string]bool),
	}
}

// SetBudget configures the spending limit for a namespace.
// A limit of 0 disables budget enforcement for that namespace.
func (b *BudgetTracker) SetBudget(namespace string, limitUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.budgets[namespace] = limitUSD
	b.checkAndUpdatePause(namespace)
}

// RecordSpend records a cost against a namespace and pauses it if the
// budget is exceeded.
func (b *BudgetTracker) RecordSpend(namespace string, costUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.spent[namespace] += costUSD
	b.checkAndUpdatePause(namespace)
}

// IsPaused returns true if the namespace has exceeded its budget.
// Returns false for namespaces with no budget configured.
func (b *BudgetTracker) IsPaused(namespace string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.paused[namespace]
}

// GetSpent returns the total spend for a namespace.
func (b *BudgetTracker) GetSpent(namespace string) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.spent[namespace]
}

// GetBudget returns the configured budget for a namespace.
// Returns 0 if no budget is set.
func (b *BudgetTracker) GetBudget(namespace string) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.budgets[namespace]
}

// Reset clears the spend and paused state for a namespace,
// typically used for periodic budget resets.
func (b *BudgetTracker) Reset(namespace string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.spent[namespace] = 0
	b.paused[namespace] = false
}

// checkAndUpdatePause evaluates whether a namespace should be paused.
// Must be called with b.mu held for writing.
func (b *BudgetTracker) checkAndUpdatePause(namespace string) {
	limit, hasLimit := b.budgets[namespace]
	if !hasLimit || limit <= 0 {
		b.paused[namespace] = false
		return
	}
	b.paused[namespace] = b.spent[namespace] >= limit
}
