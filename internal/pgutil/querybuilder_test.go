/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package pgutil

import (
	"testing"
)

func TestQueryBuilder_Add(t *testing.T) {
	qb := &QueryBuilder{}
	qb.Add("name=$?", "alice")
	qb.Add("age > $?", 30)

	if len(qb.Args()) != 2 {
		t.Fatalf("expected 2 args, got %d", len(qb.Args()))
	}
	if qb.Args()[0] != "alice" {
		t.Errorf("expected arg[0]=%q, got %v", "alice", qb.Args()[0])
	}
	if qb.Args()[1] != 30 {
		t.Errorf("expected arg[1]=%d, got %v", 30, qb.Args()[1])
	}
}

func TestQueryBuilder_Where_Empty(t *testing.T) {
	qb := &QueryBuilder{}
	if got := qb.Where(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestQueryBuilder_Where_SingleClause(t *testing.T) {
	qb := &QueryBuilder{}
	qb.Add("status=$?", "active")

	want := " AND status=$1"
	if got := qb.Where(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestQueryBuilder_Where_MultipleClauses(t *testing.T) {
	qb := &QueryBuilder{}
	qb.Add("a=$?", 1)
	qb.Add("b=$?", 2)
	qb.Add("c=$?", 3)

	want := " AND a=$1 AND b=$2 AND c=$3"
	if got := qb.Where(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestQueryBuilder_SetArgs(t *testing.T) {
	qb := &QueryBuilder{}
	existing := []any{"pre-existing"}
	qb.SetArgs(existing)
	qb.Add("x=$?", "val")

	if len(qb.Args()) != 2 {
		t.Fatalf("expected 2 args, got %d", len(qb.Args()))
	}
	if qb.Args()[0] != "pre-existing" {
		t.Errorf("expected first arg to be pre-existing, got %v", qb.Args()[0])
	}

	want := " AND x=$2"
	if got := qb.Where(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestQueryBuilder_AppendPagination_Both(t *testing.T) {
	qb := &QueryBuilder{}
	qb.Add("id=$?", 1)

	result := qb.AppendPagination("SELECT * FROM t WHERE 1=1"+qb.Where(), 10, 20)
	want := "SELECT * FROM t WHERE 1=1 AND id=$1 LIMIT $2 OFFSET $3"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
	if len(qb.Args()) != 3 {
		t.Fatalf("expected 3 args, got %d", len(qb.Args()))
	}
	if qb.Args()[1] != 10 {
		t.Errorf("expected limit arg=10, got %v", qb.Args()[1])
	}
	if qb.Args()[2] != 20 {
		t.Errorf("expected offset arg=20, got %v", qb.Args()[2])
	}
}

func TestQueryBuilder_AppendPagination_LimitOnly(t *testing.T) {
	qb := &QueryBuilder{}
	result := qb.AppendPagination("SELECT * FROM t", 5, 0)
	want := "SELECT * FROM t LIMIT $1"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
	if len(qb.Args()) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(qb.Args()))
	}
}

func TestQueryBuilder_AppendPagination_OffsetOnly(t *testing.T) {
	qb := &QueryBuilder{}
	result := qb.AppendPagination("SELECT * FROM t", 0, 10)
	want := "SELECT * FROM t OFFSET $1"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestQueryBuilder_AppendPagination_Neither(t *testing.T) {
	qb := &QueryBuilder{}
	result := qb.AppendPagination("SELECT * FROM t", 0, 0)
	want := "SELECT * FROM t"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
	if len(qb.Args()) != 0 {
		t.Errorf("expected 0 args, got %d", len(qb.Args()))
	}
}
