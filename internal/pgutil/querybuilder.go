/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

// Package pgutil provides shared PostgreSQL helpers: a parameterized query
// builder and nullable type conversion functions used across multiple packages.
package pgutil

import (
	"strconv"
	"strings"
)

// QueryBuilder accumulates parameterized WHERE clauses and their arguments.
// Use "$?" as a placeholder in clause strings; it is replaced with the
// positional parameter number (e.g. "$1", "$2") when Add is called.
type QueryBuilder struct {
	clauses []string
	args    []any
}

// Args returns the accumulated query arguments.
func (qb *QueryBuilder) Args() []any {
	return qb.args
}

// SetArgs replaces the internal argument slice. This is useful when a
// QueryBuilder must share arguments with a preceding builder (e.g. CTE +
// outer query).
func (qb *QueryBuilder) SetArgs(args []any) {
	qb.args = args
}

// Add appends a clause with a single argument. Every "$?" in clause is
// replaced with the next positional parameter number.
func (qb *QueryBuilder) Add(clause string, arg any) {
	qb.args = append(qb.args, arg)
	qb.clauses = append(qb.clauses, strings.ReplaceAll(clause, "$?", "$"+strconv.Itoa(len(qb.args))))
}

// Where returns the accumulated clauses joined with " AND " and prefixed
// with " AND ". If no clauses have been added, it returns an empty string.
// The caller is expected to include "WHERE 1=1" (or equivalent) before the
// returned fragment.
func (qb *QueryBuilder) Where() string {
	if len(qb.clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(qb.clauses, " AND ")
}

// AppendPagination appends LIMIT and OFFSET clauses to query when the
// respective values are greater than zero. Arguments are tracked internally.
func (qb *QueryBuilder) AppendPagination(query string, limit, offset int) string {
	if limit > 0 {
		qb.args = append(qb.args, limit)
		query += " LIMIT $" + strconv.Itoa(len(qb.args))
	}
	if offset > 0 {
		qb.args = append(qb.args, offset)
		query += " OFFSET $" + strconv.Itoa(len(qb.args))
	}
	return query
}
