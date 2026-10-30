package auth

import "testing"

func TestPolicyContract(t *testing.T) {
	t.Parallel()
	claims := &Claims{ClientID: "service-a", Scope: "orders:read orders:delete", Roles: []string{"orders:reader", "orders:admin"}}
	for _, tc := range []struct {
		name   string
		policy Policy
		want   bool
	}{
		{"read", Policy{Scopes: All("orders:read"), Roles: Any("orders:reader", "orders:admin"), Clients: Any("service-a")}, true},
		{"no role hierarchy", Policy{Scopes: All("orders:delete"), Roles: All("orders:admin", "orders:delete")}, false},
		{"scope exact", Policy{Scopes: Any("orders")}, false},
		{"client conjunction", Policy{Scopes: All("orders:read"), Clients: Any("other")}, false},
		{"empty fail closed", Policy{}, false},
		{"missing scope", Policy{Scopes: All("orders:write")}, false},
		{"any role", Policy{Roles: Any("missing", "orders:reader")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.Allows(claims); got != tc.want {
				t.Fatalf("Allows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPolicyValidation(t *testing.T) {
	for _, policy := range []Policy{
		{}, {Scopes: Requirement{Mode: MatchMode(42), Values: []string{"x"}}},
		{Roles: Any("")}, {Scopes: Any("orders:read ")}, {Clients: Any("a", "a")},
		{Roles: Requirement{Mode: MatchMode(42)}},
	} {
		if policy.Validate() == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	if err := (Policy{Scopes: All("orders:read")}).Validate(); err != nil {
		t.Fatal(err)
	}
}
