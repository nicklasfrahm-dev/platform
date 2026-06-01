// Package authz provides policy-based access control for skatd.
package authz

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nicklasfrahm-dev/platform/cmd/skatd/api"
	"github.com/nicklasfrahm-dev/platform/cmd/skatd/authn"
)

// Engine evaluates access requests against a set of Policies.
type Engine interface {
	// Allowed returns true if the principal identified by claims may
	// perform verb on resource in namespace.
	Allowed(claims *authn.Claims, verb, resource, namespace string) bool
	// Reload atomically replaces the full policy set.
	Reload(policies []api.Policy)
}

type engine struct {
	mu       sync.RWMutex
	policies []api.Policy
}

// New creates an Engine with an initial policy set.
func New(initial []api.Policy) Engine {
	return &engine{policies: initial}
}

func (e *engine) Reload(policies []api.Policy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies = policies
}

func (e *engine) Allowed(claims *authn.Claims, verb, resource, namespace string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, p := range e.policies {
		if matchSubject(claims, p.Spec.Subjects) && matchRules(verb, resource, namespace, p.Spec.Rules) {
			return true
		}
	}
	return false
}

func matchSubject(claims *authn.Claims, subjects []api.Subject) bool {
	for _, s := range subjects {
		if s.Issuer != "" && s.Issuer != claims.Issuer {
			continue
		}
		if !claimsMatch(claims.Extra, s.Claims) {
			continue
		}
		return true
	}
	return false
}

func claimsMatch(extra map[string]interface{}, required map[string]string) bool {
	for k, v := range required {
		actual, ok := extra[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", actual) != v {
			return false
		}
	}
	return true
}

func matchRules(verb, resource, namespace string, rules []api.Rule) bool {
	for _, rule := range rules {
		if !matchOne(verb, rule.Verbs) {
			continue
		}
		if !matchOne(resource, rule.Resources) {
			continue
		}
		if len(rule.Namespaces) > 0 && !matchOne(namespace, rule.Namespaces) {
			continue
		}
		return true
	}
	return false
}

func matchOne(value string, list []string) bool {
	for _, item := range list {
		if item == "*" || strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
