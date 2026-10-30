// Package auth separates machine authentication from declarative authorization.
package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Claims normalizes signature-verified provider claims for authorization.
type Claims struct {
	ClientID string   `json:"client_id"`
	Scope    string   `json:"scope"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// ClaimsMapping names exact provider claims, or RFC 6901 JSON pointers for nested claims.
// TokenUse/TokenUseValue optionally distinguish access tokens from other JWT types.
type ClaimsMapping struct {
	ClientID, Scopes, Roles string
	TokenUse, TokenUseValue string
}

func (m ClaimsMapping) configured() (ClaimsMapping, error) {
	if m.ClientID == "" {
		m.ClientID = "client_id"
	}
	if m.Scopes == "" {
		m.Scopes = "scope"
	}
	if m.Roles == "" {
		m.Roles = "roles"
	}
	for _, path := range []string{m.ClientID, m.Scopes, m.Roles, m.TokenUse} {
		if path == "" {
			continue
		}
		if len(path) > 512 {
			return m, errors.New("auth claims: claim selector exceeds limit")
		}
		if _, err := claimSegments(path); err != nil {
			return m, err
		}
	}
	if (m.TokenUse == "") != (m.TokenUseValue == "") || (m.TokenUseValue != "" && !validValue(m.TokenUseValue)) {
		return m, errors.New("auth claims: incomplete token-use constraint")
	}
	return m, nil
}

func claimSegments(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return []string{path}, nil
	}
	parts := strings.Split(path[1:], "/")
	if len(parts) > 16 {
		return nil, errors.New("auth claims: nested selector exceeds limit")
	}
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' {
				if j+1 == len(part) || (part[j+1] != '0' && part[j+1] != '1') {
					return nil, errors.New("auth claims: invalid JSON pointer")
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func mappedClaim(raw jwt.MapClaims, path string) any {
	parts, err := claimSegments(path)
	if err != nil {
		return nil
	}
	var current any = map[string]any(raw)
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

func claimStrings(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	array, ok := value.([]any)
	if !ok || len(array) > 64 {
		return nil, ErrInvalidToken
	}
	values := make([]string, len(array))
	for i, item := range array {
		value, ok := item.(string)
		if !ok {
			return nil, ErrInvalidToken
		}
		values[i] = value
	}
	if !validValues(values) {
		return nil, ErrInvalidToken
	}
	return values, nil
}

func (m ClaimsMapping) normalize(raw jwt.MapClaims) (*Claims, error) {
	subject, err := raw.GetSubject()
	if err != nil || !validValue(subject) {
		return nil, ErrInvalidToken
	}
	issuedAt, err := raw.GetIssuedAt()
	if err != nil || issuedAt == nil {
		return nil, ErrInvalidToken
	}
	expiresAt, err := raw.GetExpirationTime()
	if err != nil || expiresAt == nil {
		return nil, ErrInvalidToken
	}
	notBefore, err := raw.GetNotBefore()
	if err != nil {
		return nil, ErrInvalidToken
	}
	issuer, err := raw.GetIssuer()
	if err != nil {
		return nil, ErrInvalidToken
	}
	audience, err := raw.GetAudience()
	if err != nil {
		return nil, ErrInvalidToken
	}
	clientID, ok := mappedClaim(raw, m.ClientID).(string)
	if !ok || !validValue(clientID) {
		return nil, ErrInvalidToken
	}
	roles, err := claimStrings(mappedClaim(raw, m.Roles))
	if err != nil {
		return nil, err
	}
	scope := ""
	if value := mappedClaim(raw, m.Scopes); value != nil {
		if text, ok := value.(string); ok {
			scope = text
		} else {
			values, err := claimStrings(value)
			if err != nil {
				return nil, err
			}
			scope = strings.Join(values, " ")
		}
	}
	if !validValues(scopeValues(scope)) {
		return nil, ErrInvalidToken
	}
	if m.TokenUse != "" {
		value, ok := mappedClaim(raw, m.TokenUse).(string)
		if !ok || value != m.TokenUseValue {
			return nil, ErrInvalidToken
		}
	}
	jti := ""
	if value, exists := raw["jti"]; exists {
		text, ok := value.(string)
		if !ok || !validValue(text) {
			return nil, ErrInvalidToken
		}
		jti = text
	}
	return &Claims{ClientID: clientID, Scope: scope, Roles: roles, RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: subject, Audience: audience, IssuedAt: issuedAt, NotBefore: notBefore, ExpiresAt: expiresAt, ID: jti}}, nil
}

type claimsContextKey struct{}

// ClaimsFromContext returns a copy of claims installed only by authentication middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*Claims)
	if !ok || claims == nil {
		return nil, false
	}
	copy := *claims
	copy.Roles = append([]string(nil), claims.Roles...)
	copy.Audience = append(jwt.ClaimStrings(nil), claims.Audience...)
	return &copy, true
}

func validValue(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}

func validValues(values []string) bool {
	if len(values) > 64 {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !validValue(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func scopeValues(scope string) []string { return strings.Fields(scope) }
