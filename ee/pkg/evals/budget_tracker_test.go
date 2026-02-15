/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"sync"
	"testing"
)

func TestBudgetTracker_NoBudget(t *testing.T) {
	bt := NewBudgetTracker()

	// No budget set: should never be paused.
	if bt.IsPaused("default") {
		t.Fatal("expected not paused when no budget is set")
	}

	bt.RecordSpend("default", 100.0)
	if bt.IsPaused("default") {
		t.Fatal("expected not paused when no budget is set, even with spend")
	}
}

func TestBudgetTracker_SetBudgetAndEnforce(t *testing.T) {
	bt := NewBudgetTracker()

	bt.SetBudget("prod", 10.0)

	// Under budget.
	bt.RecordSpend("prod", 5.0)
	if bt.IsPaused("prod") {
		t.Fatal("expected not paused when under budget")
	}
	if got := bt.GetSpent("prod"); got != 5.0 {
		t.Fatalf("expected spent=5.0, got %v", got)
	}

	// At budget.
	bt.RecordSpend("prod", 5.0)
	if !bt.IsPaused("prod") {
		t.Fatal("expected paused when at budget")
	}
	if got := bt.GetSpent("prod"); got != 10.0 {
		t.Fatalf("expected spent=10.0, got %v", got)
	}
}

func TestBudgetTracker_ExceedBudget(t *testing.T) {
	bt := NewBudgetTracker()

	bt.SetBudget("staging", 5.0)
	bt.RecordSpend("staging", 7.5)

	if !bt.IsPaused("staging") {
		t.Fatal("expected paused when over budget")
	}
}

func TestBudgetTracker_Reset(t *testing.T) {
	bt := NewBudgetTracker()

	bt.SetBudget("prod", 10.0)
	bt.RecordSpend("prod", 15.0)

	if !bt.IsPaused("prod") {
		t.Fatal("expected paused after exceeding budget")
	}

	bt.Reset("prod")

	if bt.IsPaused("prod") {
		t.Fatal("expected not paused after reset")
	}
	if got := bt.GetSpent("prod"); got != 0 {
		t.Fatalf("expected spent=0 after reset, got %v", got)
	}
}

func TestBudgetTracker_ZeroBudget(t *testing.T) {
	bt := NewBudgetTracker()

	// Setting budget to 0 disables enforcement.
	bt.SetBudget("test", 0)
	bt.RecordSpend("test", 100.0)

	if bt.IsPaused("test") {
		t.Fatal("expected not paused when budget is 0 (disabled)")
	}
}

func TestBudgetTracker_NegativeBudget(t *testing.T) {
	bt := NewBudgetTracker()

	// Negative budget should be treated like disabled.
	bt.SetBudget("test", -1.0)
	bt.RecordSpend("test", 100.0)

	if bt.IsPaused("test") {
		t.Fatal("expected not paused when budget is negative (disabled)")
	}
}

func TestBudgetTracker_MultipleNamespaces(t *testing.T) {
	bt := NewBudgetTracker()

	bt.SetBudget("ns-a", 10.0)
	bt.SetBudget("ns-b", 20.0)

	bt.RecordSpend("ns-a", 15.0) // exceeds
	bt.RecordSpend("ns-b", 15.0) // under

	if !bt.IsPaused("ns-a") {
		t.Fatal("expected ns-a paused")
	}
	if bt.IsPaused("ns-b") {
		t.Fatal("expected ns-b not paused")
	}
}

func TestBudgetTracker_GetBudget(t *testing.T) {
	bt := NewBudgetTracker()

	if got := bt.GetBudget("missing"); got != 0 {
		t.Fatalf("expected 0 for missing namespace, got %v", got)
	}

	bt.SetBudget("prod", 50.0)
	if got := bt.GetBudget("prod"); got != 50.0 {
		t.Fatalf("expected 50.0, got %v", got)
	}
}

func TestBudgetTracker_SetBudgetUnpauses(t *testing.T) {
	bt := NewBudgetTracker()

	bt.SetBudget("prod", 10.0)
	bt.RecordSpend("prod", 10.0)

	if !bt.IsPaused("prod") {
		t.Fatal("expected paused")
	}

	// Increase the budget to unblock.
	bt.SetBudget("prod", 20.0)
	if bt.IsPaused("prod") {
		t.Fatal("expected not paused after budget increase")
	}
}

func TestBudgetTracker_ConcurrentAccess(t *testing.T) {
	bt := NewBudgetTracker()
	bt.SetBudget("concurrent", 1000.0)

	var wg sync.WaitGroup
	goroutines := 100
	spendPerGoroutine := 1.0

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			bt.RecordSpend("concurrent", spendPerGoroutine)
			_ = bt.IsPaused("concurrent")
			_ = bt.GetSpent("concurrent")
		}()
	}
	wg.Wait()

	expected := float64(goroutines) * spendPerGoroutine
	got := bt.GetSpent("concurrent")
	if !almostEqual(got, expected) {
		t.Fatalf("expected spent=%v, got %v", expected, got)
	}
}

func TestBudgetTracker_ConcurrentResetAndSpend(t *testing.T) {
	bt := NewBudgetTracker()
	bt.SetBudget("race", 100.0)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			bt.RecordSpend("race", 1.0)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			bt.Reset("race")
		}
	}()

	wg.Wait()

	// Just verify no panics from concurrent access.
	_ = bt.GetSpent("race")
	_ = bt.IsPaused("race")
}

func TestBudgetTracker_ResetNonexistent(t *testing.T) {
	bt := NewBudgetTracker()

	// Should not panic.
	bt.Reset("nonexistent")

	if bt.IsPaused("nonexistent") {
		t.Fatal("expected not paused for nonexistent namespace")
	}
	if got := bt.GetSpent("nonexistent"); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestBudgetTracker_IncrementalSpend(t *testing.T) {
	bt := NewBudgetTracker()
	bt.SetBudget("ns", 5.0)

	bt.RecordSpend("ns", 1.0)
	bt.RecordSpend("ns", 1.5)
	bt.RecordSpend("ns", 2.0)

	if bt.IsPaused("ns") {
		t.Fatal("expected not paused at 4.5")
	}

	bt.RecordSpend("ns", 0.5) // total = 5.0
	if !bt.IsPaused("ns") {
		t.Fatal("expected paused at 5.0")
	}
}
