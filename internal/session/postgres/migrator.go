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

package postgres

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // Postgres driver for migrate
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrator manages PostgreSQL schema migrations using embedded SQL files.
type Migrator struct {
	m      *migrate.Migrate
	logger logr.Logger
}

// NewMigrator creates a new Migrator from a PostgreSQL connection string.
// The connection string should be a valid PostgreSQL URL, e.g.:
// "postgres://user:pass@host:5432/dbname?sslmode=disable"
func NewMigrator(connString string, logger logr.Logger) (*Migrator, error) {
	source, err := iofs.New(MigrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, connString)
	if err != nil {
		return nil, fmt.Errorf("creating migrator: %w", err)
	}

	return &Migrator{m: m, logger: logger}, nil
}

// Up applies all pending migrations.
func (mg *Migrator) Up() error {
	mg.logger.Info("applying migrations")
	if err := mg.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}
	v, dirty, _ := mg.m.Version()
	mg.logger.Info("migrations applied", "version", v, "dirty", dirty)
	return nil
}

// Down rolls back all migrations.
func (mg *Migrator) Down() error {
	mg.logger.Info("rolling back all migrations")
	if err := mg.m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rolling back migrations: %w", err)
	}
	return nil
}

// Version returns the current migration version and dirty state.
// Returns 0 and false if no migrations have been applied.
func (mg *Migrator) Version() (uint, bool, error) {
	v, dirty, err := mg.m.Version()
	if err != nil && errors.Is(err, migrate.ErrNoChange) {
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
