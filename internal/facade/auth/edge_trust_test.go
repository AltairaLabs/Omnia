/*
Copyright 2026.

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

package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const testAliceEmail = "alice@example.com"

func reqWithEdgeHeaders(headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestEdgeTrustValidator_AdmitsWithDefaultIstioHeaders(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator()
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: testAliceEmail,
		auth.DefaultEdgeRoleHeader:    policy.RoleEditor,
		auth.DefaultEdgeEmailHeader:   testAliceEmail,
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Origin, policy.OriginEdgeTrust; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Subject, testAliceEmail; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := id.Role, policy.RoleEditor; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
	if got, want := id.Claims["email"], testAliceEmail; got != want {
		t.Errorf("Claims[email] = %q, want %q", got, want)
	}
	// Default endUserHeader = subjectHeader, so they should match when
	// only subject is set.
	if id.EndUser != id.Subject {
		t.Errorf("EndUser = %q, want %q (default mapping coincides with Subject)", id.EndUser, id.Subject)
	}
}

func TestEdgeTrustValidator_FallsThroughWhenSubjectAbsent(t *testing.T) {
	// No subject header → we have nothing to attribute to. Falling
	// through is safer than admitting an anonymous request from a
	// supposedly-trusted edge.
	t.Parallel()
	v := auth.NewEdgeTrustValidator()
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeRoleHeader:  "editor", // role without subject
		auth.DefaultEdgeEmailHeader: "x@example.com",
	})

	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestEdgeTrustValidator_DefaultRoleAppliedWhenRoleHeaderAbsent(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator()
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Role, policy.RoleViewer; got != want {
		t.Errorf("Role = %q, want %q (default)", got, want)
	}
}

func TestEdgeTrustValidator_DefaultRoleOverride(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator(auth.WithEdgeTrustDefaultRole(policy.RoleAdmin))
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Role, policy.RoleAdmin; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
}

func TestEdgeTrustValidator_CustomHeaderMapping(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustSubjectHeader("X-Auth-Subject"),
		auth.WithEdgeTrustRoleHeader("X-Auth-Role"),
		auth.WithEdgeTrustEndUserHeader("X-Auth-Subject"),
		auth.WithEdgeTrustEmailHeader("X-Auth-Email"),
	)
	r := reqWithEdgeHeaders(map[string]string{
		"X-Auth-Subject": "bob@example.com",
		"X-Auth-Role":    policy.RoleAdmin,
		"X-Auth-Email":   "bob@example.com",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Subject, "bob@example.com"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := id.Role, policy.RoleAdmin; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}

	// Default headers must NOT be read once they're overridden.
	r2 := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "should-be-ignored",
	})
	if _, err := v.Validate(context.Background(), r2); !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("default header should be ignored after override: err = %v", err)
	}
}

func TestEdgeTrustValidator_DistinctEndUserHeader(t *testing.T) {
	// When the edge supplies separate subject/endUser headers (e.g., a
	// service token acting on behalf of a human), the validator must
	// surface them distinctly so ToolPolicy CEL can compare them.
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustSubjectHeader("X-Service-Id"),
		auth.WithEdgeTrustEndUserHeader("X-On-Behalf-Of"),
	)
	r := reqWithEdgeHeaders(map[string]string{
		"X-Service-Id":   "svc-payroll",
		"X-On-Behalf-Of": testAliceEmail,
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id.Subject == id.EndUser {
		t.Errorf("Subject and EndUser should differ: both = %q", id.Subject)
	}
	if got, want := id.Subject, "svc-payroll"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := id.EndUser, testAliceEmail; got != want {
		t.Errorf("EndUser = %q, want %q", got, want)
	}
}

func TestEdgeTrustValidator_ExtraClaimsIgnoresEmptyEntries(t *testing.T) {
	// A mapping entry with an empty header OR an empty claim name is
	// garbage — the option should skip it rather than admit junk keys.
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustExtraClaims(map[string]string{
			"":              "dropped-missing-header",
			"X-Header":      "",
			"X-Real-Header": "real-claim",
		}),
	)
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
		"X-Real-Header":               "value",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Claims["real-claim"], "value"; got != want {
		t.Errorf("Claims[real-claim] = %q, want %q", got, want)
	}
	if _, ok := id.Claims["dropped-missing-header"]; ok {
		t.Error("empty-header entry should have been dropped")
	}
}

func TestEdgeTrustValidator_ExtraClaimsPropagate(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustExtraClaims(map[string]string{
			"X-User-Groups": "groups",
			"X-Tenant-Id":   "tenant",
		}),
	)
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
		"X-User-Groups":               "finance,engineering",
		"X-Tenant-Id":                 "acme",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Claims["groups"], "finance,engineering"; got != want {
		t.Errorf("Claims[groups] = %q, want %q", got, want)
	}
	if got, want := id.Claims["tenant"], "acme"; got != want {
		t.Errorf("Claims[tenant] = %q, want %q", got, want)
	}
}

func TestEdgeTrustValidator_ExtraClaimsAbsentIsOK(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustExtraClaims(map[string]string{"X-User-Groups": "groups"}),
	)
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := id.Claims["groups"]; ok {
		t.Error("absent extra-claim header should not produce an empty value in Claims")
	}
}

func TestEdgeTrustValidator_EmptyOptionsIgnored(t *testing.T) {
	// Empty strings passed to the With* options must NOT clear the
	// default mapping — the chart's CRD allows empty fields and we
	// don't want to silently disable the defaults.
	t.Parallel()
	v := auth.NewEdgeTrustValidator(
		auth.WithEdgeTrustSubjectHeader(""),
		auth.WithEdgeTrustRoleHeader(""),
		auth.WithEdgeTrustEndUserHeader(""),
		auth.WithEdgeTrustEmailHeader(""),
		auth.WithEdgeTrustDefaultRole(""),
	)
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id.Subject != "alice" {
		t.Errorf("Subject = %q, want %q (default header should still work)", id.Subject, "alice")
	}
	if id.Role != policy.RoleViewer {
		t.Errorf("Role = %q, want %q (default role should still apply)", id.Role, policy.RoleViewer)
	}
}

func TestEdgeTrustValidator_NoEmailHeaderOmitsClaim(t *testing.T) {
	t.Parallel()
	v := auth.NewEdgeTrustValidator()
	r := reqWithEdgeHeaders(map[string]string{
		auth.DefaultEdgeSubjectHeader: "alice",
	})

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id.Claims != nil {
		if _, ok := id.Claims["email"]; ok {
			t.Errorf("Claims[email] = %q, want missing when email header absent", id.Claims["email"])
		}
	}
}
