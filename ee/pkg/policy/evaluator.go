/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// Header name constants for CEL evaluation context.
const (
	HeaderToolName     = "X-Omnia-Tool-Name"
	HeaderToolRegistry = "X-Omnia-Tool-Registry"
	HeaderClaimPrefix  = "X-Omnia-Claim-"
)

// Decision represents the outcome of a policy evaluation.
type Decision struct {
	// Allowed indicates whether the request is allowed.
	Allowed bool
	// DeniedBy is the name of the rule that denied the request (empty if allowed).
	DeniedBy string
	// Message is the denial message (empty if allowed).
	Message string
	// Error is set if evaluation encountered an error.
	Error error
	// PolicyName is the name of the policy that produced this decision.
	PolicyName string
	// PolicyMode is the mode (enforce/audit) of the policy that produced this decision.
	PolicyMode string
}

// CompiledRule holds a pre-compiled CEL program for a single policy rule.
type CompiledRule struct {
	Name    string
	Program cel.Program
	Message string
}

// CompiledPolicy holds a set of pre-compiled rules for a ToolPolicy.
type CompiledPolicy struct {
	Name           string
	Namespace      string
	Selector       omniav1alpha1.ToolPolicySelector
	Rules          []CompiledRule
	RequiredClaims []omniav1alpha1.RequiredClaim
	Mode           omniav1alpha1.PolicyMode
	OnFailure      omniav1alpha1.OnFailureAction
	Audit          *omniav1alpha1.ToolPolicyAuditConfig
}

// Evaluator compiles and evaluates CEL-based ToolPolicy rules.
type Evaluator struct {
	mu       sync.RWMutex
	env      *cel.Env
	policies map[string]*CompiledPolicy // key: namespace/name
}

// NewEvaluator creates a new Evaluator with a shared CEL environment.
func NewEvaluator() (*Evaluator, error) {
	env, err := newCELEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	return &Evaluator{
		env:      env,
		policies: make(map[string]*CompiledPolicy),
	}, nil
}

// newCELEnv creates the shared CEL environment with the variables available to rules.
func newCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("headers", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("body", cel.MapType(cel.StringType, cel.DynType)),
		ext.Strings(),
	)
}

// CompilePolicy compiles all rules in a ToolPolicy and stores the result.
func (e *Evaluator) CompilePolicy(policy *omniav1alpha1.ToolPolicy) error {
	compiled, err := e.compileRules(policy)
	if err != nil {
		return err
	}

	key := policyKey(policy.Namespace, policy.Name)
	e.mu.Lock()
	e.policies[key] = compiled
	e.mu.Unlock()
	return nil
}

// compileRules compiles all rules in a ToolPolicy.
func (e *Evaluator) compileRules(policy *omniav1alpha1.ToolPolicy) (*CompiledPolicy, error) {
	compiled := &CompiledPolicy{
		Name:           policy.Name,
		Namespace:      policy.Namespace,
		Selector:       policy.Spec.Selector,
		RequiredClaims: policy.Spec.RequiredClaims,
		Mode:           policy.Spec.Mode,
		OnFailure:      policy.Spec.OnFailure,
		Audit:          policy.Spec.Audit,
		Rules:          make([]CompiledRule, 0, len(policy.Spec.Rules)),
	}

	for _, rule := range policy.Spec.Rules {
		program, err := compileCEL(e.env, rule.Deny.CEL)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.Name, err)
		}
		compiled.Rules = append(compiled.Rules, CompiledRule{
			Name:    rule.Name,
			Program: program,
			Message: rule.Deny.Message,
		})
	}

	return compiled, nil
}

// compileCEL compiles a single CEL expression.
func compileCEL(env *cel.Env, expr string) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile error: %w", issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program error: %w", err)
	}
	return program, nil
}

// RemovePolicy removes a compiled policy from the evaluator.
func (e *Evaluator) RemovePolicy(namespace, name string) {
	key := policyKey(namespace, name)
	e.mu.Lock()
	delete(e.policies, key)
	e.mu.Unlock()
}

// Evaluate evaluates all matching policies against the given context.
// It returns a Decision indicating whether the request should be allowed.
// In audit mode, the decision will be Allowed=true but DeniedBy will be set
// to indicate which rule would have denied the request.
func (e *Evaluator) Evaluate(headers map[string]string, body map[string]interface{}) Decision {
	e.mu.RLock()
	matching := e.findMatchingPolicies(headers)
	e.mu.RUnlock()

	var auditDecision *Decision
	var lastDecision Decision
	for _, p := range matching {
		lastDecision = e.evaluatePolicy(p, headers, body)
		if !lastDecision.Allowed {
			return lastDecision
		}
		// Track the first audit-mode denial for reporting
		if auditDecision == nil && lastDecision.DeniedBy != "" {
			d := lastDecision
			auditDecision = &d
		}
	}
	if auditDecision != nil {
		return *auditDecision
	}
	// Preserve policy metadata from the last evaluated policy
	lastDecision.Allowed = true
	return lastDecision
}

// findMatchingPolicies returns policies whose selector matches the request headers.
func (e *Evaluator) findMatchingPolicies(headers map[string]string) []*CompiledPolicy {
	toolName := headers[HeaderToolName]
	toolRegistry := headers[HeaderToolRegistry]

	var matching []*CompiledPolicy
	for _, policy := range e.policies {
		if matchesSelector(policy.Selector, toolRegistry, toolName) {
			matching = append(matching, policy)
		}
	}
	return matching
}

// matchesSelector checks if a tool invocation matches a policy selector.
func matchesSelector(selector omniav1alpha1.ToolPolicySelector, registry, tool string) bool {
	if selector.Registry != registry {
		return false
	}
	if len(selector.Tools) == 0 {
		return true
	}
	for _, t := range selector.Tools {
		if t == tool {
			return true
		}
	}
	return false
}

// evaluatePolicy evaluates a single compiled policy against the given context.
func (e *Evaluator) evaluatePolicy(
	policy *CompiledPolicy,
	headers map[string]string,
	body map[string]interface{},
) Decision {
	// Check required claims first
	if decision := checkRequiredClaims(policy.RequiredClaims, headers); !decision.Allowed {
		return applyMode(policy, decision)
	}

	// Evaluate CEL rules
	activation := buildActivation(headers, body)
	for _, rule := range policy.Rules {
		decision := evaluateRule(rule, activation, policy.OnFailure)
		if !decision.Allowed {
			return applyMode(policy, decision)
		}
		// Propagate errors even if the rule allowed the request (onFailure=allow)
		if decision.Error != nil {
			decision.PolicyName = policy.Name
			decision.PolicyMode = string(policy.Mode)
			return decision
		}
	}
	return Decision{Allowed: true, PolicyName: policy.Name, PolicyMode: string(policy.Mode)}
}

// checkRequiredClaims verifies that all required claims are present in headers.
func checkRequiredClaims(claims []omniav1alpha1.RequiredClaim, headers map[string]string) Decision {
	for _, claim := range claims {
		headerKey := HeaderClaimPrefix + claim.Claim
		if _, ok := headers[headerKey]; !ok {
			return Decision{
				Allowed:  false,
				DeniedBy: "required-claim:" + claim.Claim,
				Message:  claim.Message,
			}
		}
	}
	return Decision{Allowed: true}
}

// buildActivation creates the CEL activation map from headers and body.
func buildActivation(headers map[string]string, body map[string]interface{}) map[string]interface{} {
	activation := map[string]interface{}{
		"headers": headers,
	}
	if body != nil {
		activation["body"] = body
	} else {
		activation["body"] = map[string]interface{}{}
	}
	return activation
}

// evaluateRule evaluates a single CEL rule and returns the decision.
func evaluateRule(
	rule CompiledRule,
	activation map[string]interface{},
	onFailure omniav1alpha1.OnFailureAction,
) Decision {
	out, _, err := rule.Program.Eval(activation)
	if err != nil {
		return handleEvalError(rule.Name, err, onFailure)
	}

	denied, ok := isTruthy(out)
	if !ok {
		return handleEvalError(rule.Name, fmt.Errorf("rule returned non-bool type: %s", out.Type()), onFailure)
	}

	if denied {
		return Decision{
			Allowed:  false,
			DeniedBy: rule.Name,
			Message:  rule.Message,
		}
	}
	return Decision{Allowed: true}
}

// isTruthy checks if a CEL output value is a boolean true.
func isTruthy(val ref.Val) (bool, bool) {
	if val.Type() == types.BoolType {
		b, ok := val.Value().(bool)
		return b, ok
	}
	return false, false
}

// handleEvalError creates a decision based on the onFailure action.
func handleEvalError(ruleName string, err error, onFailure omniav1alpha1.OnFailureAction) Decision {
	if onFailure == omniav1alpha1.OnFailureAllow {
		return Decision{Allowed: true, Error: err}
	}
	return Decision{
		Allowed:  false,
		DeniedBy: ruleName,
		Message:  fmt.Sprintf("policy evaluation error: %v", err),
		Error:    err,
	}
}

// applyMode adjusts the decision based on the policy mode.
// In audit mode, denials are converted to allow but the decision info is preserved.
func applyMode(policy *CompiledPolicy, decision Decision) Decision {
	decision.PolicyName = policy.Name
	decision.PolicyMode = string(policy.Mode)
	if policy.Mode == omniav1alpha1.PolicyModeAudit {
		decision.Allowed = true
	}
	return decision
}

// ValidateCEL checks if a CEL expression compiles without error.
func (e *Evaluator) ValidateCEL(expr string) error {
	_, err := compileCEL(e.env, expr)
	return err
}

// PolicyCount returns the number of currently compiled policies.
func (e *Evaluator) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}

// policyKey returns a unique key for a policy.
func policyKey(namespace, name string) string {
	return namespace + "/" + name
}
