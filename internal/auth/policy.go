package auth

import "errors"

// MatchMode controls exact ANY or ALL permission matching, never role hierarchy.
type MatchMode uint8

const (
	MatchAny MatchMode = iota
	MatchAll
)

// Requirement constrains one policy dimension; no values means unrestricted.
type Requirement struct {
	Mode   MatchMode
	Values []string
}

// Any requires at least one exact value and takes ownership of a copy.
func Any(values ...string) Requirement {
	return Requirement{Mode: MatchAny, Values: append([]string(nil), values...)}
}

// All requires every exact value and takes ownership of a copy.
func All(values ...string) Requirement {
	return Requirement{Mode: MatchAll, Values: append([]string(nil), values...)}
}

// Match evaluates one dimension, including an unrestricted omitted dimension.
func (r Requirement) Match(actual []string) bool {
	if r.Mode != MatchAny && r.Mode != MatchAll || !validValues(r.Values) {
		return false
	}
	if len(r.Values) == 0 {
		return true
	}
	set := make(map[string]bool, len(actual))
	for _, value := range actual {
		set[value] = true
	}
	for _, value := range r.Values {
		if r.Mode == MatchAny && set[value] {
			return true
		}
		if r.Mode == MatchAll && !set[value] {
			return false
		}
	}
	return r.Mode == MatchAll
}

// Policy combines configured scopes, roles and client IDs with AND.
type Policy struct {
	Scopes  Requirement
	Roles   Requirement
	Clients Requirement
}

// Validate rejects empty policies, invalid modes, whitespace and duplicate entries.
func (p Policy) Validate() error {
	configured := false
	for _, r := range []Requirement{p.Scopes, p.Roles, p.Clients} {
		if r.Mode != MatchAny && r.Mode != MatchAll || !validValues(r.Values) {
			return errors.New("auth policy: invalid requirement")
		}
		configured = configured || len(r.Values) != 0
	}
	if !configured {
		return errors.New("auth policy: protected routes require an explicit policy")
	}
	return nil
}

// Allows evaluates permission matching on already verified claims, failing closed on invalid policy.
func (p Policy) Allows(claims *Claims) bool {
	return claims != nil && p.Validate() == nil && p.Scopes.Match(scopeValues(claims.Scope)) && p.Roles.Match(claims.Roles) && p.Clients.Match([]string{claims.ClientID})
}

func (p Policy) frozen() Policy {
	return Policy{Scopes: Requirement{p.Scopes.Mode, append([]string(nil), p.Scopes.Values...)}, Roles: Requirement{p.Roles.Mode, append([]string(nil), p.Roles.Values...)}, Clients: Requirement{p.Clients.Mode, append([]string(nil), p.Clients.Values...)}}
}
