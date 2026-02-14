/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package snowflake implements an analytics SyncProvider that writes session
// data to a Snowflake data warehouse using incremental watermark-based sync.
package snowflake

import (
	"errors"
	"fmt"
)

// Default configuration values.
const (
	DefaultBatchSize = 1000
	DefaultSchema    = "PUBLIC"
)

// Config holds the Snowflake connection and sync settings.
type Config struct {
	// Account is the Snowflake account identifier (e.g. "org-account").
	Account string
	// User is the authentication username.
	User string
	// Password is the authentication password.
	Password string
	// Database is the target Snowflake database.
	Database string
	// Schema is the target schema within the database. Defaults to "PUBLIC".
	Schema string
	// Warehouse is the Snowflake compute warehouse to use.
	Warehouse string
	// Role is the Snowflake role to assume. Optional.
	Role string
	// DefaultBatchSize is the number of rows per sync batch. Defaults to 1000.
	DefaultBatchSize int
}

// Validate checks that required fields are set and applies defaults.
func (c *Config) Validate() error {
	if c.Account == "" {
		return errors.New("snowflake: account is required")
	}
	if c.User == "" {
		return errors.New("snowflake: user is required")
	}
	if c.Password == "" {
		return errors.New("snowflake: password is required")
	}
	if c.Database == "" {
		return errors.New("snowflake: database is required")
	}
	if c.Warehouse == "" {
		return errors.New("snowflake: warehouse is required")
	}
	if c.Schema == "" {
		c.Schema = DefaultSchema
	}
	if c.DefaultBatchSize <= 0 {
		c.DefaultBatchSize = DefaultBatchSize
	}
	return nil
}

// DSN returns the Snowflake connection string for use with gosnowflake.
func (c *Config) DSN() string {
	dsn := fmt.Sprintf("%s:%s@%s/%s/%s?warehouse=%s",
		c.User, c.Password, c.Account, c.Database, c.Schema, c.Warehouse)
	if c.Role != "" {
		dsn += "&role=" + c.Role
	}
	return dsn
}
