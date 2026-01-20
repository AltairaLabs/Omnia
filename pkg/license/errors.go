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

package license

import (
	"errors"
	"fmt"
)

// Common license validation errors.
var (
	// ErrLicenseNotFound indicates no license was found.
	ErrLicenseNotFound = errors.New("license not found")
	// ErrLicenseExpired indicates the license has expired.
	ErrLicenseExpired = errors.New("license has expired")
	// ErrLicenseInvalid indicates the license is invalid.
	ErrLicenseInvalid = errors.New("license is invalid")
	// ErrInvalidSignature indicates the JWT signature is invalid.
	ErrInvalidSignature = errors.New("invalid license signature")
)

// ValidationError represents a license validation failure with upgrade messaging.
type ValidationError struct {
	// Feature is the feature that failed validation.
	Feature string
	// Message is a human-readable error message.
	Message string
	// UpgradeURL is the URL to upgrade the license.
	UpgradeURL string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Message
}

// UpgradeMessage returns a message suggesting an upgrade.
func (e *ValidationError) UpgradeMessage() string {
	if e.UpgradeURL != "" {
		return fmt.Sprintf("%s. Upgrade to Enterprise at: %s", e.Message, e.UpgradeURL)
	}
	return fmt.Sprintf("%s. Contact sales to upgrade to Enterprise.", e.Message)
}

// Default upgrade URL.
const DefaultUpgradeURL = "https://altairalabs.ai/enterprise"

// NewSourceTypeError creates a validation error for source type restrictions.
func NewSourceTypeError(sourceType string) *ValidationError {
	return &ValidationError{
		Feature:    "source_type_" + sourceType,
		Message:    fmt.Sprintf("%s sources require an Enterprise license", sourceType),
		UpgradeURL: DefaultUpgradeURL,
	}
}

// NewJobTypeError creates a validation error for job type restrictions.
func NewJobTypeError(jobType string) *ValidationError {
	return &ValidationError{
		Feature:    "job_type_" + jobType,
		Message:    fmt.Sprintf("%s jobs require an Enterprise license", jobType),
		UpgradeURL: DefaultUpgradeURL,
	}
}

// NewSchedulingError creates a validation error for scheduling restrictions.
func NewSchedulingError() *ValidationError {
	return &ValidationError{
		Feature:    "scheduling",
		Message:    "Scheduled jobs require an Enterprise license",
		UpgradeURL: DefaultUpgradeURL,
	}
}

// NewWorkerReplicasError creates a validation error for worker replica restrictions.
func NewWorkerReplicasError(requested, max int) *ValidationError {
	return &ValidationError{
		Feature: "worker_replicas",
		Message: fmt.Sprintf(
			"Requested %d worker replicas exceeds the open-core limit of %d. "+
				"Multiple workers require an Enterprise license", requested, max),
		UpgradeURL: DefaultUpgradeURL,
	}
}

// NewScenarioCountError creates a validation error for scenario count restrictions.
func NewScenarioCountError(count, max int) *ValidationError {
	return &ValidationError{
		Feature: "scenario_count",
		Message: fmt.Sprintf(
			"Scenario count %d exceeds the open-core limit of %d. "+
				"Unlimited scenarios require an Enterprise license", count, max),
		UpgradeURL: DefaultUpgradeURL,
	}
}

// NewLicenseExpiredError creates a validation error for expired license.
func NewLicenseExpiredError() *ValidationError {
	return &ValidationError{
		Feature:    "license_expired",
		Message:    "Your Enterprise license has expired. Please renew to continue using Enterprise features",
		UpgradeURL: DefaultUpgradeURL,
	}
}
