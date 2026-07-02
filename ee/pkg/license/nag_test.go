/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import (
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
)

// recordingLogger returns a logr.Logger that appends every message to msgs.
func recordingLogger(msgs *[]string) (loggerFn func(prefix, args string)) {
	return func(_, args string) { *msgs = append(*msgs, args) }
}

func TestNagIfUnlicensed_SilentForValidEnterprise(t *testing.T) {
	var msgs []string
	log := funcr.New(recordingLogger(&msgs), funcr.Options{})

	NagIfUnlicensed(DevLicense(), log)

	assert.Empty(t, msgs, "a valid enterprise license must not nag")
}

func TestNagIfUnlicensed_NagsForOpenCore(t *testing.T) {
	var msgs []string
	log := funcr.New(recordingLogger(&msgs), funcr.Options{})

	NagIfUnlicensed(OpenCoreLicense(), log)

	assert.NotEmpty(t, msgs, "open-core must nag")
	joined := ""
	for _, m := range msgs {
		joined += m
	}
	assert.Contains(t, joined, LicensingURL, "the nag should point at the licensing URL")
}

func TestNagIfUnlicensed_NagsForExpired(t *testing.T) {
	var msgs []string
	log := funcr.New(recordingLogger(&msgs), funcr.Options{})

	expired := DevLicense()
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	NagIfUnlicensed(expired, log)

	assert.NotEmpty(t, msgs, "an expired enterprise license must nag")
}

func TestNagIfUnlicensed_NagsForNil(t *testing.T) {
	var msgs []string
	log := funcr.New(recordingLogger(&msgs), funcr.Options{})

	NagIfUnlicensed(nil, log)

	assert.NotEmpty(t, msgs, "a nil (absent) license must nag, not panic")
}
