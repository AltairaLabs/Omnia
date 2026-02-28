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
)

// Header key constants for CEL variables.
const (
	// CELVarHeaders is the CEL variable name for HTTP headers.
	CELVarHeaders = "headers"
	// CELVarBody is the CEL variable name for request body.
	CELVarBody = "body"
)

// Decision represents the result of evaluating a single policy rule.
type Decision struct {
	// RuleName is the name of the rule that was evaluated.
	RuleName string
	// Denied indicates whether the rule denied the request.
	Denied bool
	// Message is the denial message, empty if not denied.
	Message string
}

// EvalResult represents the complete result of evaluating all rules for a policy.
type EvalResult struct {
	// PolicyName is the name of the ToolPolicy that was evaluated.
	PolicyName string
	// PolicyNamespace is the namespace of the ToolPolicy.
	PolicyNamespace string
	// Decisions contains the result of each rule evaluation.
	Decisions []Decision
	// Denied indicates whether the request was denied by any rule.
	Denied bool
	// DenyMessage is the first denial message encountered.
	DenyMessage string
	// Error is set if evaluation itself failed.
	Error error
}

// CompiledRule holds a pre-compiled CEL program for a policy rule.
type CompiledRule struct {
	Name    string
	Message string
	Program cel.Program
}

// CompiledPolicy holds all compiled rules for a single ToolPolicy.
type CompiledPolicy struct {
	Name      string
	Namespace string
	Registry  string
	Tools     []string
	Rules     []CompiledRule
	Mode      string
	OnFailure string
}

// Evaluator compiles and evaluates CEL expressions for tool policies.
type Evaluator struct {
	mu       sync.RWMutex
	env      *cel.Env
	policies map[string]*CompiledPolicy // key: namespace/name
}

// NewEvaluator creates a new CEL evaluator with the standard environment.
func NewEvaluator() (*Evaluator, error) {
	env, err := newCELEnv()
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}
	return &Evaluator{
		env:      env,
		policies: make(map[string]*CompiledPolicy),
	}, nil
}

// newCELEnv creates the standard CEL environment with headers and body variables.
func newCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable(CELVarHeaders, cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable(CELVarBody, cel.MapType(cel.StringType, cel.DynType)),
		ext.Strings(),
	)
}

// CompileRule compiles a single CEL expression and returns a CompiledRule.
func (e *Evaluator) CompileRule(name, expression, message string) (*CompiledRule, error) {
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compiling rule %q: %w", name, issues.Err())
	}

	// Verify the output type is bool.
	if !ast.OutputType().IsExactType(types.BoolType) {
		return nil, fmt.Errorf("rule %q: CEL expression must return bool, got %s", name, ast.OutputType())
	}

	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("programming rule %q: %w", name, err)
	}

	return &CompiledRule{
		Name:    name,
		Message: message,
		Program: prg,
	}, nil
}

// policyKey returns the map key for a policy.
func policyKey(namespace, name string) string {
	return namespace + "/" + name
}

// SetPolicy compiles and stores all rules for a ToolPolicy.
// Returns an error if any rule fails to compile.
func (e *Evaluator) SetPolicy(
	name, namespace, registry string,
	tools []string,
	rules []RuleInput,
	mode, onFailure string,
) error {
	compiled := make([]CompiledRule, 0, len(rules))
	for _, r := range rules {
		cr, err := e.CompileRule(r.Name, r.CEL, r.Message)
		if err != nil {
			return err
		}
		compiled = append(compiled, *cr)
	}

	policy := &CompiledPolicy{
		Name:      name,
		Namespace: namespace,
		Registry:  registry,
		Tools:     tools,
		Rules:     compiled,
		Mode:      mode,
		OnFailure: onFailure,
	}

	e.mu.Lock()
	e.policies[policyKey(namespace, name)] = policy
	e.mu.Unlock()

	return nil
}

// RemovePolicy removes a compiled policy from the evaluator.
func (e *Evaluator) RemovePolicy(namespace, name string) {
	e.mu.Lock()
	delete(e.policies, policyKey(namespace, name))
	e.mu.Unlock()
}

// RuleInput is the input needed to compile a rule.
type RuleInput struct {
	Name    string
	CEL     string
	Message string
}

// EvalContext holds the input variables for CEL evaluation.
type EvalContext struct {
	Headers map[string]string
	Body    map[string]interface{}
}

// Evaluate evaluates all matching policies for a given tool call.
func (e *Evaluator) Evaluate(registry, tool string, evalCtx *EvalContext) []EvalResult {
	e.mu.RLock()
	policies := e.matchingPolicies(registry, tool)
	e.mu.RUnlock()

	results := make([]EvalResult, 0, len(policies))
	for _, p := range policies {
		result := e.evaluatePolicy(p, evalCtx)
		results = append(results, result)
	}
	return results
}

// matchingPolicies returns policies that match the given registry/tool.
// Must be called with mu.RLock held.
func (e *Evaluator) matchingPolicies(registry, tool string) []*CompiledPolicy {
	var matched []*CompiledPolicy
	for _, p := range e.policies {
		if p.Registry != registry {
			continue
		}
		if matchesTool(p.Tools, tool) {
			matched = append(matched, p)
		}
	}
	return matched
}

// matchesTool checks if a tool matches the policy's tool list.
// An empty tools list matches all tools.
func matchesTool(policyTools []string, tool string) bool {
	if len(policyTools) == 0 {
		return true
	}
	for _, t := range policyTools {
		if t == tool {
			return true
		}
	}
	return false
}

// evaluatePolicy evaluates all rules in a single policy.
func (e *Evaluator) evaluatePolicy(p *CompiledPolicy, evalCtx *EvalContext) EvalResult {
	result := EvalResult{
		PolicyName:      p.Name,
		PolicyNamespace: p.Namespace,
		Decisions:       make([]Decision, 0, len(p.Rules)),
	}

	activation := buildActivation(evalCtx)

	for i := range p.Rules {
		decision := evaluateRule(&p.Rules[i], activation)
		result.Decisions = append(result.Decisions, decision)
		if decision.Denied && !result.Denied {
			result.Denied = true
			result.DenyMessage = decision.Message
		}
	}

	return result
}

// evaluateRule evaluates a single compiled rule against the activation.
func evaluateRule(rule *CompiledRule, activation map[string]interface{}) Decision {
	out, _, err := rule.Program.Eval(activation)
	if err != nil {
		return Decision{
			RuleName: rule.Name,
			Denied:   true,
			Message:  fmt.Sprintf("rule evaluation error: %v", err),
		}
	}

	denied := isTruthy(out)
	decision := Decision{
		RuleName: rule.Name,
		Denied:   denied,
	}
	if denied {
		decision.Message = rule.Message
	}
	return decision
}

// buildActivation creates the CEL activation map from the evaluation context.
func buildActivation(evalCtx *EvalContext) map[string]interface{} {
	headers := evalCtx.Headers
	if headers == nil {
		headers = make(map[string]string)
	}
	body := evalCtx.Body
	if body == nil {
		body = make(map[string]interface{})
	}
	return map[string]interface{}{
		CELVarHeaders: headers,
		CELVarBody:    body,
	}
}

// isTruthy checks if a CEL output value is truthy (boolean true).
func isTruthy(val ref.Val) bool {
	if val == nil {
		return false
	}
	b, ok := val.Value().(bool)
	return ok && b
}

// PolicyCount returns the number of loaded policies.
func (e *Evaluator) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}

// ValidateCEL checks whether a CEL expression compiles successfully without storing it.
func (e *Evaluator) ValidateCEL(expression string) error {
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("CEL compilation error: %w", issues.Err())
	}
	if !ast.OutputType().IsExactType(types.BoolType) {
		return fmt.Errorf("CEL expression must return bool, got %s", ast.OutputType())
	}
	return nil
}
