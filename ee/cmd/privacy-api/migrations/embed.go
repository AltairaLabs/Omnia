/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package migrations contains embedded SQL migrations for the privacy-api database.
package migrations

import "embed"

// MigrationsFS contains all SQL migration files embedded at compile time.
//
//go:embed *.sql
var MigrationsFS embed.FS
