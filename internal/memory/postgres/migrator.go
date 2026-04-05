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

// Package postgres provides PostgreSQL schema migrations for the memory database.
package postgres

import (
	"embed"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // Postgres driver for migrate
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrator manages PostgreSQL schema migrations for the memory database.
type Migrator struct {
	m      *migrate.Migrate
	logger logr.Logger
}

// NewMigrator creates a new Migrator from a PostgreSQL connection string.
// The connection string should be a valid PostgreSQL URL, e.g.:
// "postgres://user:pass@host:5432/dbname?sslmode=disable"
func NewMigrator(connStr string, log logr.Logger) (*Migrator, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, connStr)
	if err != nil {
		return nil, fmt.Errorf("creating migrator: %w", err)
	}

	return &Migrator{m: m, logger: log}, nil
}

// Up applies all pending migrations. If the database is in a dirty state
// (from a previously failed migration), it forces the version back to the
// last cleanly applied migration and retries.
func (mg *Migrator) Up() error {
	mg.logger.Info("applying memory migrations")

	// Check for dirty state before attempting migration.
	if v, dirty, err := mg.m.Version(); err == nil && dirty {
		// The failed migration was v, so the last clean version is v-1.
		// golang-migrate uses -1 (NilVersion) to represent "no version applied yet",
		// which is the correct state to force when version 1 failed.
		cleanVersion := int(v) - 1
		if cleanVersion < 1 {
			cleanVersion = -1 // NilVersion: reset to "no migrations applied"
		}
		mg.logger.Info("memory database is dirty, forcing version to last clean migration",
			"dirtyVersion", v, "forceVersion", cleanVersion)
		if err := mg.m.Force(cleanVersion); err != nil {
			return fmt.Errorf("forcing clean version %d: %w", cleanVersion, err)
		}
	}

	if err := mg.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}
	v, dirty, _ := mg.m.Version()
	mg.logger.Info("memory migrations applied", "version", v, "dirty", dirty)
	return nil
}

// Down rolls back all migrations.
func (mg *Migrator) Down() error {
	mg.logger.Info("rolling back all memory migrations")
	if err := mg.m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rolling back migrations: %w", err)
	}
	return nil
}

// Version returns the current migration version and dirty state.
// Returns 0 and false if no migrations have been applied.
func (mg *Migrator) Version() (uint, bool, error) {
	v, dirty, err := mg.m.Version()
	if err != nil && (errors.Is(err, migrate.ErrNoChange) || errors.Is(err, migrate.ErrNilVersion)) {
		return 0, false, nil
	}
	return v, dirty, err
}

// Close releases resources held by the migrator.
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	if srcErr != nil {
		return fmt.Errorf("closing source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("closing database: %w", dbErr)
	}
	return nil
}
